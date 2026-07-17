package codex

import (
	"context"
	"encoding/json"
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

type rolloutItem struct {
	Type    string `json:"type"`
	Payload struct {
		Type string `json:"type"`
	} `json:"payload"`
}

// classifyRolloutTail infers status from the tail of a rollout. A dangling
// function_call / custom_tool_call with no following *_output means the turn is
// paused awaiting the tool result or the user's approval. A trailing
// task_complete is treated as running (Codex may still be alive at the prompt).
// Pure and unit-testable.
func classifyRolloutTail(lines []string) protocol.SessionStatus {
	// Find the last response_item that is a call or an output.
	for i := len(lines) - 1; i >= 0; i-- {
		var it rolloutItem
		if err := json.Unmarshal([]byte(lines[i]), &it); err != nil {
			continue
		}
		if it.Type != "response_item" {
			continue
		}
		switch it.Payload.Type {
		case "function_call", "custom_tool_call":
			return protocol.StatusWaitingApproval // dangling call → awaiting result/approval
		case "function_call_output", "custom_tool_call_output", "message", "reasoning":
			return protocol.StatusRunning
		}
	}
	return protocol.StatusRunning
}

// detectStatus determines a Codex session's status: tmux pane scan when
// available (authoritative), else rollout-tail heuristic.
func (p *Provider) detectStatus(ctx context.Context, s protocol.Session, rolloutPath string) protocol.SessionStatus {
	if s.TmuxSession != "" {
		if out, err := exec.CommandContext(ctx, "tmux", "capture-pane", "-t", s.TmuxSession, "-p").Output(); err == nil {
			return classifyPaneText(string(out))
		}
	}
	if rolloutPath != "" {
		return classifyRolloutTail(tailLines(rolloutPath, 40))
	}
	return protocol.StatusRunning
}
