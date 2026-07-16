// Package config loads server configuration from YAML with env overrides.
package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

// TLS holds optional TLS certificate paths.
type TLS struct {
	Enabled  bool   `yaml:"enabled"`
	CertFile string `yaml:"certFile"`
	KeyFile  string `yaml:"keyFile"`
}

// Retention controls how long historical rows are kept.
type Retention struct {
	LogHours    int `yaml:"logHours"`
	MetricHours int `yaml:"metricHours"`
}

// Server is the top-level server configuration.
type Server struct {
	ListenAddr  string    `yaml:"listenAddr"`
	PublicURL   string    `yaml:"publicUrl"`
	DatabaseURL string    `yaml:"databaseUrl"` // postgres:// DSN, or a sqlite file path
	JWTSecret   string    `yaml:"jwtSecret"`
	AdminEmail  string    `yaml:"adminEmail"`
	AdminPass   string    `yaml:"adminPassword"`
	StaticDir   string    `yaml:"staticDir"`   // dir of the built dashboard SPA to serve
	AgentBinDir string    `yaml:"agentBinDir"` // dir of prebuilt agent binaries to serve at /download
	CORSOrigins []string  `yaml:"corsOrigins"`
	TLS         TLS       `yaml:"tls"`
	Retention   Retention `yaml:"retention"`
	LogLevel    string    `yaml:"logLevel"`
}

// Default returns a config with sensible defaults.
func Default() Server {
	return Server{
		ListenAddr:  ":8080",
		DatabaseURL: "mission-control.db",
		CORSOrigins: []string{"*"},
		Retention:   Retention{LogHours: 72, MetricHours: 72},
		LogLevel:    "info",
	}
}

// Load reads a YAML config file, applying defaults and env overrides.
// Env: MC_LISTEN_ADDR, MC_PUBLIC_URL, DATABASE_URL, JWT_SECRET, ADMIN_EMAIL,
// ADMIN_PASSWORD, MC_LOG_LEVEL.
func Load(path string) (Server, error) {
	cfg := Default()
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return cfg, err
		}
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return cfg, err
		}
	}
	if v := os.Getenv("MC_LISTEN_ADDR"); v != "" {
		cfg.ListenAddr = v
	}
	if v := os.Getenv("DATABASE_URL"); v != "" {
		cfg.DatabaseURL = v
	}
	if v := os.Getenv("MC_PUBLIC_URL"); v != "" {
		cfg.PublicURL = v
	}
	if v := os.Getenv("JWT_SECRET"); v != "" {
		cfg.JWTSecret = v
	}
	if v := os.Getenv("ADMIN_EMAIL"); v != "" {
		cfg.AdminEmail = v
	}
	if v := os.Getenv("ADMIN_PASSWORD"); v != "" {
		cfg.AdminPass = v
	}
	if v := os.Getenv("MC_LOG_LEVEL"); v != "" {
		cfg.LogLevel = v
	}
	if v := os.Getenv("MC_STATIC_DIR"); v != "" {
		cfg.StaticDir = v
	}
	if v := os.Getenv("MC_AGENT_BIN_DIR"); v != "" {
		cfg.AgentBinDir = v
	}
	return cfg, nil
}
