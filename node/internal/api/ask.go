package api

// Answering questions about a thread.
//
// The model never sees more than the caller was allowed to read: App.entriesFor
// filters by lane before anything reaches this file. So a share link scoped to
// the summary lane cannot be widened by asking a clever question, which is the
// obvious attack on a feature like this and the reason the filtering lives
// upstream rather than in the prompt.
//
// The instructions below are deliberate about one thing: a thread is a record
// of what people actually wrote, so an answer that goes beyond it is worse than
// no answer. "I cannot see that here" is a correct response and the prompt says
// so plainly rather than hoping.

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"
)

// DefaultModel is cheap, long-context, and good enough to read a thread and
// answer from it. Override with LAMDIS_MODEL; anything OpenRouter serves works,
// and the id is shown in the interface so nobody has to guess what answered.
const DefaultModel = "openai/gpt-5.6-luna"

const askSystem = `You answer questions from one or more shared threads.

Each thread is a record of what people and their agents actually wrote. Entries written by an agent are marked with the agent name. You are
given every entry the person asking is permitted to see, oldest first, each with
a date and an author.

Rules, in order of importance:
1. Answer only from the entries provided. Do not use outside knowledge about
   the people, companies or projects mentioned, even if you think you know them.
2. If the answer is not in the entries, say so in one sentence. "Nothing here
   says" is a good answer. Do not guess, and do not soften a gap into a maybe.
3. Quote or name the entry you are relying on when it matters, so the person can
   go and read it. Refer to entries by their date and author, not by number.
4. Be brief. Two or three sentences unless asked for more.
5. Some entries may contradict each other or be superseded by later ones. Prefer
   the later entry and say that is what you did.

You are reading someone's working record. Say what is there.`

// NewOpenRouterAsk builds the answering function, or nil when no key is set.
//
// Nil rather than an erroring stub on purpose: the interface asks whether it
// can answer and tells the person how to switch it on, which is more use than
// a button that fails when pressed.
func NewOpenRouterAsk(key, model string) func(context.Context, string, []string) (string, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil
	}
	if model == "" {
		model = DefaultModel
	}
	client := &http.Client{Timeout: 90 * time.Second}
	return func(ctx context.Context, question string, entries []string) (string, error) {
		if len(entries) == 0 {
			return "This thread has nothing in it yet, so there is nothing for me to read.", nil
		}
		var sb strings.Builder
		sb.WriteString("Thread entries, oldest first:\n\n")
		for _, e := range entries {
			sb.WriteString(e)
			sb.WriteString("\n")
		}
		sb.WriteString("\nQuestion: ")
		sb.WriteString(question)

		body, err := json.Marshal(map[string]any{
			"model": model,
			"messages": []map[string]string{
				{"role": "system", "content": askSystem},
				{"role": "user", "content": sb.String()},
			},
			"temperature": 0.2,
		})
		if err != nil {
			return "", err
		}
		req, err := http.NewRequestWithContext(ctx, "POST",
			"https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(body))
		if err != nil {
			return "", err
		}
		req.Header.Set("Authorization", "Bearer "+key)
		req.Header.Set("Content-Type", "application/json")
		// OpenRouter attributes traffic with these; harmless and polite.
		req.Header.Set("HTTP-Referer", "https://lamdis.ai")
		req.Header.Set("X-Title", "Lamdis")

		resp, err := client.Do(req)
		if err != nil {
			return "", fmt.Errorf("could not reach the model: %w", err)
		}
		defer resp.Body.Close()
		raw, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		if err != nil {
			return "", err
		}
		if resp.StatusCode != http.StatusOK {
			// Surface the provider's own words: "insufficient credits" is
			// something the person can act on, "HTTP 402" is not.
			var e struct {
				Error struct {
					Message string `json:"message"`
				} `json:"error"`
			}
			if json.Unmarshal(raw, &e) == nil && e.Error.Message != "" {
				return "", fmt.Errorf("%s", e.Error.Message)
			}
			return "", fmt.Errorf("the model returned HTTP %d", resp.StatusCode)
		}
		var out struct {
			Choices []struct {
				Message struct {
					Content string `json:"content"`
				} `json:"message"`
			} `json:"choices"`
		}
		if err := json.Unmarshal(raw, &out); err != nil {
			return "", err
		}
		if len(out.Choices) == 0 || strings.TrimSpace(out.Choices[0].Message.Content) == "" {
			return "", fmt.Errorf("the model returned nothing")
		}
		return strings.TrimSpace(out.Choices[0].Message.Content), nil
	}
}

// AskFromEnv wires the answering function from the environment.
func AskFromEnv() (func(context.Context, string, []string) (string, error), string) {
	model := strings.TrimSpace(os.Getenv("LAMDIS_MODEL"))
	if model == "" {
		model = DefaultModel
	}
	return NewOpenRouterAsk(os.Getenv("LAMDIS_OPENROUTER_KEY"), model), model
}
