package server

import (
	"encoding/base64"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/avi-pathak/mission-control.ai/internal/protocol"
)

// TestTerminalOpenRoutesToAgentAndBack verifies a dashboard can open a PTY on
// its org's agent and receives output; and that another org cannot.
func TestTerminalInteractiveFlow(t *testing.T) {
	e := newTestEnv(t)
	jwt := e.register(t, "a@acme.com")
	res := e.enrollAgent(t, jwt)

	agent := e.dialAgent(t, res.AgentKey, res.AgentID)
	defer agent.Close()
	// Register a machine via a session upsert so orgOwnsMachine passes.
	writeWS(t, agent, protocol.TypeSessionUpsert, protocol.SessionUpsert{
		Session: protocol.Session{ID: "s1", MachineID: res.AgentID, Status: protocol.StatusRunning},
	})
	time.Sleep(100 * time.Millisecond)

	dash := e.dialDashboard(t, jwt)
	defer dash.Close()
	drainSnapshot(t, dash)

	// Dashboard opens a terminal on the agent's machine.
	writeWS(t, dash, protocol.TypeTerminalOpen, protocol.TerminalOpen{
		PTYID: "pty1", MachineID: res.AgentID, Provider: "claude-code", CWD: "/tmp", Cols: 80, Rows: 24,
	})

	// The agent should receive the terminal.open (relayed by the server).
	got := readUntil(t, agent, protocol.TypeTerminalOpen, 2*time.Second)
	if got == nil {
		t.Fatal("agent never received terminal.open")
	}
	p, _ := protocol.As[protocol.TerminalOpen](got)
	if p.PTYID != "pty1" || p.MachineID != res.AgentID {
		t.Fatalf("bad relayed open: %+v", p)
	}

	// Agent emits output → dashboard should receive it (org-scoped).
	writeWS(t, agent, protocol.TypeTerminalOutput, protocol.TerminalOutput{
		PTYID: "pty1", Data: base64.StdEncoding.EncodeToString([]byte("hi")),
	})
	out := readUntil(t, dash, protocol.TypeTerminalOutput, 2*time.Second)
	if out == nil {
		t.Fatal("dashboard never received terminal.output")
	}
	op, _ := protocol.As[protocol.TerminalOutput](out)
	data, _ := base64.StdEncoding.DecodeString(op.Data)
	if string(data) != "hi" {
		t.Fatalf("bad output: %q", data)
	}
}

func TestTerminalOpenRejectedCrossOrg(t *testing.T) {
	e := newTestEnv(t)
	jwtA := e.register(t, "a@a.com")
	resA := e.enrollAgent(t, jwtA)
	agentA := e.dialAgent(t, resA.AgentKey, resA.AgentID)
	defer agentA.Close()
	writeWS(t, agentA, protocol.TypeSessionUpsert, protocol.SessionUpsert{
		Session: protocol.Session{ID: "s1", MachineID: resA.AgentID, Status: protocol.StatusRunning},
	})
	time.Sleep(100 * time.Millisecond)

	// Org B tries to open a terminal on org A's machine.
	jwtB := e.register(t, "b@b.com")
	dashB := e.dialDashboard(t, jwtB)
	defer dashB.Close()
	drainSnapshot(t, dashB)
	writeWS(t, dashB, protocol.TypeTerminalOpen, protocol.TerminalOpen{
		PTYID: "ptyX", MachineID: resA.AgentID, Provider: "claude-code", CWD: "/tmp", Cols: 80, Rows: 24,
	})

	// Org B should get a terminal.opened with ok=false; agent A must NOT get the open.
	res := readUntil(t, dashB, protocol.TypeTerminalOpened, 2*time.Second)
	if res == nil {
		t.Fatal("expected rejection terminal.opened")
	}
	op, _ := protocol.As[protocol.TerminalOpened](res)
	if op.OK {
		t.Fatal("cross-org terminal.open should be rejected")
	}
}

// helpers

func drainSnapshot(t *testing.T, c *websocket.Conn) {
	t.Helper()
	_ = c.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, _, _ = c.ReadMessage() // first frame is the snapshot
}

// readUntil reads frames until one of msgType arrives or the deadline passes.
func readUntil(t *testing.T, c *websocket.Conn, msgType string, within time.Duration) *protocol.Envelope {
	t.Helper()
	deadline := time.Now().Add(within)
	for time.Now().Before(deadline) {
		_ = c.SetReadDeadline(time.Now().Add(within))
		_, data, err := c.ReadMessage()
		if err != nil {
			return nil
		}
		env, err := protocol.Decode(data)
		if err != nil {
			continue
		}
		if env.Type == msgType {
			return env
		}
	}
	return nil
}
