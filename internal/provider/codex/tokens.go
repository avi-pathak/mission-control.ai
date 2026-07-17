package codex

import (
	"encoding/json"

	"github.com/avi-pathak/mission-control.ai/internal/protocol"
)

type tokenCountLine struct {
	Type    string `json:"type"`
	Payload struct {
		Type string `json:"type"`
		Info struct {
			TotalTokenUsage struct {
				InputTokens          int64 `json:"input_tokens"`
				CachedInputTokens    int64 `json:"cached_input_tokens"`
				OutputTokens         int64 `json:"output_tokens"`
				ReasoningOutputTokens int64 `json:"reasoning_output_tokens"`
				TotalTokens          int64 `json:"total_tokens"`
			} `json:"total_token_usage"`
		} `json:"info"`
	} `json:"payload"`
}

// tokenUsageFromRollout returns the cumulative token usage from the LAST
// token_count event in a rollout file. Codex reports a running total, so we take
// the most recent one rather than summing. Returns nil if none present.
func tokenUsageFromRollout(path string) *protocol.TokenUsage {
	// Token counts accrue over the session; scan all lines and keep the last.
	lines := tailLines(path, 1_000_000) // effectively the whole file
	var last *tokenCountLine
	for i := len(lines) - 1; i >= 0; i-- {
		var tc tokenCountLine
		if err := json.Unmarshal([]byte(lines[i]), &tc); err != nil {
			continue
		}
		if tc.Type == "event_msg" && tc.Payload.Type == "token_count" {
			last = &tc
			break
		}
	}
	if last == nil {
		return nil
	}
	u := last.Payload.Info.TotalTokenUsage
	// Codex has no cache-creation concept; reasoning tokens are output-side.
	return &protocol.TokenUsage{
		Input:         u.InputTokens,
		Output:        u.OutputTokens + u.ReasoningOutputTokens,
		CacheRead:     u.CachedInputTokens,
		CacheCreation: 0,
		Total:         u.TotalTokens,
	}
}
