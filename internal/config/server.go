// Package config loads server configuration from YAML with env overrides.
package config

import (
	"os"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// TLS holds optional TLS certificate paths.
type TLS struct {
	Enabled  bool   `yaml:"enabled"`
	CertFile string `yaml:"certFile"`
	KeyFile  string `yaml:"keyFile"`
}

// SMTP holds optional outbound email settings. When Host is empty, email is
// disabled and invite links are only returned via the API.
type SMTP struct {
	Host     string `yaml:"host"`
	Port     int    `yaml:"port"`
	Username string `yaml:"username"`
	Password string `yaml:"password"`
	From     string `yaml:"from"`
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
	SMTP        SMTP      `yaml:"smtp"`
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

// Load reads a YAML config file, applying defaults and env overrides. Env wins
// over YAML. Recognized env vars:
//   MC_LISTEN_ADDR, MC_PUBLIC_URL, DATABASE_URL, JWT_SECRET,
//   ADMIN_EMAIL, ADMIN_PASSWORD, MC_LOG_LEVEL, MC_STATIC_DIR, MC_AGENT_BIN_DIR,
//   MC_CORS_ORIGINS, MC_RETENTION_LOG_HOURS, MC_RETENTION_METRIC_HOURS,
//   MC_SMTP_HOST, MC_SMTP_PORT, MC_SMTP_USER, MC_SMTP_PASS, MC_SMTP_FROM
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
	if v := os.Getenv("MC_SMTP_HOST"); v != "" {
		cfg.SMTP.Host = v
	}
	if v := os.Getenv("MC_SMTP_PORT"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.SMTP.Port = n
		}
	}
	if v := os.Getenv("MC_SMTP_USER"); v != "" {
		cfg.SMTP.Username = v
	}
	if v := os.Getenv("MC_SMTP_PASS"); v != "" {
		cfg.SMTP.Password = v
	}
	if v := os.Getenv("MC_SMTP_FROM"); v != "" {
		cfg.SMTP.From = v
	}
	// Comma-separated list of allowed CORS origins, e.g.
	// "https://app.example.com,https://admin.example.com".
	if v := os.Getenv("MC_CORS_ORIGINS"); v != "" {
		parts := strings.Split(v, ",")
		origins := make([]string, 0, len(parts))
		for _, p := range parts {
			if t := strings.TrimSpace(p); t != "" {
				origins = append(origins, t)
			}
		}
		if len(origins) > 0 {
			cfg.CORSOrigins = origins
		}
	}
	if v := os.Getenv("MC_RETENTION_LOG_HOURS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Retention.LogHours = n
		}
	}
	if v := os.Getenv("MC_RETENTION_METRIC_HOURS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			cfg.Retention.MetricHours = n
		}
	}
	return cfg, nil
}
