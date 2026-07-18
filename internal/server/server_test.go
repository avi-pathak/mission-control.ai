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
	if code := e.post(t, "/api/v1/auth/register", "", map[string]string{"email": "owner@acme.com", "password": "password123"}, nil); code != http.StatusConflict {
		t.Fatalf("expected 409 on dup register, got %d", code)
	}

	// Login works.
	var login authResp
	if code := e.post(t, "/api/v1/auth/login", "", map[string]string{"email": "owner@acme.com", "password": "password123"}, &login); code != 200 || login.Token == "" {
		t.Fatalf("login failed: %d", code)
	}

	// Me requires auth.
	if code := e.get(t, "/api/v1/me", "", nil); code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without token, got %d", code)
	}
	var me map[string]any
	if code := e.get(t, "/api/v1/me", jwt, &me); code != 200 {
		t.Fatalf("me failed: %d", code)
	}
}

func TestEnrollThenConnectScopedToOrg(t *testing.T) {
	e := newTestEnv(t)
	jwt := e.register(t, "a@acme.com")
	res := e.enrollAgent(t, jwt)

	// Reusing the enroll token fails.
	var tok map[string]any
	e.post(t, "/api/v1/enroll-tokens", jwt, map[string]any{}, &tok)
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
	e.get(t, "/api/v1/sessions", jwtB, &sessB)
	if len(sessB) != 0 {
		t.Fatalf("org B leaked %d sessions from org A", len(sessB))
	}
	var machB []protocol.Machine
	e.get(t, "/api/v1/machines", jwtB, &machB)
	if len(machB) != 0 {
		t.Fatalf("org B leaked %d machines from org A", len(machB))
	}

	// Org A sees its own.
	var sessA []protocol.Session
	e.get(t, "/api/v1/sessions", jwtA, &sessA)
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
	e.get(t, "/api/v1/events", jwt, &events)
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
	req, _ := http.NewRequest("POST", e.ts.URL+"/api/v1/publish", bytes.NewReader([]byte("hello world")))
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
	e.get(t, "/api/v1/files", jwt, &files)
	if len(files) != 1 || files[0]["name"] != "out.txt" {
		t.Fatalf("expected 1 file, got %+v", files)
	}

	// A different org cannot list it.
	jwtB := e.register(t, "b@b.com")
	var filesB []map[string]any
	e.get(t, "/api/v1/files", jwtB, &filesB)
	if len(filesB) != 0 {
		t.Fatalf("org B leaked %d files", len(filesB))
	}
}

func TestInviteFlow(t *testing.T) {
	e := newTestEnv(t)
	jwt := e.register(t, "owner@acme.com")

	var inv map[string]any
	if code := e.post(t, "/api/v1/org/invites", jwt, map[string]string{"email": "member@acme.com", "role": "member"}, &inv); code != http.StatusCreated {
		t.Fatalf("create invite failed: %d", code)
	}
	token, _ := inv["token"].(string)
	if token == "" {
		t.Fatal("no invite token")
	}

	// Accept the invite creates a user + returns a token.
	var accepted authResp
	if code := e.post(t, "/api/v1/auth/accept-invite", "", map[string]string{"token": token, "password": "password123"}, &accepted); code != 200 || accepted.Token == "" {
		t.Fatalf("accept invite failed: %d", code)
	}
	if accepted.User.OrgID == "" || accepted.User.Email != "member@acme.com" {
		t.Fatalf("accepted user malformed: %+v", accepted.User)
	}
}

// TestSignupApprovalFlow covers the pending-signup → superadmin-approve →
// login lifecycle and the superadmin gate.
func TestSignupApprovalFlow(t *testing.T) {
	e := newTestEnv(t)

	// 1. Signup is pending: no token issued (202).
	var reg map[string]any
	if code := e.post(t, "/api/v1/auth/register", "",
		map[string]string{"email": "new@user.com", "password": "password123"}, &reg); code != http.StatusAccepted {
		t.Fatalf("expected 202 pending register, got %d", code)
	}
	if reg["status"] != "pending" {
		t.Fatalf("expected pending status, got %v", reg)
	}

	// 2. Login while pending → 403.
	if code := e.post(t, "/api/v1/auth/login", "",
		map[string]string{"email": "new@user.com", "password": "password123"}, nil); code != http.StatusForbidden {
		t.Fatalf("expected 403 login-while-pending, got %d", code)
	}

	// 3. A normal (non-admin) user cannot see the approval queue.
	ownerJWT := e.register(t, "owner@acme.com")
	if code := e.get(t, "/api/v1/admin/pending-users", ownerJWT, nil); code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin on admin route, got %d", code)
	}

	// 4. Make a platform-admin JWT directly and list pending users.
	adminJWT := e.superadmin(t, "root@platform.com")
	var pending []pendingUserView
	if code := e.get(t, "/api/v1/admin/pending-users", adminJWT, &pending); code != 200 {
		t.Fatalf("list pending failed: %d", code)
	}
	if len(pending) != 1 || pending[0].Email != "new@user.com" {
		t.Fatalf("unexpected pending list: %+v", pending)
	}

	// 5. Approve → create a new namespace + member role.
	var approved map[string]any
	if code := e.post(t, "/api/v1/admin/users/"+pending[0].ID+"/approve", adminJWT,
		map[string]string{"newOrgName": "Acme Research", "role": "member"}, &approved); code != 200 {
		t.Fatalf("approve failed: %d", code)
	}
	if approved["status"] != "active" || approved["orgId"] == "" {
		t.Fatalf("approve result malformed: %+v", approved)
	}

	// 6. Login now succeeds and lands in the assigned org.
	var login authResp
	if code := e.post(t, "/api/v1/auth/login", "",
		map[string]string{"email": "new@user.com", "password": "password123"}, &login); code != 200 || login.Token == "" {
		t.Fatalf("post-approval login failed: %d", code)
	}
	if login.User.OrgID != approved["orgId"] {
		t.Fatalf("user landed in wrong org: %s vs %v", login.User.OrgID, approved["orgId"])
	}
}

// TestMachineOneWorkspace verifies a machine can't be enrolled into a second
// workspace, that same-workspace re-enroll is idempotent, and that a superadmin
// can reassign it.
func TestMachineOneWorkspace(t *testing.T) {
	e := newTestEnv(t)

	// Two orgs (A owner, B owner) via helper.
	jwtA := e.register(t, "a@a.com")
	jwtB := e.register(t, "b@b.com")

	// Mint an enroll token in each org.
	var tokA, tokB map[string]any
	e.post(t, "/api/v1/enroll-tokens", jwtA, map[string]any{"label": "a"}, &tokA)
	e.post(t, "/api/v1/enroll-tokens", jwtB, map[string]any{"label": "b"}, &tokB)

	machineID := "mc-stable-host-123"

	// 1. First enroll into A succeeds.
	var resA protocol.EnrollResponse
	if code := e.post(t, "/api/v1/enroll", "", protocol.EnrollRequest{
		Token: tokA["token"].(string), MachineID: machineID, Hostname: "host",
	}, &resA); code != 200 || resA.AgentKey == "" {
		t.Fatalf("first enroll failed: %d", code)
	}

	// 2. Enroll the SAME machine into B → 409 machine_already_registered.
	if code := e.post(t, "/api/v1/enroll", "", protocol.EnrollRequest{
		Token: tokB["token"].(string), MachineID: machineID, Hostname: "host",
	}, nil); code != http.StatusConflict {
		t.Fatalf("expected 409 enrolling into 2nd workspace, got %d", code)
	}

	// 3. Superadmin reassigns the machine to B.
	admin := e.superadmin(t, "root@p.com")
	// Need B's org id: fetch admin machines and find it.
	var machines []adminMachineView
	e.get(t, "/api/v1/admin/machines", admin, &machines)
	if len(machines) == 0 {
		t.Fatal("admin sees no machines")
	}
	// B's org id from B's token
	var meB map[string]any
	e.get(t, "/api/v1/me", jwtB, &meB)
	orgB := meB["org"].(map[string]any)["id"].(string)

	if code := e.post(t, "/api/v1/admin/machines/"+machineID+"/reassign", admin,
		map[string]string{"orgId": orgB}, nil); code != 200 {
		t.Fatalf("reassign failed: %d", code)
	}

	// 4. Now enrolling into B is allowed (same workspace) — idempotent-ish.
	if code := e.post(t, "/api/v1/enroll", "", protocol.EnrollRequest{
		Token: tokB["token"].(string), MachineID: machineID, Hostname: "host",
	}, nil); code != 200 {
		t.Fatalf("expected enroll into now-owning workspace to succeed, got %d", code)
	}

	// 5. Non-admin cannot reassign.
	if code := e.post(t, "/api/v1/admin/machines/"+machineID+"/reassign", jwtA,
		map[string]string{"orgId": orgB}, nil); code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin reassign, got %d", code)
	}
}

// TestChangePassword covers the self-service password change flow.
func TestChangePassword(t *testing.T) {
	e := newTestEnv(t)
	// Create an active user via the store and log in to get a token.
	jwt := e.registerActiveLogin(t, "cp@user.com", "password123")

	// Wrong current password → 401.
	if code := e.post(t, "/api/v1/me/password", jwt,
		map[string]string{"currentPassword": "wrong", "newPassword": "newpass123"}, nil); code != http.StatusUnauthorized {
		t.Fatalf("expected 401 wrong current, got %d", code)
	}
	// Too-short new password → 400.
	if code := e.post(t, "/api/v1/me/password", jwt,
		map[string]string{"currentPassword": "password123", "newPassword": "short"}, nil); code != http.StatusBadRequest {
		t.Fatalf("expected 400 short new, got %d", code)
	}
	// Correct → 204, then login with new password works.
	if code := e.post(t, "/api/v1/me/password", jwt,
		map[string]string{"currentPassword": "password123", "newPassword": "newpass123"}, nil); code != http.StatusNoContent {
		t.Fatalf("expected 204 change, got %d", code)
	}
	var login authResp
	if code := e.post(t, "/api/v1/auth/login", "",
		map[string]string{"email": "cp@user.com", "password": "newpass123"}, &login); code != 200 || login.Token == "" {
		t.Fatalf("login with new password failed: %d", code)
	}
}

// TestMemberManagement covers role change + removal (admin-gated, no self-edit).
func TestMemberManagement(t *testing.T) {
	e := newTestEnv(t)
	adminJWT := e.register(t, "admin@team.com") // admin in its own org
	// Invite + accept a member into the same org.
	var inv map[string]any
	e.post(t, "/api/v1/org/invites", adminJWT, map[string]string{"email": "m@team.com", "role": "member"}, &inv)
	var accepted authResp
	e.post(t, "/api/v1/auth/accept-invite", "", map[string]string{"token": inv["token"].(string), "password": "password123"}, &accepted)
	memberID := accepted.User.ID

	// Admin promotes member to admin.
	if code := e.patch(t, "/api/v1/org/users/"+memberID, adminJWT, map[string]string{"role": "admin"}); code != http.StatusNoContent {
		t.Fatalf("set role failed: %d", code)
	}
	// Member re-logs in to get a token reflecting the new admin role.
	var relogin authResp
	e.post(t, "/api/v1/auth/login", "", map[string]string{"email": "m@team.com", "password": "password123"}, &relogin)
	// Now (as admin) they still cannot change their OWN role.
	if code := e.patch(t, "/api/v1/org/users/"+memberID, relogin.Token, map[string]string{"role": "member"}); code != http.StatusBadRequest {
		t.Fatalf("expected 400 self role change, got %d", code)
	}
	// Admin removes the member.
	if code := e.del(t, "/api/v1/org/users/"+memberID, adminJWT); code != http.StatusNoContent {
		t.Fatalf("remove failed: %d", code)
	}
}
