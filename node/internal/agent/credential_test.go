package agent

import "testing"

// A key belongs to the service it was issued for. Pointing the agent at a
// private or local model server must never hand that server the key the
// person pays OpenRouter with.
func TestModelKeyNeverLeavesItsProvider(t *testing.T) {
	cases := []struct {
		base  string
		forIt bool
		send  bool
	}{
		{"", false, true}, // OpenRouter, the default
		{"https://openrouter.ai/api/v1", false, true},           // OpenRouter, spelled out
		{"https://openrouter.ai/api/v1/", false, true},          // trailing slash
		{"http://localhost:11434/v1", false, false},             // a local model
		{"https://evil.example/v1", false, false},               // somebody else's endpoint
		{"https://openrouter.ai.evil.example/v1", false, false}, // a lookalike host
		{"https://vllm.internal/v1", true, true},                // a key set for that endpoint
	}
	for _, c := range cases {
		o := &OpenRouter{Key: "sk-or-secret", BaseURL: c.base, KeyIsForBaseURL: c.forIt}
		if got := o.keyGoesHere(); got != c.send {
			t.Errorf("base %q (own key %v): sending the key = %v, want %v", c.base, c.forIt, got, c.send)
		}
	}
}

// Switching to a local model must swap the credential, not carry the
// OpenRouter one across.
func TestLocalModelGetsNoInheritedKey(t *testing.T) {
	dir := t.TempDir()
	r := &Runner{DataDir: dir, Model: &OpenRouter{Key: "sk-or-secret", Model: "openai/gpt-5.6-luna"}}
	m, _ := r.ModelFor(Config{ModelURL: "http://localhost:11434/v1", Model: "qwen3.5:4b"})
	or, ok := m.(*OpenRouter)
	if !ok {
		t.Fatal("no client")
	}
	if or.Key != "" || or.keyGoesHere() && or.Key != "" {
		t.Fatalf("the OpenRouter key followed the person to a local server: %q", or.Key)
	}
	if or.Endpoint() != "http://localhost:11434/v1/chat/completions" {
		t.Fatalf("endpoint: %s", or.Endpoint())
	}
	// A key set for that endpoint is used, and only there.
	m, _ = r.ModelFor(Config{ModelURL: "https://vllm.internal/v1", ModelURLKey: "private-token"})
	or = m.(*OpenRouter)
	if or.Key != "private-token" || !or.keyGoesHere() {
		t.Fatalf("endpoint key not used: %+v", or)
	}
}
