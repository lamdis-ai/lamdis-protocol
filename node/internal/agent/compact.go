package agent

import (
	"encoding/json"
	"fmt"
	"strings"
)

// Compaction keeps working state, not the conversation.
//
// A long task's transcript is mostly tool output the model has already
// acted on: whole files it read, four hundred lines of a test run. Past a
// size, the older turns are replaced by one message: the harness's task
// state and a line or two per old tool call (what was read, what a command
// returned and which tests failed). The recent turns stay verbatim. It is
// deterministic, so it costs no model call and cannot hallucinate.

// transcriptSize is roughly how much the model is being sent, in characters.
func transcriptSize(msgs []Message) int {
	n := 0
	for _, m := range msgs {
		n += len(m.Content)
		for _, tc := range m.ToolCalls {
			n += len(tc.Function.Arguments) + len(tc.Function.Name)
		}
	}
	return n
}

// compactMessages keeps everything before the first assistant turn (the
// system prompt and the task) and the last keep assistant turns with their
// tool results, and distils whatever lies between.
func compactMessages(msgs []Message, keep int, state string) ([]Message, bool) {
	first := -1
	var turns []int // indexes of assistant messages
	for i, m := range msgs {
		if m.Role == "assistant" {
			if first < 0 {
				first = i
			}
			turns = append(turns, i)
		}
	}
	if first < 0 || len(turns) <= keep {
		return msgs, false
	}
	cut := turns[len(turns)-keep] // first assistant message kept verbatim
	old := msgs[first:cut]
	calls := map[string]ToolCall{}
	var sb strings.Builder
	sb.WriteString("Earlier turns of this task were compacted by the harness to save context. What they established is below; re-read a file if you need its exact text.\n\n")
	sb.WriteString(state)
	sb.WriteString("\nEarlier steps, oldest first:\n")
	for _, m := range old {
		switch m.Role {
		case "assistant":
			if t := strings.TrimSpace(m.Content); t != "" {
				sb.WriteString("- you noted: " + trunc(oneLine(t), 300) + "\n")
			}
			for _, tc := range m.ToolCalls {
				calls[tc.ID] = tc
			}
		case "tool":
			tc, ok := calls[m.ToolCallID]
			if !ok {
				continue
			}
			var args map[string]any
			json.Unmarshal([]byte(tc.Function.Arguments), &args)
			sb.WriteString("- " + observation(humanName(tc.Function.Name), args, m.Content) + "\n")
		case "user":
			// The person's words are never summarised away.
			sb.WriteString("- message: " + trunc(m.Content, 2000) + "\n")
		}
	}
	out := append([]Message(nil), msgs[:first]...)
	out = append(out, Message{Role: "user", Content: sb.String()})
	out = append(out, msgs[cut:]...)
	return out, true
}

// observation is one old tool call, distilled to what it established.
func observation(name string, args map[string]any, out string) string {
	s := func(k string) string {
		v, _ := args[k].(string)
		return v
	}
	failed := strings.HasPrefix(out, "error:") || strings.HasPrefix(out, "refused:") || strings.HasPrefix(out, "Not executed")
	if failed {
		return fmt.Sprintf("%s %s -> %s", name, trunc(jsonString(args), 200), trunc(oneLine(out), 200))
	}
	switch name {
	case "read_file":
		lines := strings.Count(out, "\n")
		rng := ""
		if args["start"] != nil || args["end"] != nil {
			rng = fmt.Sprintf(" lines %v-%v", args["start"], args["end"])
		}
		return fmt.Sprintf("read %s%s (%d lines)", s("path"), rng, lines)
	case "edit_file", "write_file":
		return trunc(oneLine(out), 200)
	case "run", "run_tests", "run_checks":
		head := strings.SplitN(out, "\n", 3)
		status := strings.Join(head[:min(2, len(head))], " ")
		var fails []string
		if strings.Contains(out, "exit 0") && !strings.Contains(out, "failed") {
			return trunc(status, 300)
		}
		fails = failureLines(out, 6)
		return trunc(status, 300) + "\n    " + strings.Join(fails, "\n    ")
	case "explore":
		return "explore " + trunc(s("task"), 160) + " found:\n    " + trunc(strings.ReplaceAll(out, "\n", "\n    "), 1500)
	case "search_files", "find_symbol", "find_references", "find_tests", "list_files":
		lines := strings.Split(strings.TrimSpace(out), "\n")
		shown := lines
		if len(shown) > 5 {
			shown = shown[:5]
		}
		return fmt.Sprintf("%s %s -> %d results: %s", name, trunc(jsonString(args), 160), len(lines), trunc(strings.Join(shown, "; "), 500))
	}
	return fmt.Sprintf("%s %s -> %s", name, trunc(jsonString(args), 160), trunc(oneLine(out), 300))
}

func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}
