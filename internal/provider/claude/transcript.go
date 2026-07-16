package claude

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/avi-pathak/mission-control.ai/internal/protocol"
	"github.com/avi-pathak/mission-control.ai/internal/provider"
)

// claudeProjectsDir returns the Claude projects directory. It honors
// MC_CLAUDE_DIR (the path to a `.claude` directory) so a containerized agent
// with host access can point at a mounted host `~/.claude`. Falls back to
// $HOME/.claude.
func claudeProjectsDir() string {
	if d := os.Getenv("MC_CLAUDE_DIR"); d != "" {
		return filepath.Join(d, "projects")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude", "projects")
}

// encodeCWD maps a working directory to Claude Code's project dir name, which
// replaces path separators and dots with dashes
// (e.g. /Users/a/work/x.y -> -Users-a-work-x-y).
func encodeCWD(cwd string) string {
	r := strings.NewReplacer("/", "-", ".", "-")
	return r.Replace(cwd)
}

// transcriptFile finds the newest *.jsonl transcript for a working directory.
// The newest file corresponds to the most recently active session in that cwd.
func transcriptFile(cwd string) (string, bool) {
	base := claudeProjectsDir()
	if base == "" || cwd == "" {
		return "", false
	}
	dir := filepath.Join(base, encodeCWD(cwd))
	entries, err := os.ReadDir(dir)
	if err != nil {
		return "", false
	}
	var newest string
	var newestMod time.Time
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".jsonl") {
			continue
		}
		info, err := e.Info()
		if err != nil {
			continue
		}
		if info.ModTime().After(newestMod) {
			newestMod = info.ModTime()
			newest = filepath.Join(dir, e.Name())
		}
	}
	if newest == "" {
		return "", false
	}
	return newest, true
}

const transcriptBackfillLines = 200

// tailTranscript streams a Claude Code transcript file as rendered log lines.
// It backfills the last N rendered lines, then polls for appended content.
// If the file shrinks (rotation/truncation) the read offset resets.
//
// NOTE: sessions are keyed by PID during discovery but transcripts by cwd, so
// two Claude processes in the same directory tail the same newest transcript.
// That is acceptable for the common one-per-repo case.
func tailTranscript(ctx context.Context, s protocol.Session, path string) <-chan provider.LogLine {
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
			case ch <- provider.LogLine{
				SessionID: s.ID,
				Stream:    protocol.StreamStdout,
				Line:      line,
				TS:        time.Now().UnixMilli(),
			}:
				return true
			case <-ctx.Done():
				return false
			}
		}

		// Backfill: emit the tail of the existing file.
		var offset int64
		if rendered, end := renderTail(path, transcriptBackfillLines); end >= 0 {
			offset = end
			for _, l := range rendered {
				if !emit(l) {
					return
				}
			}
		}

		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				info, err := os.Stat(path)
				if err != nil {
					continue
				}
				size := info.Size()
				if size < offset {
					offset = 0 // rotated/truncated
				}
				if size == offset {
					continue
				}
				lines, end := readFrom(path, offset)
				offset = end
				for _, raw := range lines {
					if l := renderTranscriptLine(raw); l != "" {
						if !emit(l) {
							return
						}
					}
				}
			}
		}
	}()
	return ch
}

// readFrom reads complete newline-terminated lines starting at byte offset and
// returns them plus the new offset (end of the last complete line).
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
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1024*1024), 8*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		lines = append(lines, line)
		consumed += int64(len(line)) + 1 // +1 for the newline
	}
	return lines, consumed
}

// renderTail reads the whole file, renders each JSON line, and returns the last
// n non-empty rendered lines plus the file's end offset.
func renderTail(path string, n int) ([]string, int64) {
	f, err := os.Open(path)
	if err != nil {
		return nil, -1
	}
	defer f.Close()
	var rendered []string
	var end int64
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 1024*1024), 8*1024*1024)
	for sc.Scan() {
		line := sc.Text()
		end += int64(len(line)) + 1
		if l := renderTranscriptLine(line); l != "" {
			rendered = append(rendered, l)
		}
	}
	if len(rendered) > n {
		rendered = rendered[len(rendered)-n:]
	}
	return rendered, end
}

// --- rendering ---

type transcriptMessage struct {
	Type    string          `json:"type"`
	Message json.RawMessage `json:"message"`
}

type innerMessage struct {
	Role    string          `json:"role"`
	Content json.RawMessage `json:"content"`
}

type contentBlock struct {
	Type  string          `json:"type"`
	Text  string          `json:"text"`
	Name  string          `json:"name"`
	Input json.RawMessage `json:"input"`
}

// ANSI colors reused by the dashboard's ansiToHtml and xterm.
const (
	ansiReset  = "\x1b[0m"
	ansiGreen  = "\x1b[32m"
	ansiCyan   = "\x1b[36m"
	ansiYellow = "\x1b[33m"
	ansiGray   = "\x1b[90m"
)

// renderTranscriptLine converts one transcript JSON line into a readable,
// ANSI-colored log line. Returns "" for lines that should be skipped.
func renderTranscriptLine(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	var m transcriptMessage
	if err := json.Unmarshal([]byte(raw), &m); err != nil {
		return ""
	}
	switch m.Type {
	case "user", "assistant":
		return renderChat(m)
	default:
		// mode, permission-mode, file-history-snapshot, summary, etc. → skip.
		return ""
	}
}

func renderChat(m transcriptMessage) string {
	var inner innerMessage
	if err := json.Unmarshal(m.Message, &inner); err != nil {
		return ""
	}
	role := inner.Role
	if role == "" {
		role = m.Type
	}

	// content may be a plain string or an array of blocks.
	var asString string
	if err := json.Unmarshal(inner.Content, &asString); err == nil {
		return chatLine(role, asString)
	}
	var blocks []contentBlock
	if err := json.Unmarshal(inner.Content, &blocks); err != nil {
		return ""
	}
	var parts []string
	for _, b := range blocks {
		switch b.Type {
		case "text":
			if t := strings.TrimSpace(b.Text); t != "" {
				parts = append(parts, chatLine(role, t))
			}
		case "tool_use":
			input := compact(string(b.Input), 120)
			parts = append(parts, ansiYellow+"⚙ tool: "+b.Name+ansiReset+" "+ansiGray+input+ansiReset)
		case "tool_result":
			// tool results are often verbose; summarize.
			parts = append(parts, ansiGray+"  ↳ (tool result)"+ansiReset)
		}
	}
	return strings.Join(parts, "\n")
}

func chatLine(role, text string) string {
	text = compact(text, 2000)
	switch role {
	case "user":
		return ansiGreen + "▸ user: " + ansiReset + text
	case "assistant":
		return ansiCyan + "● assistant: " + ansiReset + text
	default:
		return "  " + text
	}
}

// compact collapses whitespace and truncates to max runes.
func compact(s string, max int) string {
	s = strings.Join(strings.Fields(s), " ")
	r := []rune(s)
	if len(r) > max {
		return string(r[:max]) + "…"
	}
	return s
}
