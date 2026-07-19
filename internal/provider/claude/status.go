package claude

import (
	"context"
	"os/exec"
	"regexp"
	"strings"

	"github.com/avi-pathak/mission-control.ai/internal/protocol"
)

// Approval-prompt patterns Claude Code shows when it needs the user to confirm
// an action or make a selection. Matched case-insensitively against the visible
// tmux pane.
var approvalPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)do you want to proceed`),
	regexp.MustCompile(`(?i)do you want to (create|make|allow|run|edit|delete)`),
	regexp.MustCompile(`(?i)\bwould you like to proceed`),
	regexp.MustCompile(`(?im)^\s*❯?\s*1\.\s*yes`),        // numbered choice list
	regexp.MustCompile(`(?i)\b1\.\s*yes\b.*\b2\.\s*no\b`), // inline yes/no options
	regexp.MustCompile(`(?i)\(y/n\)`),
	regexp.MustCompile(`(?i)press\s+enter\s+to\s+continue`),
	regexp.MustCompile(`(?i)allow this (tool|command|action)`),
	// Interactive selection menus: a "❯" cursor on a numbered option, or the
	// standard footer Claude shows for a menu awaiting the user's choice.
	regexp.MustCompile(`(?im)^\s*❯\s*\d+\.`),
	regexp.MustCompile(`(?i)enter to (select|confirm|continue)`),
	regexp.MustCompile(`(?i)esc to (cancel|interrupt|exit)`),
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

// detectStatus determines a session's real status. Blocked ("waiting_approval")
// detection requires reading the live tmux pane, which shows the actual approval
// prompt. Sessions not running under tmux can't be reliably classified as
// blocked (a transcript's dangling tool_use is indistinguishable from normal
// mid-execution), so they default to running. This avoids false "blocked"
// notifications for the common non-tmux case.
func (p *Provider) detectStatus(ctx context.Context, s protocol.Session, sharedCWD bool) protocol.SessionStatus {
	_ = sharedCWD // retained for signature stability; no longer needed
	if s.TmuxSession != "" {
		out, err := exec.CommandContext(ctx, "tmux", "capture-pane", "-t", s.TmuxSession, "-p").Output()
		if err == nil {
			return classifyPaneText(string(out))
		}
	}
	return protocol.StatusRunning
}
