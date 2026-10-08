// Package agent is the node's built-in agent: the thing that answers when a
// person types, and keeps working in their threads when they are not.
//
// One runner serves both. It reads only what the person may read, writes
// under its own delegated key on the person's behalf, and leaves a record of
// every run in the thread so that "what did my agent do" is answered by the
// same log as everything else.
package agent

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync/atomic"
	"time"
)

// Message is one turn in the OpenAI-style chat format OpenRouter speaks.
type Message struct {
	Role       string     `json:"role"`
	Content    string     `json:"content"`
	ToolCalls  []ToolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
	// parts are attachments sent alongside Content (see attach.go).
	parts []part
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

// Streamer is optional so small test models and integrations can keep using
// the simple completion interface. Interactive clients use it to render the
// first useful words while the provider is still generating the turn.
type Streamer interface {
	Stream(context.Context, []Message, []ToolSpec, func(string)) (Message, Usage, error)
}

// DefaultModel is cheap, long-context, and handles tool calls. Override with
// LAMDIS_MODEL; the id is shown in the interface so nobody has to guess.
const DefaultModel = "z-ai/glm-5.3-flash"

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
	return &OpenRouter{Key: key, Model: model, HTTP: newModelClient()}
}

// newModelClient is the transport every model call uses. It has no overall
// timeout on purpose: a long-context completion that is not streamed sends
// nothing at all until the last token, and a fixed limit here cut off
// answers that were simply long. The caller's context decides how long a
// turn may take; the transport only gives up on connections that never
// open.
func newModelClient() *http.Client {
	return &http.Client{Transport: &http.Transport{
		// The default dialer waits thirty seconds on a handshake that
		// has already failed. Ten is long enough to be patient and
		// short enough that a retry still feels immediate.
		TLSHandshakeTimeout: 10 * time.Second,
		MaxIdleConnsPerHost: 4,
		IdleConnTimeout:     90 * time.Second,
		ForceAttemptHTTP2:   true,
	}}
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

// Stream implements the OpenAI-compatible SSE chat stream. Tool-call deltas
// are accumulated just like a normal completion; only assistant text is
// exposed to the terminal as it arrives.
func (o *OpenRouter) Stream(ctx context.Context, msgs []Message, tools []ToolSpec, onText func(string)) (Message, Usage, error) {
	// The same patience as Complete, for as long as nothing has reached the
	// screen. Once text has been shown, a retry would show it twice, so a
	// failure after that point is the caller's to handle.
	var last error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			wait := time.Duration(1<<uint(attempt-1)) * time.Second
			select {
			case <-ctx.Done():
				return Message{}, Usage{}, ctx.Err()
			case <-time.After(wait):
			}
		}
		shown := false
		m, u, err := o.stream(ctx, msgs, tools, func(t string) {
			shown = true
			if onText != nil {
				onText(t)
			}
		})
		if err == nil {
			return m, u, nil
		}
		last = err
		if shown || !worthRetrying(err) || ctx.Err() != nil {
			return Message{}, u, err
		}
	}
	return Message{}, Usage{}, last
}

// streamIdle is how long a stream may go without producing anything, before
// the first token or between two, before it is treated as stalled. Output
// counts, reasoning included: a model thinking for minutes is producing.
// Keep-alive comments do not count. A router keeps sending them while its
// provider has stopped working on the request, and counting them let a
// hung request look alive for ten minutes.
var streamIdle = 120 * time.Second

func (o *OpenRouter) stream(ctx context.Context, msgs []Message, tools []ToolSpec, onText func(string)) (Message, Usage, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	var stalled atomic.Bool
	idle := time.AfterFunc(streamIdle, func() { stalled.Store(true); cancel() })
	defer idle.Stop()
	req := map[string]any{"model": o.Model, "messages": msgs, "temperature": 0.2, "stream": true}
	if o.isOpenRouter() {
		req["provider"] = map[string]any{"data_collection": "deny"}
	}
	if len(tools) > 0 {
		var ts []map[string]any
		for _, t := range tools {
			ts = append(ts, map[string]any{"type": "function", "function": map[string]any{"name": t.Name, "description": t.Description, "parameters": t.Parameters}})
		}
		req["tools"], req["tool_choice"] = ts, "auto"
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
	hr.Header.Set("Accept", "text/event-stream")
	hr.Header.Set("HTTP-Referer", "https://lamdis.ai")
	hr.Header.Set("X-Title", "Lamdis")
	gone := func(err error) error {
		if stalled.Load() {
			return retryable{fmt.Errorf("the model stopped sending anything for %s", streamIdle)}
		}
		return friendly(err)
	}
	resp, err := o.HTTP.Do(hr)
	if err != nil {
		return Message{}, Usage{}, gone(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		msg := fmt.Sprintf("the model returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
		var e struct {
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(raw, &e) == nil && e.Error.Message != "" {
			msg = e.Error.Message
		}
		// Busy and broken pass; refused and unpaid do not.
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			return Message{}, Usage{}, retryable{fmt.Errorf("%s", msg)}
		}
		return Message{}, Usage{}, fmt.Errorf("%s", msg)
	}
	var content strings.Builder
	var calls []ToolCall
	var usage Usage
	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 4096), 1<<20)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "data:") {
			continue
		}
		data := strings.TrimSpace(strings.TrimPrefix(line, "data:"))
		if data == "[DONE]" {
			break
		}
		var chunk struct {
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
			Choices []struct {
				Delta struct {
					Content          string `json:"content"`
					Reasoning        string `json:"reasoning"`
					ReasoningContent string `json:"reasoning_content"`
					ToolCalls        []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
			} `json:"choices"`
			Usage struct {
				Prompt     int `json:"prompt_tokens"`
				Completion int `json:"completion_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal([]byte(data), &chunk) != nil {
			continue
		}
		// A provider that fails after the stream has started says so in a
		// chunk rather than a status code; without this it reads as an
		// empty answer.
		if chunk.Error != nil {
			return Message{}, usage, retryable{fmt.Errorf("the model failed mid-answer: %s", chunk.Error.Message)}
		}
		if len(chunk.Choices) == 0 {
			continue
		}
		c := chunk.Choices[0]
		if c.Delta.Content != "" || c.Delta.Reasoning != "" || c.Delta.ReasoningContent != "" || len(c.Delta.ToolCalls) > 0 {
			idle.Reset(streamIdle)
		}
		if c.Delta.Content != "" {
			content.WriteString(c.Delta.Content)
			if onText != nil {
				onText(c.Delta.Content)
			}
		}
		for _, tc := range c.Delta.ToolCalls {
			for len(calls) <= tc.Index {
				calls = append(calls, ToolCall{Type: "function"})
			}
			if tc.ID != "" {
				calls[tc.Index].ID = tc.ID
			}
			calls[tc.Index].Function.Name += tc.Function.Name
			calls[tc.Index].Function.Arguments += tc.Function.Arguments
		}
		usage.Prompt, usage.Completion = chunk.Usage.Prompt, chunk.Usage.Completion
	}
	if err := scanner.Err(); err != nil {
		return Message{}, usage, gone(err)
	}
	return Message{Role: "assistant", Content: content.String(), ToolCalls: calls}, usage, nil
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
		// A body cut off part-way is the same transient failure as one
		// that never started, and is retried the same way.
		return Message{}, Usage{}, friendly(err)
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

// ListModels returns the live OpenRouter catalog for the model picker.
func (o *OpenRouter) ListModels(ctx context.Context) ([]string, error) {
	if !o.isOpenRouter() || o.Key == "" {
		return nil, fmt.Errorf("model listing needs an OpenRouter key")
	}
	base := strings.TrimRight(o.BaseURL, "/")
	if base == "" {
		base = openRouterURL
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/models", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+o.Key)
	resp, err := o.HTTP.Do(req)
	if err != nil {
		return nil, friendly(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return nil, fmt.Errorf("OpenRouter returned HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	var out struct {
		Data []struct {
			ID      string `json:"id"`
			Context int    `json:"context_length"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(out.Data))
	for _, m := range out.Data {
		if m.ID != "" {
			ids = append(ids, m.ID)
		}
	}
	sort.Strings(ids)
	return ids, nil
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
