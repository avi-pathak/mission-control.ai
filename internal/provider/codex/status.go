package codex

import (
	"context"
	"os/exec"
	"regexp"
	"strings"

	"github.com/avi-pathak/mission-control.ai/internal/protocol"
)

// Codex approval-prompt patterns shown in its TUI when it needs the user to
// confirm running a command or applying a patch.
var approvalPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)allow (this )?command`),
	regexp.MustCompile(`(?i)approve (this )?(command|patch|edit|action)`),
	regexp.MustCompile(`(?i)do you want to (run|apply|proceed)`),
	regexp.MustCompile(`(?im)^\s*❯?\s*1\.\s*yes`),
	regexp.MustCompile(`(?i)\by\s*/\s*n\b`),
	regexp.MustCompile(`(?i)\(y/n\)`),
	regexp.MustCompile(`(?i)requires approval`),
}

// classifyPaneText decides whether visible terminal text indicates Codex is
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

// detectStatus determines a Codex session's status. Blocked ("waiting_approval")
// detection requires the live tmux pane (the actual approval prompt); a dangling
// function_call in the rollout is indistinguishable from normal mid-execution,
// so non-tmux sessions default to running to avoid false "blocked" alerts.
func (p *Provider) detectStatus(ctx context.Context, s protocol.Session, rolloutPath string) protocol.SessionStatus {
	_ = rolloutPath // no longer used for status; kept for signature stability
	if s.TmuxSession != "" {
		if out, err := exec.CommandContext(ctx, "tmux", "capture-pane", "-t", s.TmuxSession, "-p").Output(); err == nil {
			return classifyPaneText(string(out))
		}
	}
	return protocol.StatusRunning
}
