// Package agent implements the Mission Control agent daemon runtime.
package agent

import (
	"context"
	"crypto/tls"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/avi-pathak/mission-control.ai/internal/protocol"
	"go.uber.org/zap"
)

// wsClient is a reconnecting WebSocket client to the server.
type wsClient struct {
	serverURL string
	apiKey    string
	insecure  bool
	log       *zap.Logger

	mu   sync.Mutex
	conn *websocket.Conn

	// onMessage receives server->agent messages (e.g. commands).
	onMessage func(env *protocol.Envelope)
	// onConnect fires after a successful (re)connection + hello handshake.
	onConnect func()

	hello protocol.AgentHello
}

func newWSClient(serverURL, apiKey string, insecure bool, hello protocol.AgentHello, log *zap.Logger) *wsClient {
	return &wsClient{serverURL: serverURL, apiKey: apiKey, insecure: insecure, hello: hello, log: log}
}

func (c *wsClient) dialURL() (string, error) {
	u, err := url.Parse(c.serverURL)
	if err != nil {
		return "", err
	}
	// Normalize http(s) -> ws(s).
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	}
	u.Path = "/ws"
	q := u.Query()
	q.Set("role", string(protocol.RoleAgent))
	u.RawQuery = q.Encode()
	return u.String(), nil
}

// run maintains the connection with exponential backoff until ctx is done.
func (c *wsClient) run(ctx context.Context) {
	backoff := time.Second
	const maxBackoff = 30 * time.Second
	for {
		if ctx.Err() != nil {
			return
		}
		if err := c.connect(ctx); err != nil {
			c.log.Warn("connection lost", zap.Error(err), zap.Duration("retryIn", backoff))
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < maxBackoff {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second // reset after a clean session
	}
}

func (c *wsClient) connect(ctx context.Context) error {
	dialURL, err := c.dialURL()
	if err != nil {
		return err
	}
	dialer := *websocket.DefaultDialer
	dialer.HandshakeTimeout = 10 * time.Second
	if c.insecure {
		dialer.TLSClientConfig = &tls.Config{InsecureSkipVerify: true} //nolint:gosec
	}
	conn, _, err := dialer.DialContext(ctx, dialURL, http.Header{})
	if err != nil {
		return err
	}
	c.setConn(conn)
	defer c.setConn(nil)

	// Handshake: send agent.hello first.
	if err := c.send(protocol.TypeAgentHello, c.hello); err != nil {
		_ = conn.Close()
		return err
	}
	c.log.Info("connected to server", zap.String("url", dialURL))
	if c.onConnect != nil {
		c.onConnect()
	}

	// Read loop.
	conn.SetReadLimit(1 << 20)
	for {
		if ctx.Err() != nil {
			_ = conn.Close()
			return ctx.Err()
		}
		_, data, err := conn.ReadMessage()
		if err != nil {
			return err
		}
		env, err := protocol.Decode(data)
		if err != nil {
			continue
		}
		if c.onMessage != nil {
			c.onMessage(env)
		}
	}
}

func (c *wsClient) setConn(conn *websocket.Conn) {
	c.mu.Lock()
	c.conn = conn
	c.mu.Unlock()
}

// send encodes and writes a typed message. Safe for concurrent use.
func (c *wsClient) send(msgType string, payload any) error {
	data, err := protocol.Encode(msgType, payload)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return websocket.ErrCloseSent
	}
	_ = c.conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return c.conn.WriteMessage(websocket.TextMessage, data)
}
