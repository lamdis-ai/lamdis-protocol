package agent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func answering(reply string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]any{"content": reply}}},
			"usage":   map[string]int{"prompt_tokens": 10, "completion_tokens": 3},
		})
	}
}

func client(url string) *OpenRouter {
	return &OpenRouter{Key: "k", Model: "m", BaseURL: url,
		HTTP: &http.Client{Timeout: 5 * time.Second}}
}

// A connection that drops once is not news, and nobody should hear about it.
func TestABlipIsRiddenOutRatherThanReported(t *testing.T) {
	var calls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&calls, 1) == 1 {
			// Hang up mid-response, which is what a dropped connection is.
			if hj, ok := w.(http.Hijacker); ok {
				c, _, _ := hj.Hijack()
				c.Close()
				return
			}
		}
		answering("two quotes are open")(w, r)
	}))
	defer srv.Close()

	m, _, err := client(srv.URL).Complete(context.Background(), []Message{{Role: "user", Content: "hey"}}, nil)
	if err != nil {
		t.Fatalf("a single dropped connection reached the person: %v", err)
	}
	if m.Content != "two quotes are open" {
		t.Fatalf("answer: %q", m.Content)
	}
	if calls < 2 {
		t.Fatalf("it did not try again: %d calls", calls)
	}
}

// Being busy passes. Being refused does not, and trying four times would
// only waste somebody's minute.
func TestBusyIsRetriedAndRefusedIsNot(t *testing.T) {
	var busy int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if atomic.AddInt32(&busy, 1) <= 2 {
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": "slow down"}})
			return
		}
		answering("done")(w, r)
	}))
	defer srv.Close()
	if _, _, err := client(srv.URL).Complete(context.Background(), []Message{{Content: "x"}}, nil); err != nil {
		t.Fatalf("a busy provider should be waited out: %v", err)
	}

	var tries int32
	refuse := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&tries, 1)
		w.WriteHeader(http.StatusPaymentRequired)
		json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"message": providerCreditWords}})
	}))
	defer refuse.Close()
	_, _, err := client(refuse.URL).Complete(context.Background(), []Message{{Content: "x"}}, nil)
	if !errors.As(err, new(OutOfCredit)) {
		t.Fatalf("running out of credit should be recognised: %v", err)
	}
	if strings.Contains(err.Error(), "openrouter.ai/workspaces") || strings.Contains(err.Error(), "9717d") {
		t.Fatalf("the key's dashboard link reached the person: %v", err)
	}
	if tries != 1 {
		t.Fatalf("a refusal was retried %d times", tries)
	}
}

// Whatever does reach somebody should be a sentence, not a stack of Go.
func TestFailuresAreReadable(t *testing.T) {
	// Nothing listening at all.
	_, _, err := client("https://127.0.0.1:1").Complete(context.Background(), []Message{{Content: "x"}}, nil)
	if err == nil {
		t.Fatal("expected a failure")
	}
	for _, ugly := range []string{"net/http", "dial tcp", "x509", "TLS handshake", "EOF"} {
		if strings.Contains(err.Error(), ugly) {
			t.Fatalf("a person was shown %q: %v", ugly, err)
		}
	}
	if !strings.Contains(err.Error(), "connection") && !strings.Contains(err.Error(), "reach") {
		t.Fatalf("that will not tell anybody anything: %v", err)
	}
}

// Giving up has to be quick when somebody is waiting.
func TestItGivesUpRatherThanHangingOn(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hj, ok := w.(http.Hijacker); ok {
			c, _, _ := hj.Hijack()
			c.Close()
		}
	}))
	defer srv.Close()
	start := time.Now()
	if _, _, err := client(srv.URL).Complete(context.Background(), []Message{{Content: "x"}}, nil); err == nil {
		t.Fatal("expected it to give up")
	}
	if d := time.Since(start); d > 20*time.Second {
		t.Fatalf("it held on for %s", d.Round(time.Second))
	}
}

// What OpenRouter said on 2026-10-19 when the host key ran dry: it names the
// key's dashboard, which belongs to whoever runs the host.
const providerCreditWords = "This request requires more credits, or fewer max_tokens. You requested up to 65536 tokens, but can only afford 49434. To increase, visit https://openrouter.ai/workspaces/default/keys/9717d9994a78e29ea09e8fc7a0b99edcc62d75321 and adjust the key's total limit."

func TestRunningDryEvenMidStreamSaysWhatToDo(t *testing.T) {
	// Some providers report it inside a 200 stream; the wording is the tell.
	if !errors.As(creditError(200, providerCreditWords), new(OutOfCredit)) {
		t.Fatal("credit wording inside a stream was not recognised")
	}
	if creditError(400, "tool schema invalid") != nil {
		t.Fatal("an ordinary refusal was taken for running out of credit")
	}
	shared, own := (&Runner{}).creditNote(Config{}), (&Runner{}).creditNote(Config{OpenRouterKey: "sk-or-x"})
	if !strings.Contains(shared, "openrouter.ai/keys") || !strings.Contains(shared, "Settings") {
		t.Fatalf("someone on the host's key should be told how to bring their own: %q", shared)
	}
	if strings.Contains(own, "free") || !strings.Contains(own, "Add credit") {
		t.Fatalf("someone on their own key should be told to top it up: %q", own)
	}
}
