package server

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/avi-pathak/mission-control.ai/internal/auth"
	"github.com/avi-pathak/mission-control.ai/internal/config"
	"github.com/avi-pathak/mission-control.ai/internal/protocol"
	"github.com/avi-pathak/mission-control.ai/internal/store"
	"go.uber.org/zap"
)

// testEnv is a running server with an httptest server in front of it.
type testEnv struct {
	srv    *Server
	ts     *httptest.Server
	wsBase string
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	st, err := store.Open(":memory:", zap.NewNop())
	if err != nil {
		t.Fatal(err)
	}
	srv := New(config.Server{JWTSecret: "test-secret"}, zap.NewNop(), st)
	go srv.hub.Run()
	ts := httptest.NewServer(srv.router())
	t.Cleanup(ts.Close)
	return &testEnv{srv: srv, ts: ts, wsBase: "ws" + strings.TrimPrefix(ts.URL, "http")}
}

// register creates an ACTIVE org+owner directly via the store (bypassing the
// pending-approval signup flow) and returns a JWT. Most tests just need a
// working authenticated user in their own org.
func (e *testEnv) register(t *testing.T, email string) string {
	t.Helper()
	now := time.Now()
	org, err := e.srv.store.CreateOrg(email, "org-"+email, now)
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	hash, _ := auth.HashPassword("password123")
	u, err := e.srv.store.CreateUser(store.User{
		OrgID: org.ID, Email: strings.ToLower(email), PasswordHash: hash,
		Role: auth.RoleOwner, Status: store.UserStatusActive,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	tok, err := e.srv.auth.Issue(u.ID, u.OrgID, u.Role, u.Email, now)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return tok
}

// superadmin creates an active platform-admin user and returns a JWT.
func (e *testEnv) superadmin(t *testing.T, email string) string {
	t.Helper()
	now := time.Now()
	org, err := e.srv.store.CreateOrg(email, "org-"+email, now)
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	hash, _ := auth.HashPassword("password123")
	u, err := e.srv.store.CreateUser(store.User{
		OrgID: org.ID, Email: strings.ToLower(email), PasswordHash: hash,
		Role: auth.RoleOwner, Status: store.UserStatusActive, PlatformAdmin: true,
	})
	if err != nil {
		t.Fatalf("create superadmin: %v", err)
	}
	tok, err := e.srv.auth.IssueWithAdmin(u.ID, u.OrgID, u.Role, u.Email, true, now)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return tok
}

// mintEnrollAndKey registers an org (if token given uses it), creates an enroll
// token, and enrolls to obtain an agent key + id.
func (e *testEnv) enrollAgent(t *testing.T, jwt string) protocol.EnrollResponse {
	t.Helper()
	var tok map[string]any
	e.post(t, "/api/v1/enroll-tokens", jwt, map[string]any{"label": "test"}, &tok)
	var res protocol.EnrollResponse
	e.post(t, "/api/v1/enroll", "", protocol.EnrollRequest{
		Token: tok["token"].(string), Hostname: "h1", OS: "linux", Arch: "amd64",
	}, &res)
	if res.AgentKey == "" {
		t.Fatal("enroll returned no key")
	}
	return res
}

func (e *testEnv) post(t *testing.T, path, jwt string, body, out any) int {
	t.Helper()
	buf, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", e.ts.URL+path, bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	if jwt != "" {
		req.Header.Set("Authorization", "Bearer "+jwt)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		_ = json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode
}

func (e *testEnv) get(t *testing.T, path, jwt string, out any) int {
	t.Helper()
	req, _ := http.NewRequest("GET", e.ts.URL+path, nil)
	if jwt != "" {
		req.Header.Set("Authorization", "Bearer "+jwt)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if out != nil {
		_ = json.NewDecoder(resp.Body).Decode(out)
	}
	return resp.StatusCode
}

func (e *testEnv) dialAgent(t *testing.T, key, agentID string) *websocket.Conn {
	t.Helper()
	c, _, err := websocket.DefaultDialer.Dial(e.wsBase+"/ws?role=agent", nil)
	if err != nil {
		t.Fatal(err)
	}
	writeWS(t, c, protocol.TypeAgentHello, protocol.AgentHello{APIKey: key, AgentID: agentID, Hostname: "h1"})
	return c
}

func (e *testEnv) dialDashboard(t *testing.T, jwt string) *websocket.Conn {
	t.Helper()
	c, _, err := websocket.DefaultDialer.Dial(e.wsBase+"/ws?role=dashboard&token="+jwt, nil)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func writeWS(t *testing.T, c *websocket.Conn, msgType string, payload any) {
	t.Helper()
	data, _ := protocol.Encode(msgType, payload)
	if err := c.WriteMessage(websocket.TextMessage, data); err != nil {
		t.Fatal(err)
	}
}

func readWS(t *testing.T, c *websocket.Conn) *protocol.Envelope {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, data, err := c.ReadMessage()
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	env, _ := protocol.Decode(data)
	return env
}

// registerActiveLogin creates an active user with a known password (via store)
// and returns a JWT — used by change-password tests.
func (e *testEnv) registerActiveLogin(t *testing.T, email, password string) string {
	t.Helper()
	now := time.Now()
	org, err := e.srv.store.CreateOrg(email, "org-"+email, now)
	if err != nil {
		t.Fatalf("create org: %v", err)
	}
	hash, _ := auth.HashPassword(password)
	u, err := e.srv.store.CreateUser(store.User{
		OrgID: org.ID, Email: strings.ToLower(email), PasswordHash: hash,
		Role: auth.RoleAdmin, Status: store.UserStatusActive,
	})
	if err != nil {
		t.Fatalf("create user: %v", err)
	}
	tok, _ := e.srv.auth.Issue(u.ID, u.OrgID, u.Role, u.Email, now)
	return tok
}

func (e *testEnv) patch(t *testing.T, path, jwt string, body any) int {
	t.Helper()
	buf, _ := json.Marshal(body)
	req, _ := http.NewRequest("PATCH", e.ts.URL+path, bytes.NewReader(buf))
	req.Header.Set("Content-Type", "application/json")
	if jwt != "" {
		req.Header.Set("Authorization", "Bearer "+jwt)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}

func (e *testEnv) del(t *testing.T, path, jwt string) int {
	t.Helper()
	req, _ := http.NewRequest("DELETE", e.ts.URL+path, nil)
	if jwt != "" {
		req.Header.Set("Authorization", "Bearer "+jwt)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	return resp.StatusCode
}
