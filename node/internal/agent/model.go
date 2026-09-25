// Package agent is the node's built-in agent: the thing that answers when a
// person types, and keeps working in their threads when they are not.
//
// One runner serves both. It reads only what the person may read, writes
// under its own delegated key on the person's behalf, and leaves a record of
// every run in the thread so that "what did my agent do" is answered by the
// same log as everything else.
package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Message is one turn in the OpenAI-style chat format OpenRouter speaks.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

// ToolCall is a model's request to call a tool.
type ToolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

// ToolSpec describes a tool to the model.
type ToolSpec struct {
	Name        string
	Description string
	Parameters  map[string]any
}

// Usage is what a completion cost.
type Usage struct {
	Prompt     int
	Completion int
}

// Model completes a conversation, optionally calling tools.
type Model interface {
	Complete(ctx context.Context, msgs []Message, tools []ToolSpec) (Message, Usage, error)
}

// DefaultModel is cheap, long-context, and handles tool calls. Override with
// LAMDIS_MODEL; the id is shown in the interface so nobody has to guess.
const DefaultModel = "openai/gpt-5.6-luna"

// OpenRouter is a Model over an OpenAI-compatible chat completions API.
// OpenRouter by default; BaseURL points it at anything else that speaks the
// same shape, such as a local Ollama or vLLM at http://localhost:11434/v1.
type OpenRouter struct {
	Key     string
	Model   string
	BaseURL string
	// KeyIsForBaseURL says the key was set for this endpoint specifically,
	// not inherited from the OpenRouter setting. Only then is it sent to a
	// host that is not OpenRouter.
	KeyIsForBaseURL bool
	HTTP            *http.Client
}

const openRouterURL = "https://openrouter.ai/api/v1"

// openRouterHost is the only host the OpenRouter key is ever sent to. A
// person can point the agent at a local or private model server, and that
// must not become a way to hand their paid key to whoever owns that URL.
const openRouterHost = "openrouter.ai"

// keyGoesHere reports whether the credential belongs to the endpoint being
// called. A key set for OpenRouter travels only to OpenRouter; a key set
// explicitly for a custom endpoint travels only there.
func (o *OpenRouter) keyGoesHere() bool {
	base := strings.TrimRight(o.BaseURL, "/")
	if base == "" || base == openRouterURL {
		return true
	}
	u, err := url.Parse(base)
	if err != nil {
		return false
	}
	if u.Hostname() == openRouterHost || strings.HasSuffix(u.Hostname(), "."+openRouterHost) {
		return true
	}
	// A custom endpoint only ever receives a key the person set for it.
	return o.KeyIsForBaseURL
}

// Endpoint is where completions go.
func (o *OpenRouter) Endpoint() string {
	base := strings.TrimRight(o.BaseURL, "/")
	if base == "" {
		base = openRouterURL
	}
	return base + "/chat/completions"
}

// NewOpenRouter returns nil when no key is set, so callers can ask "can I
// answer" and tell the person how to switch it on.
func NewOpenRouter(key, model string) *OpenRouter {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	if model == "" {
		model = DefaultModel
	}
	return &OpenRouter{Key: key, Model: model, HTTP: &http.Client{Timeout: 120 * time.Second,
		Transport: &http.Transport{
			// The default dialer waits thirty seconds on a handshake that
			// has already failed. Ten is long enough to be patient and
			// short enough that a retry still feels immediate.
			TLSHandshakeTimeout:   10 * time.Second,
			ResponseHeaderTimeout: 110 * time.Second,
			MaxIdleConnsPerHost:   4,
			IdleConnTimeout:       90 * time.Second,
			ForceAttemptHTTP2:     true,
		}}}
}

// Complete asks the model, and rides out the ordinary failures of talking
// to something across the internet.
//
// A handshake that times out once is not news, and it should not reach a
// person as a Go error string with their question thrown away. So the
// things that pass on their own are retried quietly, and only what is
// actually wrong is reported, in words.
func (o *OpenRouter) Complete(ctx context.Context, msgs []Message, tools []ToolSpec) (Message, Usage, error) {
	var last error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			// A moment, then longer, then longer still.
			wait := time.Duration(1<<uint(attempt-1)) * time.Second
			select {
			case <-ctx.Done():
				return Message{}, Usage{}, ctx.Err()
			case <-time.After(wait):
			}
		}
		m, u, err := o.complete(ctx, msgs, tools)
		if err == nil {
			return m, u, nil
		}
		last = err
		if !worthRetrying(err) || ctx.Err() != nil {
			return Message{}, Usage{}, err
		}
	}
	return Message{}, Usage{}, last
}

// retryable marks a failure that is likely to pass on its own.
type retryable struct{ error }

func worthRetrying(err error) bool {
	var r retryable
	return errors.As(err, &r)
}

// friendly turns a transport failure into something worth reading. The
// words matter: somebody who sees "TLS handshake timeout" learns nothing
// they can act on, and assumes the product is broken.
func friendly(err error) error {
	s := err.Error()
	switch {
	case strings.Contains(s, "TLS handshake") || strings.Contains(s, "connection reset") ||
		strings.Contains(s, "connection refused") || strings.Contains(s, "EOF"):
		return retryable{fmt.Errorf("the connection to the model dropped")}
	case strings.Contains(s, "no such host") || strings.Contains(s, "server misbehaving"):
		return retryable{fmt.Errorf("could not look up the model's address; check the network")}
	case strings.Contains(s, "deadline exceeded") || strings.Contains(s, "Client.Timeout"):
		return retryable{fmt.Errorf("the model took too long to answer")}
	case strings.Contains(s, "context canceled"):
		return fmt.Errorf("that was cancelled")
	}
	return retryable{fmt.Errorf("could not reach the model: %w", err)}
}

func (o *OpenRouter) complete(ctx context.Context, msgs []Message, tools []ToolSpec) (Message, Usage, error) {
	req := map[string]any{"model": o.Model, "messages": msgs, "temperature": 0.2}
	if o.isOpenRouter() {
		// Only providers that neither store prompts nor train on them.
		req["provider"] = map[string]any{"data_collection": "deny"}
	}
	if len(tools) > 0 {
		var ts []map[string]any
		for _, t := range tools {
			ts = append(ts, map[string]any{"type": "function", "function": map[string]any{
				"name": t.Name, "description": t.Description, "parameters": t.Parameters}})
		}
		req["tools"] = ts
		req["tool_choice"] = "auto"
	}
	body, err := json.Marshal(req)
	if err != nil {
		return Message{}, Usage{}, err
	}
	hr, err := http.NewRequestWithContext(ctx, "POST", o.Endpoint(), bytes.NewReader(body))
	if err != nil {
		return Message{}, Usage{}, err
	}
	if o.Key != "" && o.keyGoesHere() {
		hr.Header.Set("Authorization", "Bearer "+o.Key)
	}
	hr.Header.Set("Content-Type", "application/json")
	hr.Header.Set("HTTP-Referer", "https://lamdis.ai")
	hr.Header.Set("X-Title", "Lamdis")
	resp, err := o.HTTP.Do(hr)
	if err != nil {
		return Message{}, Usage{}, friendly(err)
	}
	defer resp.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return Message{}, Usage{}, err
	}
	if resp.StatusCode != http.StatusOK {
		// The provider's own words: "insufficient credits" is actionable,
		// "HTTP 402" is not.
		var e struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		msg := fmt.Sprintf("the model returned HTTP %d", resp.StatusCode)
		if json.Unmarshal(raw, &e) == nil && e.Error.Message != "" {
			msg = e.Error.Message
		}
		// Busy and broken pass; refused and unpaid do not.
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			return Message{}, Usage{}, retryable{fmt.Errorf("%s", msg)}
		}
		return Message{}, Usage{}, fmt.Errorf("%s", msg)
	}
	var out struct {
		Choices []struct {
			Message Message `json:"message"`
		} `json:"choices"`
		Usage struct {
			Prompt     int `json:"prompt_tokens"`
			Completion int `json:"completion_tokens"`
		} `json:"usage"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return Message{}, Usage{}, err
	}
	if len(out.Choices) == 0 {
		if out.Error.Message != "" {
			return Message{}, Usage{}, fmt.Errorf("%s", out.Error.Message)
		}
		return Message{}, Usage{}, fmt.Errorf("the model returned nothing")
	}
	m := out.Choices[0].Message
	m.Role = "assistant"
	return m, Usage{Prompt: out.Usage.Prompt, Completion: out.Usage.Completion}, nil
}

func (o *OpenRouter) isOpenRouter() bool {
	b := strings.TrimRight(o.BaseURL, "/")
	return b == "" || b == openRouterURL
}

// SearchCostFetches is how much of the day's fetch budget one web search
// uses: a search costs about what five page fetches do.
const SearchCostFetches = 5

// WebSearch asks OpenRouter's web plugin for current results. It only works
// against OpenRouter itself; a local model server has no search.
func (o *OpenRouter) WebSearch(ctx context.Context, query string) (string, []string, error) {
	if !o.isOpenRouter() || o.Key == "" {
		return "", nil, fmt.Errorf("web search needs an OpenRouter model; this node runs a local one")
	}
	req := map[string]any{
		"model": o.Model,
		"messages": []map[string]string{{"role": "user", "content": "Search the web for: " + query +
			"\nReport what you find as a short list: each result's name, one line on what it is, and its web address or phone number. Only include things the search results actually show."}},
		"plugins":     []map[string]any{{"id": "web", "max_results": 6}},
		"provider":    map[string]any{"data_collection": "deny"},
		"temperature": 0,
	}
	body, _ := json.Marshal(req)
	hr, err := http.NewRequestWithContext(ctx, "POST", o.Endpoint(), bytes.NewReader(body))
	if err != nil {
		return "", nil, err
	}
	hr.Header.Set("Authorization", "Bearer "+o.Key)
	hr.Header.Set("Content-Type", "application/json")
	hr.Header.Set("HTTP-Referer", "https://lamdis.ai")
	hr.Header.Set("X-Title", "Lamdis")
	resp, err := o.HTTP.Do(hr)
	if err != nil {
		return "", nil, friendly(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	var out struct {
		Choices []struct {
			Message struct {
				Content     string `json:"content"`
				Annotations []struct {
					URLCitation struct {
						URL string `json:"url"`
					} `json:"url_citation"`
				} `json:"annotations"`
			} `json:"message"`
		} `json:"choices"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if json.Unmarshal(raw, &out) != nil {
		return "", nil, fmt.Errorf("the search returned something unreadable")
	}
	if out.Error.Message != "" || len(out.Choices) == 0 {
		return "", nil, fmt.Errorf("search failed: %s", out.Error.Message)
	}
	m := out.Choices[0].Message
	var urls []string
	for _, a := range m.Annotations {
		if u := strings.Replace(a.URLCitation.URL, "?utm_source=openai", "", 1); u != "" {
			urls = append(urls, u)
		}
	}
	return strings.ReplaceAll(m.Content, "?utm_source=openai", ""), urls, nil
}
