package agent

import (
	"time"
)

// This explicit allowlist deliberately excludes shell, browser, MCP, writes,
// approvals and search_context (which may run the embedding indexer).
func parallelRead(name string) bool {
	switch name {
	case "read_file", "list_files", "search_files", "where", "list_threads", "read_thread", "fetch_url", "explore":
		return true
	}
	return codeReadOnly[name]
}

type toolResult struct {
	out, outcome string
	done         bool
	rec          runRec
	took         time.Duration
}

func mergeRunRec(dst *runRec, src runRec) {
	for _, id := range src.Threads {
		if !contains(dst.Threads, id) {
			dst.Threads = append(dst.Threads, id)
		}
	}
	dst.ToolCalls = append(dst.ToolCalls, src.ToolCalls...)
	dst.Fetches = append(dst.Fetches, src.Fetches...)
	dst.External = append(dst.External, src.External...)
	dst.Outputs = append(dst.Outputs, src.Outputs...)
	for k, v := range src.Tokens {
		dst.Tokens[k] += v
	}
}

// closeToolCalls supplies skipped results after a barrier ends execution.
func closeToolCalls(msgs []Message, reason string) []Message {
	last := -1
	for i, m := range msgs {
		if m.Role == "assistant" && len(m.ToolCalls) > 0 {
			last = i
		}
	}
	if last < 0 {
		return msgs
	}
	seen := map[string]bool{}
	for _, m := range msgs[last+1:] {
		if m.Role == "tool" {
			seen[m.ToolCallID] = true
		}
	}
	for _, tc := range msgs[last].ToolCalls {
		if !seen[tc.ID] {
			msgs = append(msgs, Message{Role: "tool", ToolCallID: tc.ID, Content: "Not executed: " + reason})
		}
	}
	return msgs
}
