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

// register creates an org+owner and returns the JWT.
func (e *testEnv) register(t *testing.T, email string) string {
	t.Helper()
	var out authResp
	e.post(t, "/api/auth/register", "", map[string]string{
		"email": email, "password": "password123", "orgName": email,
	}, &out)
	if out.Token == "" {
		t.Fatal("no token from register")
	}
	return out.Token
}

// mintEnrollAndKey registers an org (if token given uses it), creates an enroll
// token, and enrolls to obtain an agent key + id.
func (e *testEnv) enrollAgent(t *testing.T, jwt string) protocol.EnrollResponse {
	t.Helper()
	var tok map[string]any
	e.post(t, "/api/enroll-tokens", jwt, map[string]any{"label": "test"}, &tok)
	var res protocol.EnrollResponse
	e.post(t, "/api/enroll", "", protocol.EnrollRequest{
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
