// Package config (agent.go) loads agent configuration.
package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// Agent is the agent daemon configuration.
type Agent struct {
	ServerURL     string   `yaml:"serverUrl"`     // ws(s) or http(s); http(s) is auto-derived for REST
	APIKey        string   `yaml:"apiKey"`        // durable agent key (set after enrollment)
	EnrollToken   string   `yaml:"enrollToken"`   // one-time token; exchanged for APIKey on first run
	AgentID       string   `yaml:"agentId"`       // stable id; from enrollment or derived
	HostnameOverride string `yaml:"hostname"`
	Providers     []string `yaml:"providers"`
	DiscoverEvery int      `yaml:"discoverEverySeconds"`
	MetricsEvery  int      `yaml:"metricsEverySeconds"`
	HeartbeatEvery int     `yaml:"heartbeatEverySeconds"`
	InsecureTLS   bool     `yaml:"insecureSkipVerify"`
	LogLevel      string   `yaml:"logLevel"`

	// path is the file this config was loaded from, used to persist credentials
	// obtained during enrollment. Not serialized.
	path string `yaml:"-"`
}

// Path returns the file this config was loaded from (may be empty).
func (a Agent) Path() string { return a.path }

// DefaultAgent returns agent defaults.
func DefaultAgent() Agent {
	return Agent{
		ServerURL:      "ws://localhost:8080",
		Providers:      []string{"claude-code", "codex", "gemini"},
		DiscoverEvery:  5,
		MetricsEvery:   3,
		HeartbeatEvery: 10,
		LogLevel:       "info",
	}
}

// LoadAgent reads agent.yaml with defaults and env overrides.
// Env: MC_SERVER_URL, MC_API_KEY, MC_AGENT_ID, MC_ENROLL_TOKEN.
func LoadAgent(path string) (Agent, error) {
	cfg := DefaultAgent()
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return cfg, err
		}
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return cfg, err
		}
	}
	cfg.path = path
	if v := os.Getenv("MC_SERVER_URL"); v != "" {
		cfg.ServerURL = v
	}
	if v := os.Getenv("MC_API_KEY"); v != "" {
		cfg.APIKey = v
	}
	if v := os.Getenv("MC_AGENT_ID"); v != "" {
		cfg.AgentID = v
	}
	if v := os.Getenv("MC_ENROLL_TOKEN"); v != "" {
		cfg.EnrollToken = v
	}
	return cfg, nil
}

// SaveCredentials writes the durable apiKey + agentId (and clears the one-time
// enrollToken) back to the config file so subsequent runs reuse the key.
// No-op if the config was not loaded from a file.
func (a *Agent) SaveCredentials(apiKey, agentID string) error {
	a.APIKey = apiKey
	a.AgentID = agentID
	a.EnrollToken = ""
	if a.path == "" {
		return nil
	}
	data, err := yaml.Marshal(a)
	if err != nil {
		return err
	}
	return os.WriteFile(a.path, data, 0o600)
}
