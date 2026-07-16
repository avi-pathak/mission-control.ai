package server

import (
	"net/http"

	"github.com/gorilla/websocket"
	"github.com/avi-pathak/mission-control.ai/internal/hub"
	"github.com/avi-pathak/mission-control.ai/internal/protocol"
	"go.uber.org/zap"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  4096,
	WriteBufferSize: 4096,
	CheckOrigin:     func(r *http.Request) bool { return true },
}

// handleWS upgrades a connection and negotiates role. Agents authenticate via
// the agent.hello key (mapping to an org); dashboards authenticate via a JWT.
func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	role := protocol.Role(r.URL.Query().Get("role"))
	if role != protocol.RoleAgent && role != protocol.RoleDashboard {
		writeError(w, http.StatusBadRequest, "bad_role", "role must be agent or dashboard")
		return
	}

	// Dashboards authenticate (JWT) before upgrade to resolve their org.
	var dashOrg string
	if role == protocol.RoleDashboard {
		claims, err := s.auth.Verify(bearer(r))
		if err != nil {
			writeError(w, http.StatusUnauthorized, "unauthorized", "invalid token")
			return
		}
		dashOrg = claims.OrgID
	}

	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		s.log.Warn("ws upgrade failed", zap.Error(err))
		return
	}

	if role == protocol.RoleDashboard {
		c := s.hub.NewClient(conn, protocol.RoleDashboard, dashOrg, "")
		// Enqueue the org snapshot BEFORE registering so it is the first frame.
		c.Send(protocol.TypeSnapshot, s.state.Snapshot(dashOrg))
		s.hub.Register(c)
		return
	}

	// Agent: read the first message, expect agent.hello for auth + identity.
	_, data, err := conn.ReadMessage()
	if err != nil {
		_ = conn.Close()
		return
	}
	env, err := protocol.Decode(data)
	if err != nil || env.Type != protocol.TypeAgentHello {
		_ = conn.Close()
		return
	}
	hello, err := protocol.As[protocol.AgentHello](env)
	if err != nil {
		_ = conn.Close()
		return
	}
	info, ok := s.lookupAgentKey(hello.APIKey)
	if !ok {
		s.log.Warn("agent auth failed", zap.String("host", hello.Hostname))
		_ = conn.Close()
		return
	}

	c := s.hub.NewClient(conn, protocol.RoleAgent, info.OrgID, hello.AgentID)
	s.onAgentHello(c, hello)
	s.hub.Register(c)
}

// registerHubHandlers wires hub callbacks to state + store mutations.
func (s *Server) registerHubHandlers() {
	s.hub.OnAgentMessage = s.onAgentMessage
	s.hub.OnDashboardMessage = s.onDashboardMessage
	s.hub.OnUnregister = s.onUnregister
}

func (s *Server) onAgentHello(c *hub.Client, hello protocol.AgentHello) {
	m := protocol.Machine{
		ID:           hello.AgentID,
		Hostname:     hello.Hostname,
		OS:           hello.OS,
		Arch:         hello.Arch,
		CPUCores:     hello.CPUCores,
		TotalMem:     hello.TotalMem,
		AgentVersion: hello.AgentVersion,
		Online:       true,
		LastSeenAt:   protocol.NowMillis(),
	}
	s.state.UpsertMachine(c.OrgID, m)
	_ = s.store.UpsertMachine(c.OrgID, m)
	s.hub.EmitOrg(c.OrgID, protocol.TypeMachineUpsert, protocol.MachineUpsert{Machine: m})
	s.log.Info("agent connected", zap.String("host", hello.Hostname), zap.String("id", hello.AgentID))
	s.emitServerEvent(c.OrgID, hello.AgentID, "", protocol.EventAgentConnected,
		"Agent connected · "+hello.Hostname, protocol.SeveritySuccess)
}

func (s *Server) onUnregister(c *hub.Client) {
	if c.Role != protocol.RoleAgent || c.MachineID == "" {
		return
	}
	if m, ok := s.state.SetMachineOnline(c.OrgID, c.MachineID, false); ok {
		_ = s.store.UpsertMachine(c.OrgID, m)
		s.hub.EmitOrg(c.OrgID, protocol.TypeAgentStatus, protocol.AgentStatus{MachineID: c.MachineID, Online: false})
		s.hub.EmitOrg(c.OrgID, protocol.TypeMachineUpsert, protocol.MachineUpsert{Machine: m})
		s.emitServerEvent(c.OrgID, c.MachineID, "", protocol.EventAgentOffline,
			"Agent disconnected · "+m.Hostname, protocol.SeverityWarn)
	}
	for _, id := range s.state.RemoveMachineSessions(c.OrgID, c.MachineID) {
		s.hub.EmitOrg(c.OrgID, protocol.TypeSessionRemoved, protocol.SessionRemoved{SessionID: id})
	}
	s.log.Info("agent disconnected", zap.String("id", c.MachineID))
}
