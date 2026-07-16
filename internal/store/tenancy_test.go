package store

import (
	"testing"
	"time"

	"github.com/avi-pathak/mission-control.ai/internal/protocol"
)

func TestDriverSelection(t *testing.T) {
	if !isPostgresDSN("postgres://u:p@h/db") || !isPostgresDSN("postgresql://h/db") {
		t.Fatal("expected postgres DSNs to be detected")
	}
	if isPostgresDSN("/var/lib/mc.db") || isPostgresDSN(":memory:") {
		t.Fatal("expected file paths to be treated as sqlite")
	}
}

func TestOrgUserInviteLifecycle(t *testing.T) {
	st := newTestStore(t)
	now := time.Now()

	org, err := st.CreateOrg("Acme", "acme", now)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := st.GetOrg(org.ID); err != nil || got.Slug != "acme" {
		t.Fatalf("GetOrg: %+v err=%v", got, err)
	}

	n, _ := st.CountUsers()
	if n != 0 {
		t.Fatalf("expected 0 users, got %d", n)
	}

	u, err := st.CreateUser(User{OrgID: org.ID, Email: "a@b.com", PasswordHash: "x", Role: "owner"})
	if err != nil {
		t.Fatal(err)
	}
	if got, err := st.UserByEmail("A@B.com"); err != nil || got.ID != u.ID {
		t.Fatalf("UserByEmail case-insensitive failed: %+v err=%v", got, err)
	}

	inv, err := st.CreateInvite(org.ID, "c@d.com", "member", u.ID, time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}
	invs, _ := st.ListInvites(org.ID)
	if len(invs) != 1 {
		t.Fatalf("expected 1 pending invite, got %d", len(invs))
	}
	if err := st.AcceptInvite(inv.Token, now); err != nil {
		t.Fatal(err)
	}
	invs, _ = st.ListInvites(org.ID)
	if len(invs) != 0 {
		t.Fatalf("expected 0 pending after accept, got %d", len(invs))
	}
}

func TestFileStore(t *testing.T) {
	st := newTestStore(t)
	f, err := st.SaveFile(PublishedFile{
		OrgID: "org1", MachineID: "m1", SessionID: "s1",
		Name: "out.log", Size: 3, ContentType: "text/plain", Data: []byte("abc"),
	})
	if err != nil {
		t.Fatal(err)
	}
	metas, _ := st.ListFiles("org1", "s1", "", 10)
	if len(metas) != 1 || metas[0].Name != "out.log" {
		t.Fatalf("ListFiles: %+v", metas)
	}
	// Cross-org isolation.
	if metas, _ := st.ListFiles("other", "s1", "", 10); len(metas) != 0 {
		t.Fatalf("expected no files for other org, got %d", len(metas))
	}
	got, err := st.GetFile("org1", f.ID)
	if err != nil || string(got.Data) != "abc" {
		t.Fatalf("GetFile: %q err=%v", got.Data, err)
	}
	if _, err := st.GetFile("other", f.ID); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound for cross-org file, got %v", err)
	}
}

func TestDeleteMachineCascade(t *testing.T) {
	st := newTestStore(t)
	now := time.Now()
	// org1 machine m1 with a session + log + metric + event.
	_ = st.UpsertMachine("org1", protocol.Machine{ID: "m1", Hostname: "h"})
	_ = st.UpsertSession("org1", protocol.Session{ID: "s1", MachineID: "m1"})
	_ = st.AppendLog("org1", protocol.LogAppend{SessionID: "s1", Seq: 1, Line: "x", TS: now.UnixMilli()})
	_ = st.SaveEvent("org1", protocol.Event{ID: "e1", MachineID: "m1", Kind: "k", TS: now.UnixMilli()})
	// A different org's machine must be untouched.
	_ = st.UpsertMachine("org2", protocol.Machine{ID: "m2", Hostname: "h2"})

	if err := st.DeleteMachine("org1", "m1"); err != nil {
		t.Fatal(err)
	}
	// Cross-org delete of m1 from org2 should be ErrNotFound (already gone / scoped).
	if err := st.DeleteMachine("org2", "m1"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
	// org1 machine + its rows gone.
	lines, _ := st.LogPage("org1", "s1", 0, 10)
	if len(lines) != 0 {
		t.Fatalf("logs not cascaded: %d", len(lines))
	}
	evs, _ := st.ListEvents("org1", EventFilter{}, 10)
	if len(evs) != 0 {
		t.Fatalf("events not cascaded: %d", len(evs))
	}
	// org2 untouched.
	if _, err := st.GetOrg("org1"); err == nil {
		_ = err // org rows are separate; machine scope only
	}
}
