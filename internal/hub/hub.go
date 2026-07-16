// Package hub implements the WebSocket fan-out hub connecting agents and
// dashboards through the server.
package hub

import (
	"sync"

	"github.com/gorilla/websocket"
	"github.com/avi-pathak/mission-control.ai/internal/protocol"
	"go.uber.org/zap"
)

// Client is a single WebSocket connection (agent or dashboard).
type Client struct {
	ID        string
	Role      protocol.Role
	OrgID     string // tenant this connection belongs to
	MachineID string // set for agent clients
	conn      *websocket.Conn
	send      chan []byte
	hub       *Hub
	log       *zap.Logger
	closeOnce sync.Once
}

// Hub maintains the set of connected clients and routes messages.
type Hub struct {
	mu         sync.RWMutex
	clients    map[string]*Client
	agentsByID map[string]*Client // machineID -> agent client

	register   chan *Client
	unregister chan *Client
	log        *zap.Logger

	// OnAgentMessage is invoked for every inbound message from an agent.
	OnAgentMessage func(c *Client, env *protocol.Envelope)
	// OnDashboardMessage is invoked for every inbound message from a dashboard.
	OnDashboardMessage func(c *Client, env *protocol.Envelope)
	// OnRegister / OnUnregister fire on connect/disconnect lifecycle.
	OnRegister   func(c *Client)
	OnUnregister func(c *Client)
}

// New creates a Hub.
func New(log *zap.Logger) *Hub {
	return &Hub{
		clients:    make(map[string]*Client),
		agentsByID: make(map[string]*Client),
		register:   make(chan *Client),
		unregister: make(chan *Client),
		log:        log,
	}
}

// Run processes register/unregister events until ctx-driven stop via Close.
func (h *Hub) Run() {
	for {
		select {
		case c := <-h.register:
			h.mu.Lock()
			h.clients[c.ID] = c
			if c.Role == protocol.RoleAgent && c.MachineID != "" {
				h.agentsByID[c.MachineID] = c
			}
			h.mu.Unlock()
			if h.OnRegister != nil {
				h.OnRegister(c)
			}
		case c := <-h.unregister:
			h.mu.Lock()
			if _, ok := h.clients[c.ID]; ok {
				delete(h.clients, c.ID)
				if c.Role == protocol.RoleAgent && h.agentsByID[c.MachineID] == c {
					delete(h.agentsByID, c.MachineID)
				}
				close(c.send)
			}
			h.mu.Unlock()
			if h.OnUnregister != nil {
				h.OnUnregister(c)
			}
		}
	}
}

// BroadcastOrg sends raw bytes to every connected dashboard in an org.
func (h *Hub) BroadcastOrg(orgID string, msg []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, c := range h.clients {
		if c.Role == protocol.RoleDashboard && c.OrgID == orgID {
			c.enqueue(msg)
		}
	}
}

// SendToAgent routes a message to the agent owning machineID. Returns false if
// no such agent is connected.
func (h *Hub) SendToAgent(machineID string, msg []byte) bool {
	h.mu.RLock()
	c, ok := h.agentsByID[machineID]
	h.mu.RUnlock()
	if !ok {
		return false
	}
	c.enqueue(msg)
	return true
}

// SendToOrgAgents delivers a raw message to every connected agent in an org.
// Agents ignore messages for ptyIds they don't own, so this is a safe way to
// route terminal input without the server tracking pty→machine.
func (h *Hub) SendToOrgAgents(orgID string, msg []byte) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, c := range h.clients {
		if c.Role == protocol.RoleAgent && c.OrgID == orgID {
			c.enqueue(msg)
		}
	}
}

// EmitOrg encodes and broadcasts a typed message to an org's dashboards.
func (h *Hub) EmitOrg(orgID, msgType string, payload any) {
	data, err := protocol.Encode(msgType, payload)
	if err != nil {
		h.log.Error("encode dashboard message", zap.Error(err))
		return
	}
	h.BroadcastOrg(orgID, data)
}
