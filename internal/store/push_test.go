package store

import (
	"testing"

	"go.uber.org/zap"
)

func TestPushSubscriptionCRUD(t *testing.T) {
	st, err := Open(":memory:", zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	sub := PushSubscription{
		OrgID: "org1", UserID: "u1", Endpoint: "https://push/abc",
		P256dh: "p", Auth: "a",
	}
	if err := st.SavePushSubscription(sub); err != nil {
		t.Fatalf("save: %v", err)
	}

	// Idempotent upsert by endpoint: same endpoint refreshes, no duplicate.
	sub.P256dh = "p2"
	if err := st.SavePushSubscription(sub); err != nil {
		t.Fatalf("resave: %v", err)
	}
	byOrg, _ := st.ListPushSubscriptionsForOrg("org1")
	if len(byOrg) != 1 {
		t.Fatalf("expected 1 subscription, got %d", len(byOrg))
	}
	if byOrg[0].P256dh != "p2" {
		t.Fatalf("expected refreshed key p2, got %q", byOrg[0].P256dh)
	}

	byUser, _ := st.ListPushSubscriptionsForUser("u1")
	if len(byUser) != 1 {
		t.Fatalf("expected 1 for user, got %d", len(byUser))
	}

	if err := st.DeletePushSubscription("https://push/abc"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	byOrg, _ = st.ListPushSubscriptionsForOrg("org1")
	if len(byOrg) != 0 {
		t.Fatalf("expected 0 after delete, got %d", len(byOrg))
	}
}
