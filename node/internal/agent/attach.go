package agent

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	protolog "github.com/lamdis-ai/lamdis-protocol/node/internal/log"
)

// Attachment is a file on a message, as the message records it.
type Attachment struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Type string `json:"type"`
	Size int64  `json:"size"`
	URL  string `json:"url"`
}

// part is one piece of a multi-part message (OpenAI/OpenRouter shape).
type part struct {
	Type     string         `json:"type"`
	Text     string         `json:"text,omitempty"`
	ImageURL map[string]any `json:"image_url,omitempty"`
	File     map[string]any `json:"file,omitempty"`
}

// MarshalJSON sends a message with attachments as parts, and any other
// message exactly as before.
func (m Message) MarshalJSON() ([]byte, error) {
	type plain Message
	if len(m.parts) == 0 {
		return json.Marshal(plain(m))
	}
	ps := append([]part{{Type: "text", Text: m.Content}}, m.parts...)
	return json.Marshal(struct {
		Role    string `json:"role"`
		Content []part `json:"content"`
	}{m.Role, ps})
}

func sizeWords(n int64) string {
	switch {
	case n >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(n)/(1<<20))
	case n >= 1<<10:
		return fmt.Sprintf("%d KB", n>>10)
	}
	return fmt.Sprintf("%d bytes", n)
}

var attachExt = map[string]string{
	"image/png": ".png", "image/jpeg": ".jpg", "image/gif": ".gif", "image/webp": ".webp",
	"application/pdf": ".pdf", "text/plain": ".txt", "text/markdown": ".md", "text/csv": ".csv",
	"application/json": ".json",
}

// openAttachment reads a file's bytes: through the host when it lives in
// another account there, else from this account's own files.
func (r *Runner) openAttachment(a Attachment) ([]byte, error) {
	if r.OpenFile != nil {
		if b, err := r.OpenFile(a.URL, a.ID); err == nil {
			return b, nil
		}
	}
	ext, ok := attachExt[a.Type]
	if !ok || len(a.ID) != 32 || strings.ContainsAny(a.ID, "./\\") {
		return nil, fmt.Errorf("unknown file")
	}
	return os.ReadFile(filepath.Join(r.DataDir, "files", a.ID+ext))
}

const (
	maxAttachParts = 4
	maxAttachBytes = 12 << 20
	maxTextChars   = 20000
)

// attachRecent adds the files on the triggering entry, then the most recent
// ones in the thread, to the user message, within a small budget.
func (r *Runner) attachRecent(m *Message, tl *protolog.ThreadLog, trig *protolog.Entry) {
	var entries []*protolog.Entry
	if trig != nil {
		entries = append(entries, trig)
	}
	all := tl.Entries()
	for i := len(all) - 1; i >= 0 && len(all)-i <= 30; i-- {
		if trig == nil || all[i].ID != trig.ID {
			entries = append(entries, all[i])
		}
	}
	seen := map[string]bool{}
	total := 0
	for _, e := range entries {
		var b struct {
			Files []Attachment `json:"files"`
		}
		if json.Unmarshal(e.Body, &b) != nil {
			continue
		}
		for _, f := range b.Files {
			if seen[f.ID] || len(m.parts) >= maxAttachParts {
				continue
			}
			seen[f.ID] = true
			data, err := r.openAttachment(f)
			if err != nil || total+len(data) > maxAttachBytes {
				continue
			}
			total += len(data)
			label := "Attached: " + f.Name
			switch {
			case strings.HasPrefix(f.Type, "image/"):
				m.parts = append(m.parts, part{Type: "text", Text: label},
					part{Type: "image_url", ImageURL: map[string]any{"url": "data:" + f.Type + ";base64," + base64.StdEncoding.EncodeToString(data)}})
			case f.Type == "application/pdf":
				m.parts = append(m.parts, part{Type: "file", File: map[string]any{"filename": f.Name,
					"file_data": "data:application/pdf;base64," + base64.StdEncoding.EncodeToString(data)}})
			default:
				text := string(data)
				if !utf8.ValidString(text) {
					continue
				}
				if len(text) > maxTextChars {
					text = text[:maxTextChars] + "\n…(truncated)"
				}
				m.parts = append(m.parts, part{Type: "text", Text: untrusted("file:"+f.Name, text)})
			}
		}
	}
}
