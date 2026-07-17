package codex

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
	ansiReset  = "\x1b[0m"
	ansiGreen  = "\x1b[32m"
	ansiCyan   = "\x1b[36m"
	ansiYellow = "\x1b[33m"
	ansiGray   = "\x1b[90m"
)

// renderRolloutLine converts one rollout JSON line into a readable, ANSI-colored
// log line. Returns "" for lines that should be skipped. Pure/testable.
func renderRolloutLine(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var top struct {
		Type    string          `json:"type"`
		Payload json.RawMessage `json:"payload"`
	}
	if err := json.Unmarshal([]byte(raw), &top); err != nil {
		return ""
	}
	switch top.Type {
	case "event_msg":
		var p struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		}
		_ = json.Unmarshal(top.Payload, &p)
		switch p.Type {
		case "user_message":
			return ansiGreen + "▸ user: " + ansiReset + compact(p.Message, 2000)
		case "agent_message":
			return ansiCyan + "● codex: " + ansiReset + compact(p.Message, 2000)
		}
	case "response_item":
		var p struct {
			Type string `json:"type"`
			Name string `json:"name"`
		}
		_ = json.Unmarshal(top.Payload, &p)
		switch p.Type {
		case "function_call", "custom_tool_call":
			name := p.Name
			if name == "" {
				name = "tool"
			}
			return ansiYellow + "⚙ tool: " + name + ansiReset
		case "function_call_output", "custom_tool_call_output":
			return ansiGray + "  ↳ (tool result)" + ansiReset
		}
	}
	return ""
}

// tailRollout streams a rollout file as rendered log lines: backfill the tail,
// then poll for appended content. Mirrors the Claude transcript tail pattern.
func tailRollout(ctx context.Context, s protocol.Session, path string) <-chan provider.LogLine {
	ch := make(chan provider.LogLine, 128)
	go func() {
		defer close(ch)
		var seq int64
		emit := func(line string) bool {
			if line == "" {
				return true
			}
			seq++
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
			if !emit(renderRolloutLine(l)) {
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
					if !emit(renderRolloutLine(raw)) {
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
	// Read line by line.
	buf := make([]byte, 0, 64*1024)
	tmp := make([]byte, 32*1024)
	data := buf
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
		line := string(data[:idx])
		lines = append(lines, line)
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
