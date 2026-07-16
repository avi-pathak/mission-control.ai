package store

import (
	"testing"
	"time"

	"go.uber.org/zap"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(":memory:", zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	return st
}

func TestEnrollTokenLifecycle(t *testing.T) {
	st := newTestStore(t)
	now := time.Now()

	tok, err := st.CreateEnrollToken("org1", "prod-1", time.Hour, now)
	if err != nil {
		t.Fatal(err)
	}
	if tok.Status(now) != TokenActive {
		t.Fatalf("expected active, got %s", tok.Status(now))
	}

	// Consume mints an agent key.
	key, err := st.ConsumeEnrollToken(tok.Token, "m1", "prod-1", now)
	if err != nil {
		t.Fatalf("consume: %v", err)
	}
	if key.MachineID != "m1" || key.Key == "" {
		t.Fatalf("bad minted key: %+v", key)
	}

	// Second consume must fail (single-use).
	if _, err := st.ConsumeEnrollToken(tok.Token, "m2", "x", now); err != ErrTokenInvalid {
		t.Fatalf("expected ErrTokenInvalid on reuse, got %v", err)
	}

	keys, _ := st.ActiveAgentKeys()
	if len(keys) != 1 {
		t.Fatalf("expected 1 active key, got %d", len(keys))
	}
}

func TestEnrollTokenExpiryAndRevoke(t *testing.T) {
	st := newTestStore(t)
	now := time.Now()

	expired, _ := st.CreateEnrollToken("org1", "", time.Minute, now)
	if _, err := st.ConsumeEnrollToken(expired.Token, "m1", "", now.Add(2*time.Minute)); err != ErrTokenInvalid {
		t.Fatalf("expected invalid for expired, got %v", err)
	}

	revoked, _ := st.CreateEnrollToken("org1", "", time.Hour, now)
	if err := st.RevokeEnrollToken("org1", revoked.Token); err != nil {
		t.Fatal(err)
	}
	if _, err := st.ConsumeEnrollToken(revoked.Token, "m1", "", now); err != ErrTokenInvalid {
		t.Fatalf("expected invalid for revoked, got %v", err)
	}
}

func TestRevokeAgentKeysForMachine(t *testing.T) {
	st := newTestStore(t)
	now := time.Now()
	tok, _ := st.CreateEnrollToken("org1", "", time.Hour, now)
	_, _ = st.ConsumeEnrollToken(tok.Token, "m1", "", now)

	if err := st.RevokeAgentKeysForMachine("m1"); err != nil {
		t.Fatal(err)
	}
	keys, _ := st.ActiveAgentKeys()
	if len(keys) != 0 {
		t.Fatalf("expected 0 active keys after revoke, got %d", len(keys))
	}
}
