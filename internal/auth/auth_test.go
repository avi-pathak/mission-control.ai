package auth

import (
	"testing"
	"time"
)

func TestPasswordHashing(t *testing.T) {
	hash, err := HashPassword("s3cret")
	if err != nil {
		t.Fatal(err)
	}
	if !CheckPassword(hash, "s3cret") {
		t.Fatal("correct password rejected")
	}
	if CheckPassword(hash, "wrong") {
		t.Fatal("wrong password accepted")
	}
}

func TestJWTRoundTrip(t *testing.T) {
	m := NewManager("test-secret", time.Hour)
	now := time.Now()
	tok, err := m.Issue("u1", "o1", RoleOwner, "a@b.com", now)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := m.Verify(tok)
	if err != nil {
		t.Fatal(err)
	}
	if claims.UserID != "u1" || claims.OrgID != "o1" || claims.Role != RoleOwner {
		t.Fatalf("bad claims: %+v", claims)
	}
}

func TestJWTRejectsBadSecret(t *testing.T) {
	tok, _ := NewManager("secret-a", time.Hour).Issue("u1", "o1", RoleMember, "e", time.Now())
	if _, err := NewManager("secret-b", time.Hour).Verify(tok); err != ErrInvalidToken {
		t.Fatalf("expected ErrInvalidToken, got %v", err)
	}
}

func TestJWTRejectsExpired(t *testing.T) {
	m := NewManager("secret", time.Hour)
	tok, _ := m.Issue("u1", "o1", RoleMember, "e", time.Now().Add(-2*time.Hour))
	if _, err := m.Verify(tok); err == nil {
		t.Fatal("expected expired token to be rejected")
	}
}
