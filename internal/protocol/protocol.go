// Package protocol defines the versioned JSON wire format exchanged over the
// Mission Control WebSocket between agents, the server, and dashboards.
//
// The envelope is intentionally flat and free of behaviour so it can be
// re-expressed as protobuf/gRPC in the future without changing semantics.
// The canonical TypeScript mirror lives in packages/protocol; keep both in
// sync. docs/PROTOCOL.md is the human-readable spec.
package protocol

import "encoding/json"

// Version is the current protocol version. Bump on breaking envelope changes.
const Version = 1

// Message types. Prefix indicates the primary direction, but the server is
// always one participant.
const (
	// Agent -> Server
	TypeAgentHello     = "agent.hello"
	TypeAgentHeartbeat = "agent.heartbeat"
	TypeSessionUpsert  = "session.upsert"
	TypeSessionRemoved = "session.removed"
	TypeLogAppend      = "log.append"
	TypeMetricSample   = "metric.sample"
	TypeCommandAck     = "command.ack"
	TypeCommandResult  = "command.result"
	TypeEventReport    = "event.report"

	// Server -> Agent
	TypeCommand = "command"

	// Server -> Dashboard
	TypeSnapshot      = "snapshot"
	TypeMachineUpsert = "machine.upsert"
	TypeMachineRemoved = "machine.removed"
	TypeAgentStatus   = "agent.status"
	TypeEventAppend   = "event.append"

	// Interactive terminal (dashboard <-> agent, relayed by server).
	// Dashboard -> Agent
	TypeTerminalOpen   = "terminal.open"
	TypeTerminalAttach = "terminal.attach"
	TypeTerminalInput  = "terminal.input"
	TypeTerminalResize = "terminal.resize"
	TypeTerminalClose  = "terminal.close"
	// Agent -> Dashboard
	TypeTerminalOutput = "terminal.output"
	TypeTerminalOpened = "terminal.opened"
	TypeTerminalExit   = "terminal.exit"

	// Either
	TypeError = "error"
)

// Role identifies how a WebSocket connection participates.
type Role string

const (
	RoleAgent     Role = "agent"
	RoleDashboard Role = "dashboard"
)

// Envelope wraps every message on the wire.
type Envelope struct {
	V       int             `json:"v"`
	Type    string          `json:"type"`
	TS      int64           `json:"ts"`             // unix millis
	ID      string          `json:"id,omitempty"`   // optional correlation id
	Payload json.RawMessage `json:"payload"`
}
