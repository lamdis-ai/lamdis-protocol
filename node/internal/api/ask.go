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
	"context"
	"os"
	"strings"

	"github.com/lamdis-ai/lamdis-protocol/node/internal/agent"
)

// DefaultModel is cheap, long-context, and good enough to read a thread and
// answer from it. Override with LAMDIS_MODEL; anything OpenRouter serves works,
// and the id is shown in the interface so nobody has to guess what answered.
const DefaultModel = agent.DefaultModel

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
// can answer and tells the person how to switch it on. This is the no-tools
// path the share sheet uses to draft summaries; chat goes through the agent.
func NewOpenRouterAsk(key, model string) func(context.Context, string, []string) (string, error) {
	m := agent.NewOpenRouter(key, model)
	if m == nil {
		return nil
	}
	return func(ctx context.Context, question string, entries []string) (string, error) {
		return AskWith(ctx, m, question, entries)
	}
}

// AskWith answers one question from entries using any Model, no tools.
func AskWith(ctx context.Context, m agent.Model, question string, entries []string) (string, error) {
	{
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
		out, _, err := m.Complete(ctx, []agent.Message{{Role: "system", Content: askSystem}, {Role: "user", Content: sb.String()}}, nil)
		if err != nil {
			return "", err
		}
		return strings.TrimSpace(out.Content), nil
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
