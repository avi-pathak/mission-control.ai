package server

import (
	"github.com/avi-pathak/mission-control.ai/internal/hub"
	"github.com/avi-pathak/mission-control.ai/internal/protocol"
	"go.uber.org/zap"
)

// onAgentMessage dispatches inbound agent messages to state/store/broadcast,
// all scoped to the agent connection's org.
func (s *Server) onAgentMessage(c *hub.Client, env *protocol.Envelope) {
	org := c.OrgID
	switch env.Type {
	case protocol.TypeAgentHeartbeat:
		hb, err := protocol.As[protocol.AgentHeartbeat](env)
		if err != nil {
			return
		}
		if m, ok := s.state.ApplyHeartbeat(org, hb, env.TS); ok {
			_ = s.store.UpsertMachine(org, m)
			s.hub.EmitOrg(org, protocol.TypeMachineUpsert, protocol.MachineUpsert{Machine: m})
		}

	case protocol.TypeSessionUpsert:
		p, err := protocol.As[protocol.SessionUpsert](env)
		if err != nil {
			return
		}
		p.Session.MachineID = c.MachineID
		sess := s.state.UpsertSession(org, p.Session)
		_ = s.store.UpsertSession(org, sess)
		s.hub.EmitOrg(org, protocol.TypeSessionUpsert, protocol.SessionUpsert{Session: sess})

	case protocol.TypeSessionRemoved:
		p, err := protocol.As[protocol.SessionRemoved](env)
		if err != nil {
			return
		}
		s.state.RemoveSession(org, p.SessionID)
		s.hub.EmitOrg(org, protocol.TypeSessionRemoved, p)

	case protocol.TypeLogAppend:
		p, err := protocol.As[protocol.LogAppend](env)
		if err != nil {
			return
		}
		_ = s.store.AppendLog(org, p)
		s.hub.EmitOrg(org, protocol.TypeLogAppend, p)

	case protocol.TypeMetricSample:
		p, err := protocol.As[protocol.MetricSample](env)
		if err != nil {
			return
		}
		_ = s.store.AppendMetric(org, p)
		if _, ok := s.state.ApplyMetric(org, p); ok {
			s.hub.EmitOrg(org, protocol.TypeMetricSample, p)
		}

	case protocol.TypeEventReport:
		p, err := protocol.As[protocol.EventReport](env)
		if err != nil {
			return
		}
		// Bind to the connection's machine + org to prevent spoofing.
		p.Event.MachineID = c.MachineID
		s.recordEvent(org, p.Event)

	case protocol.TypeCommandResult:
		p, err := protocol.As[protocol.CommandResult](env)
		if err != nil {
			return
		}
		s.log.Info("command result",
			zap.String("session", p.SessionID),
			zap.Bool("ok", p.OK),
			zap.String("err", p.Error))

	case protocol.TypeCommandAck:
		// best-effort; nothing to do currently.

	case protocol.TypeTerminalOutput, protocol.TypeTerminalOpened, protocol.TypeTerminalExit:
		// Relay interactive-terminal output back to the requesting org's dashboards.
		s.handleAgentTerminalOutput(env)

	default:
		s.log.Debug("unhandled agent message", zap.String("type", env.Type))
	}
}
