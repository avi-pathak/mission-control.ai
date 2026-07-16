package hub

import (
	"time"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"github.com/avi-pathak/mission-control.ai/internal/protocol"
	"go.uber.org/zap"
)

const (
	writeWait      = 10 * time.Second
	pongWait       = 60 * time.Second
	pingPeriod     = (pongWait * 9) / 10
	maxMessageSize = 1 << 20 // 1 MiB
	sendBuffer     = 256
)

// NewClient constructs a Client bound to a websocket connection.
func (h *Hub) NewClient(conn *websocket.Conn, role protocol.Role, orgID, machineID string) *Client {
	return &Client{
		ID:        uuid.NewString(),
		Role:      role,
		OrgID:     orgID,
		MachineID: machineID,
		conn:      conn,
		send:      make(chan []byte, sendBuffer),
		hub:       h,
		log:       h.log,
	}
}

// Register adds the client to the hub and starts its pumps.
func (h *Hub) Register(c *Client) {
	h.register <- c
	go c.writePump()
	go c.readPump()
}

func (c *Client) enqueue(msg []byte) {
	select {
	case c.send <- msg:
	default:
		// Slow consumer: drop the connection to protect the hub.
		c.log.Warn("client send buffer full, dropping", zap.String("id", c.ID))
		c.close()
	}
}

// Send encodes and enqueues a typed message directly to this client.
func (c *Client) Send(msgType string, payload any) {
	data, err := protocol.Encode(msgType, payload)
	if err != nil {
		c.log.Error("encode client message", zap.Error(err))
		return
	}
	c.enqueue(data)
}

func (c *Client) close() {
	c.closeOnce.Do(func() {
		_ = c.conn.Close()
	})
}

func (c *Client) readPump() {
	defer func() {
		c.hub.unregister <- c
		c.close()
	}()
	c.conn.SetReadLimit(maxMessageSize)
	_ = c.conn.SetReadDeadline(time.Now().Add(pongWait))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongWait))
	})
	for {
		_, data, err := c.conn.ReadMessage()
		if err != nil {
			if websocket.IsUnexpectedCloseError(err, websocket.CloseGoingAway, websocket.CloseNormalClosure) {
				c.log.Debug("ws read error", zap.Error(err))
			}
			return
		}
		env, err := protocol.Decode(data)
		if err != nil {
			c.log.Warn("bad envelope", zap.Error(err))
			continue
		}
		switch c.Role {
		case protocol.RoleAgent:
			if c.hub.OnAgentMessage != nil {
				c.hub.OnAgentMessage(c, env)
			}
		case protocol.RoleDashboard:
			if c.hub.OnDashboardMessage != nil {
				c.hub.OnDashboardMessage(c, env)
			}
		}
	}
}

func (c *Client) writePump() {
	ticker := time.NewTicker(pingPeriod)
	defer func() {
		ticker.Stop()
		c.close()
	}()
	for {
		select {
		case msg, ok := <-c.send:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if !ok {
				_ = c.conn.WriteMessage(websocket.CloseMessage, []byte{})
				return
			}
			if err := c.conn.WriteMessage(websocket.TextMessage, msg); err != nil {
				return
			}
		case <-ticker.C:
			_ = c.conn.SetWriteDeadline(time.Now().Add(writeWait))
			if err := c.conn.WriteMessage(websocket.PingMessage, nil); err != nil {
				return
			}
		}
	}
}
