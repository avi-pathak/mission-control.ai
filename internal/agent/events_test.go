package agent

import (
	"testing"

	"github.com/avi-pathak/mission-control.ai/internal/protocol"
)

func kinds(evs []protocol.Event) []string {
	out := make([]string, len(evs))
	for i, e := range evs {
		out[i] = e.Kind
	}
	return out
}

func TestDiffSessionEvents(t *testing.T) {
	base := protocol.Session{ID: "s1", Repo: "mc", Branch: "main", Status: protocol.StatusRunning}

	t.Run("new session emits started", func(t *testing.T) {
		evs := diffSessionEvents(protocol.Session{}, base, false)
		if len(evs) != 1 || evs[0].Kind != protocol.EventSessionStarted {
			t.Fatalf("got %v", kinds(evs))
		}
		if evs[0].Severity != protocol.SeveritySuccess {
			t.Fatalf("expected success severity, got %s", evs[0].Severity)
		}
	})

	t.Run("no change emits nothing", func(t *testing.T) {
		if evs := diffSessionEvents(base, base, true); len(evs) != 0 {
			t.Fatalf("expected no events, got %v", kinds(evs))
		}
	})

	t.Run("status change", func(t *testing.T) {
		cur := base
		cur.Status = protocol.StatusError
		evs := diffSessionEvents(base, cur, true)
		if len(evs) != 1 || evs[0].Kind != protocol.EventStatusChanged {
			t.Fatalf("got %v", kinds(evs))
		}
		if evs[0].Severity != protocol.SeverityError {
			t.Fatalf("expected error severity, got %s", evs[0].Severity)
		}
	})

	t.Run("branch change", func(t *testing.T) {
		cur := base
		cur.Branch = "feature"
		evs := diffSessionEvents(base, cur, true)
		if len(evs) != 1 || evs[0].Kind != protocol.EventBranchChanged {
			t.Fatalf("got %v", kinds(evs))
		}
	})

	t.Run("new commit", func(t *testing.T) {
		prev := base
		prev.Git = &protocol.GitInfo{RecentCommits: []protocol.Commit{{Hash: "aaa"}}}
		cur := base
		cur.Git = &protocol.GitInfo{RecentCommits: []protocol.Commit{{Hash: "bbb", Message: "fix"}}}
		evs := diffSessionEvents(prev, cur, true)
		if len(evs) != 1 || evs[0].Kind != protocol.EventCommitCreated {
			t.Fatalf("got %v", kinds(evs))
		}
		if evs[0].Meta["hash"] != "bbb" {
			t.Fatalf("expected hash meta bbb, got %q", evs[0].Meta["hash"])
		}
	})

	t.Run("multiple simultaneous changes", func(t *testing.T) {
		cur := base
		cur.Status = protocol.StatusWaitingApproval
		cur.Branch = "dev"
		evs := diffSessionEvents(base, cur, true)
		if len(evs) != 2 {
			t.Fatalf("expected 2 events, got %v", kinds(evs))
		}
	})
}
