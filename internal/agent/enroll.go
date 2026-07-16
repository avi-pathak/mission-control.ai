package agent

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"runtime"
	"strings"
	"time"

	"github.com/avi-pathak/mission-control.ai/internal/config"
	"github.com/avi-pathak/mission-control.ai/internal/protocol"
	"go.uber.org/zap"
)

// httpBaseFromServerURL converts a ws(s)/http(s) server URL into an http(s)
// base URL suitable for REST calls.
func httpBaseFromServerURL(serverURL string) (string, error) {
	u, err := url.Parse(serverURL)
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "ws":
		u.Scheme = "http"
	case "wss":
		u.Scheme = "https"
	case "http", "https":
		// keep
	default:
		return "", fmt.Errorf("unsupported server url scheme %q", u.Scheme)
	}
	u.Path = ""
	u.RawQuery = ""
	return strings.TrimRight(u.String(), "/"), nil
}

// Bootstrap ensures the agent has a durable API key. If the config has no
// apiKey but has an enrollToken, it performs the enrollment exchange, persists
// the returned credentials back to the config file, and updates cfg in place.
// It is a no-op when an apiKey is already present or no token is available.
func Bootstrap(cfg *config.Agent, hostname string, log *zap.Logger) error {
	if cfg.APIKey != "" || cfg.EnrollToken == "" {
		return nil
	}
	log.Info("enrolling with server", zap.String("server", cfg.ServerURL))
	res, err := enroll(cfg.ServerURL, cfg.EnrollToken, hostname, cfg.InsecureTLS)
	if err != nil {
		return err
	}
	// Keep the serverUrl the agent actually reached — it demonstrably works and
	// may differ from the server's advertised PublicURL (e.g. the operator used
	// host.docker.internal, a LAN IP, or a tunnel). Only fall back to the
	// advertised URL if the agent has no serverUrl configured at all.
	if cfg.ServerURL == "" && res.ServerURL != "" {
		if ws, cerr := wsFromHTTPBase(res.ServerURL); cerr == nil {
			cfg.ServerURL = ws
		}
	}
	if err := cfg.SaveCredentials(res.AgentKey, res.AgentID); err != nil {
		return fmt.Errorf("persist credentials: %w", err)
	}
	log.Info("enrollment complete", zap.String("agentId", res.AgentID))
	return nil
}

// wsFromHTTPBase converts an http(s) base into a ws(s) URL.
func wsFromHTTPBase(base string) (string, error) {
	u, err := url.Parse(base)
	if err != nil {
		return "", err
	}
	switch u.Scheme {
	case "http":
		u.Scheme = "ws"
	case "https":
		u.Scheme = "wss"
	}
	return strings.TrimRight(u.String(), "/"), nil
}

// enroll exchanges a one-time token for a durable agent key.
func enroll(serverURL, token, hostname string, insecure bool) (protocol.EnrollResponse, error) {
	base, err := httpBaseFromServerURL(serverURL)
	if err != nil {
		return protocol.EnrollResponse{}, err
	}
	body, _ := json.Marshal(protocol.EnrollRequest{
		Token:    token,
		Hostname: hostname,
		OS:       runtime.GOOS,
		Arch:     runtime.GOARCH,
	})
	client := &http.Client{Timeout: 15 * time.Second}
	if insecure {
		client.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}} //nolint:gosec
	}
	resp, err := client.Post(base+"/api/enroll", "application/json", bytes.NewReader(body))
	if err != nil {
		return protocol.EnrollResponse{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		msg, _ := io.ReadAll(resp.Body)
		return protocol.EnrollResponse{}, fmt.Errorf("enroll failed (%d): %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	var out protocol.EnrollResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return protocol.EnrollResponse{}, err
	}
	if out.AgentKey == "" {
		return protocol.EnrollResponse{}, fmt.Errorf("enroll returned empty key")
	}
	return out, nil
}
