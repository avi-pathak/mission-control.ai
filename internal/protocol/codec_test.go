package protocol

import "testing"

func TestEncodeDecodeRoundTrip(t *testing.T) {
	orig := SessionUpsert{Session: Session{ID: "s1", Status: StatusRunning, Repo: "mc"}}
	data, err := Encode(TypeSessionUpsert, orig)
	if err != nil {
		t.Fatal(err)
	}
	env, err := Decode(data)
	if err != nil {
		t.Fatal(err)
	}
	if env.V != Version || env.Type != TypeSessionUpsert {
		t.Fatalf("bad envelope: %+v", env)
	}
	got, err := As[SessionUpsert](env)
	if err != nil {
		t.Fatal(err)
	}
	if got.Session.ID != "s1" || got.Session.Status != StatusRunning {
		t.Fatalf("payload mismatch: %+v", got)
	}
}
