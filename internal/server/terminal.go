package server

import (
	"encoding/json"
	"sync"

	"github.com/avi-pathak/mission-control.ai/internal/hub"
	"github.com/avi-pathak/mission-control.ai/internal/protocol"
)

// orgOwnsMachine reports whether a machine belongs to the org (from live state).
func (s *Server) orgOwnsMachine(orgID, machineID string) bool {
	for _, m := range s.state.Snapshot(orgID).Machines {
		if m.ID == machineID {
			return true
		}
	}
	return false
}

// forwardToAgent relays a raw envelope to the agent owning machineID.
func (s *Server) forwardToAgent(machineID string, env *protocol.Envelope) {
	s.hub.SendToAgent(machineID, mustRaw(env))
}

// forwardToOrgAgents relays a raw envelope to every agent in the org.
func (s *Server) forwardToOrgAgents(orgID string, env *protocol.Envelope) {
	s.hub.SendToOrgAgents(orgID, mustRaw(env))
}

// mustRaw re-serializes an envelope to bytes for forwarding, preserving the
// original payload (which is already JSON).
func mustRaw(env *protocol.Envelope) []byte {
	data, err := json.Marshal(env)
	if err != nil {
		return nil
	}
	return data
}

// terminalRouter tracks which org owns each live PTY so agent output is fanned
// back only to that org's dashboards.
type terminalRouter struct {
	mu  sync.RWMutex
	org map[string]string // ptyId -> orgID
}

func newTerminalRouter() *terminalRouter {
	return &terminalRouter{org: make(map[string]string)}
}

func (t *terminalRouter) bind(ptyID, orgID string) {
	t.mu.Lock()
	t.org[ptyID] = orgID
	t.mu.Unlock()
}

func (t *terminalRouter) orgOf(ptyID string) (string, bool) {
	t.mu.RLock()
	o, ok := t.org[ptyID]
	t.mu.RUnlock()
	return o, ok
}

func (t *terminalRouter) release(ptyID string) {
	t.mu.Lock()
	delete(t.org, ptyID)
	t.mu.Unlock()
}

// onDashboardMessage relays interactive-terminal messages from a dashboard to
// the agent that owns the target machine, enforcing org ownership.
func (s *Server) onDashboardMessage(c *hub.Client, env *protocol.Envelope) {
	switch env.Type {
	case protocol.TypeTerminalOpen:
		p, err := protocol.As[protocol.TerminalOpen](env)
		if err != nil {
			return
		}
		if !s.orgOwnsMachine(c.OrgID, p.MachineID) {
			c.Send(protocol.TypeTerminalOpened, protocol.TerminalOpened{
				PTYID: p.PTYID, OK: false, Error: "machine not found in your org",
			})
			return
		}
		s.termRouter.bind(p.PTYID, c.OrgID)
		s.forwardToAgent(p.MachineID, env)

	case protocol.TypeTerminalAttach:
		p, err := protocol.As[protocol.TerminalAttach](env)
		if err != nil {
			return
		}
		if !s.orgOwnsMachine(c.OrgID, p.MachineID) {
			return
		}
		s.termRouter.bind(p.PTYID, c.OrgID)
		s.forwardToAgent(p.MachineID, env)

	case protocol.TypeTerminalInput, protocol.TypeTerminalResize, protocol.TypeTerminalClose:
		// Route by ptyId → the machine is resolved via the bound org; we forward
		// to the agent that owns the pty. We look up the pty's machine from the
		// live agent set by broadcasting to the owning agent.
		ptyID := terminalPTYID(env)
		if ptyID == "" {
			return
		}
		org, ok := s.termRouter.orgOf(ptyID)
		if !ok || org != c.OrgID {
			return // not this org's pty
		}
		// Forward to whichever agent in the org owns it. Agents ignore unknown
		// ptyIds, so a targeted broadcast to the org's agents is safe & simple.
		s.forwardToOrgAgents(org, env)
		if env.Type == protocol.TypeTerminalClose {
			s.termRouter.release(ptyID)
		}
	}
}

// terminalPTYID extracts the ptyId from an input/resize/close envelope.
func terminalPTYID(env *protocol.Envelope) string {
	switch env.Type {
	case protocol.TypeTerminalInput:
		if p, err := protocol.As[protocol.TerminalInput](env); err == nil {
			return p.PTYID
		}
	case protocol.TypeTerminalResize:
		if p, err := protocol.As[protocol.TerminalResize](env); err == nil {
			return p.PTYID
		}
	case protocol.TypeTerminalClose:
		if p, err := protocol.As[protocol.TerminalClose](env); err == nil {
			return p.PTYID
		}
	}
	return ""
}

// handleAgentTerminalOutput fans agent PTY output/lifecycle back to the org's
// dashboards. Called from the agent message handler.
func (s *Server) handleAgentTerminalOutput(env *protocol.Envelope) {
	ptyID := agentTerminalPTYID(env)
	if ptyID == "" {
		return
	}
	org, ok := s.termRouter.orgOf(ptyID)
	if !ok {
		return
	}
	s.hub.BroadcastOrg(org, mustRaw(env))
	if env.Type == protocol.TypeTerminalExit {
		s.termRouter.release(ptyID)
	}
}

func agentTerminalPTYID(env *protocol.Envelope) string {
	switch env.Type {
	case protocol.TypeTerminalOutput:
		if p, err := protocol.As[protocol.TerminalOutput](env); err == nil {
			return p.PTYID
		}
	case protocol.TypeTerminalOpened:
		if p, err := protocol.As[protocol.TerminalOpened](env); err == nil {
			return p.PTYID
		}
	case protocol.TypeTerminalExit:
		if p, err := protocol.As[protocol.TerminalExit](env); err == nil {
			return p.PTYID
		}
	}
	return ""
}
