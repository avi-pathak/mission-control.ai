package state

import (
	"testing"

	"github.com/avi-pathak/mission-control.ai/internal/protocol"
)

const org = "org1"

func TestUpsertAndSnapshot(t *testing.T) {
	m := New()
	m.UpsertMachine(org, protocol.Machine{ID: "m1", Hostname: "host", Online: true})
	m.UpsertSession(org, protocol.Session{ID: "s1", MachineID: "m1", Status: protocol.StatusRunning, StartedAt: 100})
	m.UpsertSession(org, protocol.Session{ID: "s2", MachineID: "m1", Status: protocol.StatusError, StartedAt: 200})

	snap := m.Snapshot(org)
	if len(snap.Machines) != 1 || len(snap.Sessions) != 2 {
		t.Fatalf("unexpected snapshot sizes: %d machines, %d sessions", len(snap.Machines), len(snap.Sessions))
	}
	if snap.Sessions[0].ID != "s2" {
		t.Fatalf("expected s2 first, got %s", snap.Sessions[0].ID)
	}
}

func TestRemoveMachineSessions(t *testing.T) {
	m := New()
	m.UpsertSession(org, protocol.Session{ID: "s1", MachineID: "m1"})
	m.UpsertSession(org, protocol.Session{ID: "s2", MachineID: "m2"})
	removed := m.RemoveMachineSessions(org, "m1")
	if len(removed) != 1 || removed[0] != "s1" {
		t.Fatalf("expected [s1], got %v", removed)
	}
	if _, ok := m.Session(org, "s1"); ok {
		t.Fatal("s1 should be gone")
	}
	if _, ok := m.Session(org, "s2"); !ok {
		t.Fatal("s2 should remain")
	}
}

func TestApplyHeartbeat(t *testing.T) {
	m := New()
	m.UpsertMachine(org, protocol.Machine{ID: "m1"})
	got, ok := m.ApplyHeartbeat(org, protocol.AgentHeartbeat{AgentID: "m1", CPUPct: 42}, 999)
	if !ok || got.CPUPct != 42 || !got.Online {
		t.Fatalf("heartbeat not applied: %+v ok=%v", got, ok)
	}
}

func TestOrgIsolation(t *testing.T) {
	m := New()
	m.UpsertSession("orgA", protocol.Session{ID: "a1", MachineID: "m1"})
	m.UpsertSession("orgB", protocol.Session{ID: "b1", MachineID: "m2"})

	if len(m.Snapshot("orgA").Sessions) != 1 {
		t.Fatal("orgA should see exactly its session")
	}
	if _, ok := m.Session("orgA", "b1"); ok {
		t.Fatal("orgA must not see orgB's session")
	}
	if _, ok := m.Session("orgB", "a1"); ok {
		t.Fatal("orgB must not see orgA's session")
	}
}
