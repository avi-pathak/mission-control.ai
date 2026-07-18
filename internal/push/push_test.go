package push

import "testing"

func TestSenderDisabled(t *testing.T) {
	s := New(Config{}) // no keys
	if s.Enabled() {
		t.Fatal("sender with no keys should be disabled")
	}
	// Send is a no-op (nil) when disabled.
	if err := s.Send(Subscription{Endpoint: "https://x"}, Payload{Title: "hi"}); err != nil {
		t.Fatalf("disabled Send should be a no-op, got %v", err)
	}
	if s.PublicKey() != "" {
		t.Fatal("expected empty public key when disabled")
	}
}

func TestSenderEnabled(t *testing.T) {
	s := New(Config{VAPIDPublic: "pub", VAPIDPrivate: "priv", Subject: "mailto:x@y.z"})
	if !s.Enabled() {
		t.Fatal("sender with keys should be enabled")
	}
	if s.PublicKey() != "pub" {
		t.Fatalf("public key = %q, want pub", s.PublicKey())
	}
}
