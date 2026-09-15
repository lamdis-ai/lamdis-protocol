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
	return &OpenRouter{Key: key, Model: model, HTTP: &http.Client{Timeout: 120 * time.Second}}
}

func (o *OpenRouter) Complete(ctx context.Context, msgs []Message, tools []ToolSpec) (Message, Usage, error) {
	req := map[string]any{"model": o.Model, "messages": msgs, "temperature": 0.2}
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
		return Message{}, Usage{}, fmt.Errorf("could not reach the model: %w", err)
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
		if json.Unmarshal(raw, &e) == nil && e.Error.Message != "" {
			return Message{}, Usage{}, fmt.Errorf("%s", e.Error.Message)
		}
		return Message{}, Usage{}, fmt.Errorf("the model returned HTTP %d", resp.StatusCode)
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
