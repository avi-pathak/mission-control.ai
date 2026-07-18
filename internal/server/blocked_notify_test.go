package server

import (
	"testing"
	"time"

	"github.com/avi-pathak/mission-control.ai/internal/protocol"
)

func sess(id string, st protocol.SessionStatus) protocol.Session {
	return protocol.Session{ID: id, Status: st}
}

func TestBlockedTracker_NotifiesOnceAfterThreshold(t *testing.T) {
	tr := newBlockedTracker()
	threshold := 30 * time.Second
	t0 := time.Unix(1_700_000_000, 0)

	// Session becomes blocked at t0 — not yet past threshold.
	got := tr.reconcile([]protocol.Session{sess("a", protocol.StatusWaitingApproval)}, threshold, t0)
	if len(got) != 0 {
		t.Fatalf("expected no notify at t0, got %d", len(got))
	}

	// 20s later — still under threshold.
	got = tr.reconcile([]protocol.Session{sess("a", protocol.StatusWaitingApproval)}, threshold, t0.Add(20*time.Second))
	if len(got) != 0 {
		t.Fatalf("expected no notify at 20s, got %d", len(got))
	}

	// 31s later — crosses threshold, notify once.
	got = tr.reconcile([]protocol.Session{sess("a", protocol.StatusWaitingApproval)}, threshold, t0.Add(31*time.Second))
	if len(got) != 1 || got[0].ID != "a" {
		t.Fatalf("expected notify for a at 31s, got %+v", got)
	}

	// 40s later — already notified, no repeat.
	got = tr.reconcile([]protocol.Session{sess("a", protocol.StatusWaitingApproval)}, threshold, t0.Add(40*time.Second))
	if len(got) != 0 {
		t.Fatalf("expected no repeat notify, got %d", len(got))
	}
}

func TestBlockedTracker_ReArmsAfterUnblock(t *testing.T) {
	tr := newBlockedTracker()
	threshold := 10 * time.Second
	t0 := time.Unix(1_700_000_000, 0)

	// Block → notify.
	tr.reconcile([]protocol.Session{sess("a", protocol.StatusWaitingApproval)}, threshold, t0)
	got := tr.reconcile([]protocol.Session{sess("a", protocol.StatusWaitingApproval)}, threshold, t0.Add(11*time.Second))
	if len(got) != 1 {
		t.Fatalf("expected first notify, got %d", len(got))
	}

	// Unblock (running) → tracker clears the episode.
	tr.reconcile([]protocol.Session{sess("a", protocol.StatusRunning)}, threshold, t0.Add(12*time.Second))

	// Re-block → should notify AGAIN after the threshold.
	tr.reconcile([]protocol.Session{sess("a", protocol.StatusWaitingApproval)}, threshold, t0.Add(20*time.Second))
	got = tr.reconcile([]protocol.Session{sess("a", protocol.StatusWaitingApproval)}, threshold, t0.Add(31*time.Second))
	if len(got) != 1 {
		t.Fatalf("expected re-notify after re-block, got %d", len(got))
	}
}

func TestBlockedTracker_ForgetsVanishedSessions(t *testing.T) {
	tr := newBlockedTracker()
	threshold := 10 * time.Second
	t0 := time.Unix(1_700_000_000, 0)

	tr.reconcile([]protocol.Session{sess("a", protocol.StatusWaitingApproval)}, threshold, t0)
	// Session vanishes.
	tr.reconcile([]protocol.Session{}, threshold, t0.Add(5*time.Second))
	if len(tr.since) != 0 || len(tr.notified) != 0 {
		t.Fatalf("expected tracker to forget vanished session, since=%d notified=%d", len(tr.since), len(tr.notified))
	}
}

func TestBlockedTracker_MultipleSessions(t *testing.T) {
	tr := newBlockedTracker()
	threshold := 10 * time.Second
	t0 := time.Unix(1_700_000_000, 0)

	sessions := []protocol.Session{
		sess("a", protocol.StatusWaitingApproval),
		sess("b", protocol.StatusRunning),
		sess("c", protocol.StatusWaitingApproval),
	}
	tr.reconcile(sessions, threshold, t0)
	got := tr.reconcile(sessions, threshold, t0.Add(11*time.Second))
	if len(got) != 2 {
		t.Fatalf("expected a and c to notify, got %d", len(got))
	}
}
