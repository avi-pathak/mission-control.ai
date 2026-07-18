package gemini

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"time"

	"github.com/avi-pathak/mission-control.ai/internal/protocol"
	"github.com/avi-pathak/mission-control.ai/internal/provider"
)

const (
	ansiReset = "\x1b[0m"
	ansiGreen = "\x1b[32m"
	ansiCyan  = "\x1b[36m"
	ansiGray  = "\x1b[90m"
)

// renderSessionLine converts one session JSONL line into a readable, ANSI-colored
// log line. Returns "" for lines that should be skipped. Pure/testable.
//
// Gemini user content is [{text}]; gemini content is a plain string.
func renderSessionLine(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var top struct {
		Type    string          `json:"type"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal([]byte(raw), &top); err != nil {
		return ""
	}
	switch top.Type {
	case "user":
		var parts []struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(top.Content, &parts); err == nil {
			var sb strings.Builder
			for _, pt := range parts {
				sb.WriteString(pt.Text)
			}
			msg := compact(sb.String(), 2000)
			// Skip the synthetic session_context bootstrap message.
			if strings.HasPrefix(msg, "<session_context>") {
				return ""
			}
			return ansiGreen + "▸ user: " + ansiReset + msg
		}
	case "gemini":
		var text string
		if err := json.Unmarshal(top.Content, &text); err == nil {
			return ansiCyan + "● gemini: " + ansiReset + compact(text, 2000)
		}
	}
	return ""
}

// tailSession streams a session file as rendered log lines: backfill the tail,
// then poll for appended content. Mirrors the Codex rollout tail pattern.
func tailSession(ctx context.Context, s protocol.Session, path string) <-chan provider.LogLine {
	ch := make(chan provider.LogLine, 128)
	go func() {
		defer close(ch)
		emit := func(line string) bool {
			if line == "" {
				return true
			}
			select {
			case ch <- provider.LogLine{SessionID: s.ID, Stream: protocol.StreamStdout, Line: line, TS: time.Now().UnixMilli()}:
				return true
			case <-ctx.Done():
				return false
			}
		}

		// Backfill last 200 rendered lines + record byte offset.
		var offset int64
		for _, l := range tailLines(path, 200) {
			if !emit(renderSessionLine(l)) {
				return
			}
		}
		if fi, err := os.Stat(path); err == nil {
			offset = fi.Size()
		}

		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				fi, err := os.Stat(path)
				if err != nil {
					continue
				}
				if fi.Size() < offset {
					offset = 0 // rotated
				}
				if fi.Size() == offset {
					continue
				}
				lines, end := readFrom(path, offset)
				offset = end
				for _, raw := range lines {
					if !emit(renderSessionLine(raw)) {
						return
					}
				}
			}
		}
	}()
	return ch
}

// readFrom reads complete newline-terminated lines from byte offset, returning
// them plus the new offset.
func readFrom(path string, offset int64) ([]string, int64) {
	f, err := os.Open(path)
	if err != nil {
		return nil, offset
	}
	defer f.Close()
	if _, err := f.Seek(offset, 0); err != nil {
		return nil, offset
	}
	var lines []string
	consumed := offset
	var data []byte
	tmp := make([]byte, 32*1024)
	for {
		n, rerr := f.Read(tmp)
		data = append(data, tmp[:n]...)
		if rerr != nil {
			break
		}
	}
	for {
		idx := indexByte(data, '\n')
		if idx < 0 {
			break
		}
		lines = append(lines, string(data[:idx]))
		consumed += int64(idx) + 1
		data = data[idx+1:]
	}
	return lines, consumed
}

func indexByte(b []byte, c byte) int {
	for i := range b {
		if b[i] == c {
			return i
		}
	}
	return -1
}

func compact(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > max {
		return string(r[:max]) + "…"
	}
	return s
}
