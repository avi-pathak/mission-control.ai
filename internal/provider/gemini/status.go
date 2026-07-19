package gemini

import (
	"context"
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

// detectStatus determines a Gemini session's status. Blocked detection requires
// the live tmux pane (the actual prompt); the session-file tail can't reliably
// distinguish "waiting" from normal mid-execution, so non-tmux sessions default
// to running to avoid false "blocked" alerts.
func (p *Provider) detectStatus(ctx context.Context, s protocol.Session, sessionPath string) protocol.SessionStatus {
	_ = sessionPath // no longer used for status; kept for signature stability
	if s.TmuxSession != "" {
		if out, err := exec.CommandContext(ctx, "tmux", "capture-pane", "-t", s.TmuxSession, "-p").Output(); err == nil {
			return classifyPaneText(string(out))
		}
	}
	return protocol.StatusRunning
}
