package gemini

import (
	"context"
	"encoding/json"
	"os/exec"
	"regexp"
	"strings"

	"github.com/avi-pathak/mission-control.ai/internal/protocol"
)

// Gemini approval-prompt patterns shown in its TUI when it needs the user to
// confirm running a command or applying an edit.
var approvalPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)allow (this )?(command|execution)`),
	regexp.MustCompile(`(?i)apply (this )?(change|edit|patch)\?`),
	regexp.MustCompile(`(?i)do you want to (run|apply|proceed|allow)`),
	regexp.MustCompile(`(?i)waiting for (user )?confirmation`),
	regexp.MustCompile(`(?im)^\s*❯?\s*1\.\s*yes`),
	regexp.MustCompile(`(?i)\by\s*/\s*n\b`),
	regexp.MustCompile(`(?i)\(y/n\)`),
}

// classifyPaneText decides whether visible terminal text indicates Gemini is
// blocked waiting for approval. Pure and unit-testable.
func classifyPaneText(pane string) protocol.SessionStatus {
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

type sessionItem struct {
	Type string `json:"type"`
}

// classifySessionTail infers status from the tail of a session file. A trailing
// user message with no following gemini reply means Gemini is still working on
// the turn (running). Pure and unit-testable.
func classifySessionTail(lines []string) protocol.SessionStatus {
	for i := len(lines) - 1; i >= 0; i-- {
		var it sessionItem
		if err := json.Unmarshal([]byte(lines[i]), &it); err != nil {
			continue
		}
		switch it.Type {
		case "user", "gemini":
			return protocol.StatusRunning
		}
	}
	return protocol.StatusRunning
}

// detectStatus determines a Gemini session's status: tmux pane scan when
// available (authoritative), else session-tail heuristic.
func (p *Provider) detectStatus(ctx context.Context, s protocol.Session, sessionPath string) protocol.SessionStatus {
	if s.TmuxSession != "" {
		if out, err := exec.CommandContext(ctx, "tmux", "capture-pane", "-t", s.TmuxSession, "-p").Output(); err == nil {
			return classifyPaneText(string(out))
		}
	}
	if sessionPath != "" {
		return classifySessionTail(tailLines(sessionPath, 40))
	}
	return protocol.StatusRunning
}
