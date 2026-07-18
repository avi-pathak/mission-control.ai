package agent

import (
	"bytes"
	"crypto/tls"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Publish uploads a file to the server's /api/v1/publish endpoint using the
// agent's durable key. sessionID is optional.
func Publish(serverURL, apiKey, path, sessionID string, insecure bool) error {
	if apiKey == "" {
		return fmt.Errorf("agent is not enrolled (no apiKey); run the agent once to enroll first")
	}
	base, err := httpBaseFromServerURL(serverURL)
	if err != nil {
		return err
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return err
	}

	req, err := http.NewRequest("POST", base+"/api/v1/publish", bytes.NewReader(data))
	if err != nil {
		return err
	}
	req.Header.Set("X-API-Key", apiKey)
	req.Header.Set("X-MC-Filename", filepath.Base(path))
	if sessionID != "" {
		req.Header.Set("X-MC-Session", sessionID)
	}
	req.Header.Set("Content-Type", "application/octet-stream")

	client := &http.Client{Timeout: 60 * time.Second}
	if insecure {
		client.Transport = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}} //nolint:gosec
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusCreated {
		msg, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("publish failed (%d): %s", resp.StatusCode, string(msg))
	}
	return nil
}
