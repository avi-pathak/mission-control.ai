package claude

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"regexp"
	"strings"

	"github.com/avi-pathak/mission-control.ai/internal/protocol"
)

// Approval-prompt patterns Claude Code shows when it needs the user to confirm
// an action. Matched case-insensitively against the visible tmux pane.
var approvalPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)do you want to proceed`),
	regexp.MustCompile(`(?i)do you want to (create|make|allow|run|edit|delete)`),
	regexp.MustCompile(`(?i)\bwould you like to proceed`),
	regexp.MustCompile(`(?im)^\s*❯?\s*1\.\s*yes`),        // numbered choice list
	regexp.MustCompile(`(?i)\b1\.\s*yes\b.*\b2\.\s*no\b`), // inline yes/no options
	regexp.MustCompile(`(?i)\(y/n\)`),
	regexp.MustCompile(`(?i)press\s+enter\s+to\s+continue`),
	regexp.MustCompile(`(?i)allow this (tool|command|action)`),
}

// classifyPaneText decides whether visible terminal text indicates the session
// is blocked waiting for approval. Pure and unit-testable.
func classifyPaneText(pane string) protocol.SessionStatus {
	// Only consider the last ~15 non-empty lines (the current prompt area).
	lines := strings.Split(pane, "\n")
	var tail []string
	for i := len(lines) - 1; i >= 0 && len(tail) < 15; i-- {
		if strings.TrimSpace(lines[i]) != "" {
			tail = append(tail, lines[i])
		}
	}
	joined := strings.Join(tail, "\n")
	for _, re := range approvalPatterns {
		if re.MatchString(joined) {
			return protocol.StatusWaitingApproval
		}
	}
	return protocol.StatusRunning
}

// transcriptTailStatus is the shape needed to classify a transcript's last turn.
type ttMsg struct {
	Type    string `json:"type"`
	Message struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	} `json:"message"`
}

// classifyTranscriptTail decides status from the last meaningful transcript
// message. A dangling assistant tool_use (no following user/tool_result) means
// the turn is paused — likely awaiting permission. Pure and unit-testable.
func classifyTranscriptTail(lines []string) protocol.SessionStatus {
	// Walk backwards to the last user/assistant message.
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if line == "" {
			continue
		}
		var m ttMsg
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			continue
		}
		if m.Type == "user" {
			// A user/tool_result followed the assistant → not blocked.
			return protocol.StatusRunning
		}
		if m.Type == "assistant" {
			// If the assistant's last message contains a tool_use, the turn is
			// awaiting the tool result / permission.
			var blocks []struct {
				Type string `json:"type"`
			}
			if err := json.Unmarshal(m.Message.Content, &blocks); err == nil {
				for _, b := range blocks {
					if b.Type == "tool_use" {
						return protocol.StatusWaitingApproval
					}
				}
			}
			return protocol.StatusRunning
		}
	}
	return protocol.StatusRunning
}

// detectStatus determines a session's real status. For tmux-backed sessions it
// scans the live pane (authoritative); otherwise it inspects the transcript
// tail (heuristic). Defaults to running.
func (p *Provider) detectStatus(ctx context.Context, s protocol.Session) protocol.SessionStatus {
	if s.TmuxSession != "" {
		out, err := exec.CommandContext(ctx, "tmux", "capture-pane", "-t", s.TmuxSession, "-p").Output()
		if err == nil {
			return classifyPaneText(string(out))
		}
	}
	if path, ok := transcriptFile(s.CWD); ok {
		if data, err := os.ReadFile(path); err == nil {
			// Only need the tail; split and pass the last N lines.
			lines := strings.Split(string(data), "\n")
			if len(lines) > 40 {
				lines = lines[len(lines)-40:]
			}
			return classifyTranscriptTail(lines)
		}
	}
	return protocol.StatusRunning
}
