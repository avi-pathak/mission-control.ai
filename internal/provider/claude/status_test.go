package claude

import (
	"testing"

	"github.com/avi-pathak/mission-control.ai/internal/protocol"
)

func TestClassifyPaneText(t *testing.T) {
	cases := []struct {
		name string
		pane string
		want protocol.SessionStatus
	}{
		{
			"proceed prompt",
			"some output\n Do you want to proceed?\n ❯ 1. Yes\n   2. No",
			protocol.StatusWaitingApproval,
		},
		{
			"numbered yes",
			"Edit file?\n❯ 1. Yes\n  2. No, tell Claude what to do differently",
			protocol.StatusWaitingApproval,
		},
		{
			"y/n prompt",
			"Run this command? (y/n)",
			protocol.StatusWaitingApproval,
		},
		{
			"allow tool",
			"Allow this command to run?",
			protocol.StatusWaitingApproval,
		},
		{
			// Real interactive selection menu (numbered options with a ❯ cursor
			// and the standard footer) — a session waiting for the user's choice.
			"selection menu",
			"  ❯ 1. Application Programming Interface\n  2. Applied Program Integration\n  3. Automated Process\nEnter to select · ↑/↓ to navigate · Esc to cancel",
			protocol.StatusWaitingApproval,
		},
		{
			"idle input prompt",
			"╭─── Claude Code ───╮\n│ ready │\n❯ type your message",
			protocol.StatusRunning,
		},
		{
			"working output",
			"Reading files...\nEditing app.tsx\nRunning tests",
			protocol.StatusRunning,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := classifyPaneText(c.pane); got != c.want {
				t.Fatalf("classifyPaneText = %s, want %s", got, c.want)
			}
		})
	}
}
