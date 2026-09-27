package agent

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestAttachmentsReachTheModel(t *testing.T) {
	m := Message{Role: "user", Content: "which grout?"}
	plain, _ := json.Marshal(m)
	if !strings.Contains(string(plain), `"content":"which grout?"`) {
		t.Fatalf("plain message changed shape: %s", plain)
	}
	m.parts = []part{{Type: "image_url", ImageURL: map[string]any{"url": "data:image/png;base64,AAAA"}}}
	raw, _ := json.Marshal(m)
	if !strings.Contains(string(raw), `"content":[{"type":"text","text":"which grout?"},{"type":"image_url"`) {
		t.Fatalf("parts: %s", raw)
	}
}
