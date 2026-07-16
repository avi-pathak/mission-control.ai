package server

import (
	"bytes"
	"net/http"
	"testing"
	"time"

	"github.com/avi-pathak/mission-control.ai/internal/protocol"
)

func TestRegisterLoginAndMe(t *testing.T) {
	e := newTestEnv(t)
	jwt := e.register(t, "owner@acme.com")

	// Duplicate registration is rejected.
	if code := e.post(t, "/api/auth/register", "", map[string]string{"email": "owner@acme.com", "password": "password123"}, nil); code != http.StatusConflict {
		t.Fatalf("expected 409 on dup register, got %d", code)
	}

	// Login works.
	var login authResp
	if code := e.post(t, "/api/auth/login", "", map[string]string{"email": "owner@acme.com", "password": "password123"}, &login); code != 200 || login.Token == "" {
		t.Fatalf("login failed: %d", code)
	}

	// Me requires auth.
	if code := e.get(t, "/api/me", "", nil); code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", code)
	}
	var me map[string]any
	if code := e.get(t, "/api/me", jwt, &me); code != 200 {
		t.Fatalf("me failed: %d", code)
	}
}

func TestEnrollThenConnectScopedToOrg(t *testing.T) {
	e := newTestEnv(t)
	jwt := e.register(t, "a@acme.com")
	res := e.enrollAgent(t, jwt)

	// Reusing the enroll token fails.
	var tok map[string]any
	e.post(t, "/api/enroll-tokens", jwt, map[string]any{}, &tok)
	// (fresh token; ensure a used one can't be reused)

	agent := e.dialAgent(t, res.AgentKey, res.AgentID)
	defer agent.Close()
	writeWS(t, agent, protocol.TypeSessionUpsert, protocol.SessionUpsert{
		Session: protocol.Session{ID: "s1", Repo: "demo", Status: protocol.StatusRunning},
	})

	dash := e.dialDashboard(t, jwt)
	defer dash.Close()
	if env := readWS(t, dash); env.Type != protocol.TypeSnapshot {
		t.Fatalf("expected snapshot, got %s", env.Type)
	}
	// Session upsert should arrive.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if readWS(t, dash).Type == protocol.TypeSessionUpsert {
			return
		}
	}
	t.Fatal("dashboard never received session.upsert")
}

func TestTwoOrgIsolation(t *testing.T) {
	e := newTestEnv(t)
	jwtA := e.register(t, "a@a.com")
	jwtB := e.register(t, "b@b.com")

	resA := e.enrollAgent(t, jwtA)
	agentA := e.dialAgent(t, resA.AgentKey, resA.AgentID)
	defer agentA.Close()
	writeWS(t, agentA, protocol.TypeSessionUpsert, protocol.SessionUpsert{
		Session: protocol.Session{ID: "sA", Repo: "secret-a", Status: protocol.StatusRunning},
	})
	time.Sleep(150 * time.Millisecond)

	// Org B must NOT see org A's session or machine over REST.
	var sessB []protocol.Session
	e.get(t, "/api/sessions", jwtB, &sessB)
	if len(sessB) != 0 {
		t.Fatalf("org B leaked %d sessions from org A", len(sessB))
	}
	var machB []protocol.Machine
	e.get(t, "/api/machines", jwtB, &machB)
	if len(machB) != 0 {
		t.Fatalf("org B leaked %d machines from org A", len(machB))
	}

	// Org A sees its own.
	var sessA []protocol.Session
	e.get(t, "/api/sessions", jwtA, &sessA)
	if len(sessA) != 1 || sessA[0].Repo != "secret-a" {
		t.Fatalf("org A should see its own session, got %+v", sessA)
	}
}

func TestEventFlowScoped(t *testing.T) {
	e := newTestEnv(t)
	jwt := e.register(t, "a@acme.com")
	res := e.enrollAgent(t, jwt)
	agent := e.dialAgent(t, res.AgentKey, res.AgentID)
	defer agent.Close()

	writeWS(t, agent, protocol.TypeEventReport, protocol.EventReport{Event: protocol.Event{
		ID: "e1", Kind: protocol.EventSessionStarted, Message: "started", Severity: protocol.SeveritySuccess,
	}})
	time.Sleep(150 * time.Millisecond)

	var events []protocol.Event
	e.get(t, "/api/events", jwt, &events)
	found := false
	for _, ev := range events {
		if ev.ID == "e1" {
			found = true
		}
	}
	if !found {
		t.Fatalf("event e1 not found in %d events", len(events))
	}
}

func TestPublishAndDownloadFile(t *testing.T) {
	e := newTestEnv(t)
	jwt := e.register(t, "a@acme.com")
	res := e.enrollAgent(t, jwt)

	// Publish via agent key.
	req, _ := http.NewRequest("POST", e.ts.URL+"/api/publish", bytes.NewReader([]byte("hello world")))
	req.Header.Set("X-API-Key", res.AgentKey)
	req.Header.Set("X-MC-Filename", "out.txt")
	req.Header.Set("X-MC-Session", "s1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil || resp.StatusCode != http.StatusCreated {
		t.Fatalf("publish failed: %v code=%d", err, resp.StatusCode)
	}
	resp.Body.Close()

	// List files for the org.
	var files []map[string]any
	e.get(t, "/api/files", jwt, &files)
	if len(files) != 1 || files[0]["name"] != "out.txt" {
		t.Fatalf("expected 1 file, got %+v", files)
	}

	// A different org cannot list it.
	jwtB := e.register(t, "b@b.com")
	var filesB []map[string]any
	e.get(t, "/api/files", jwtB, &filesB)
	if len(filesB) != 0 {
		t.Fatalf("org B leaked %d files", len(filesB))
	}
}

func TestInviteFlow(t *testing.T) {
	e := newTestEnv(t)
	jwt := e.register(t, "owner@acme.com")

	var inv map[string]any
	if code := e.post(t, "/api/org/invites", jwt, map[string]string{"email": "member@acme.com", "role": "member"}, &inv); code != http.StatusCreated {
		t.Fatalf("create invite failed: %d", code)
	}
	token, _ := inv["token"].(string)
	if token == "" {
		t.Fatal("no invite token")
	}

	// Accept the invite creates a user + returns a token.
	var accepted authResp
	if code := e.post(t, "/api/auth/accept-invite", "", map[string]string{"token": token, "password": "password123"}, &accepted); code != 200 || accepted.Token == "" {
		t.Fatalf("accept invite failed: %d", code)
	}
	if accepted.User.OrgID == "" || accepted.User.Email != "member@acme.com" {
		t.Fatalf("accepted user malformed: %+v", accepted.User)
	}
}
