package gemini

import (
	"encoding/json"

	"github.com/avi-pathak/mission-control.ai/internal/protocol"
)

// geminiTokenLine is a "gemini"-type message carrying a cumulative token count.
// The Gemini CLI reports a running total per assistant turn, so we take the
// most recent one rather than summing.
type geminiTokenLine struct {
	Type   string `json:"type"`
	Tokens struct {
		Input    int64 `json:"input"`
		Output   int64 `json:"output"`
		Cached   int64 `json:"cached"`
		Thoughts int64 `json:"thoughts"`
		Tool     int64 `json:"tool"`
		Total    int64 `json:"total"`
	} `json:"tokens"`
}

// tokenUsageFromSession returns the cumulative token usage from the LAST gemini
// message that carries a token count. Returns nil if none present.
func tokenUsageFromSession(path string) *protocol.TokenUsage {
	lines := tailLines(path, 1_000_000) // effectively the whole file
	for i := len(lines) - 1; i >= 0; i-- {
		var tc geminiTokenLine
		if err := json.Unmarshal([]byte(lines[i]), &tc); err != nil {
			continue
		}
		if tc.Type != "gemini" || tc.Tokens.Total == 0 {
			continue
		}
		// Gemini has no cache-creation concept; thinking tokens are output-side.
		return &protocol.TokenUsage{
			Input:         tc.Tokens.Input,
			Output:        tc.Tokens.Output + tc.Tokens.Thoughts,
			CacheRead:     tc.Tokens.Cached,
			CacheCreation: 0,
			Total:         tc.Tokens.Total,
		}
	}
	return nil
}
