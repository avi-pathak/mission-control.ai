package claude

import (
	"bufio"
	"encoding/json"
	"os"

	"github.com/avi-pathak/mission-control.ai/internal/protocol"
)

// tokenLine is the minimal shape we parse from a transcript line to extract
// Claude token usage.
type tokenLine struct {
	Message struct {
		Usage struct {
			InputTokens              int64 `json:"input_tokens"`
			OutputTokens             int64 `json:"output_tokens"`
			CacheReadInputTokens     int64 `json:"cache_read_input_tokens"`
			CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
		} `json:"usage"`
	} `json:"message"`
}

// tokenUsageForCWD sums token usage across the newest transcript for a working
// directory. Input/output are summed across turns; cache-read/creation are also
// summed (Claude reports per-turn values). Returns nil if no transcript exists.
func tokenUsageForCWD(cwd string) *protocol.TokenUsage {
	path, ok := transcriptFile(cwd)
	if !ok {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()

	var u protocol.TokenUsage
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1024*1024), 8*1024*1024)
	seen := false
	for sc.Scan() {
		var tl tokenLine
		if err := json.Unmarshal(sc.Bytes(), &tl); err != nil {
			continue
		}
		us := tl.Message.Usage
		if us.InputTokens == 0 && us.OutputTokens == 0 &&
			us.CacheReadInputTokens == 0 && us.CacheCreationInputTokens == 0 {
			continue
		}
		seen = true
		u.Input += us.InputTokens
		u.Output += us.OutputTokens
		u.CacheRead += us.CacheReadInputTokens
		u.CacheCreation += us.CacheCreationInputTokens
	}
	if !seen {
		return nil
	}
	u.Total = u.Input + u.Output + u.CacheRead + u.CacheCreation
	return &u
}
