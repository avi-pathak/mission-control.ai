// Package store provides GORM-backed persistence (Postgres or SQLite) for
// machines, sessions, events, logs, metrics, tenancy (orgs/users/invites) and
// published files.
package store

import (
	"strings"
	"time"

	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
	glogger "gorm.io/gorm/logger"
)

// --- Tenancy ---

// Org is a tenant. All domain data is scoped to an org.
type Org struct {
	ID        string `gorm:"primaryKey"`
	Name      string
	Slug      string `gorm:"uniqueIndex"`
	CreatedAt time.Time
}

// User account statuses.
const (
	UserStatusPending = "pending"
	UserStatusActive  = "active"
)

// User is a dashboard user. Pending users have no org and no access until a
// platform admin approves and assigns them to an org.
type User struct {
	ID            string `gorm:"primaryKey"`
	OrgID         string `gorm:"index"` // empty until approved
	Email         string `gorm:"uniqueIndex"`
	PasswordHash  string
	Role          string // owner | admin | member
	Status        string // pending | active
	PlatformAdmin bool   // platform-level superadmin (approves signups)
	CreatedAt     time.Time
}

// Invite lets an owner/admin add a user to their org by email.
type Invite struct {
	Token      string `gorm:"primaryKey"`
	OrgID      string `gorm:"index"`
	Email      string `gorm:"index"`
	Role       string
	InvitedBy  string
	ExpiresAt  time.Time
	AcceptedAt *time.Time
	CreatedAt  time.Time
}

// PublishedFile is an artifact uploaded by an agent, stored as a blob.
type PublishedFile struct {
	ID          string `gorm:"primaryKey"`
	OrgID       string `gorm:"index"`
	MachineID   string `gorm:"index"`
	SessionID   string `gorm:"index"`
	Name        string
	Size        int64
	ContentType string
	SHA256      string
	Data        []byte
	CreatedAt   time.Time
}

// --- Domain (all org-scoped) ---

// Machine is the persisted host record.
type Machine struct {
	ID           string `gorm:"primaryKey"`
	OrgID        string `gorm:"index"`
	Hostname     string
	OS           string
	Arch         string
	CPUCores     int
	TotalMem     uint64
	AgentVersion string
	LastSeenAt   time.Time
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

// Session is the persisted session record (history + current).
type Session struct {
	ID             string `gorm:"primaryKey"`
	OrgID          string `gorm:"index"`
	MachineID      string `gorm:"index"`
	Provider       string
	Status         string `gorm:"index"`
	Repo           string `gorm:"index"`
	Branch         string
	CWD            string
	PID            int
	CurrentCommand string
	Version        string
	TmuxSession    string
	StartedAt      time.Time
	LastActivityAt time.Time
	EndedAt        *time.Time
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// LogLine is a persisted log entry for a session.
type LogLine struct {
	ID        uint64 `gorm:"primaryKey;autoIncrement"`
	OrgID     string `gorm:"index"`
	SessionID string `gorm:"index:idx_log_session_seq"`
	Seq       int64  `gorm:"index:idx_log_session_seq"`
	Stream    string
	Line      string
	TS        time.Time
}

// MetricSample is a persisted per-session resource sample.
type MetricSample struct {
	ID        uint64 `gorm:"primaryKey;autoIncrement"`
	OrgID     string `gorm:"index"`
	SessionID string `gorm:"index:idx_metric_session_ts"`
	CPUPct    float64
	MemBytes  uint64
	TS        time.Time `gorm:"index:idx_metric_session_ts"`
}

// Event is an audit/timeline entry (activity feed).
type Event struct {
	ID        uint64 `gorm:"primaryKey;autoIncrement"`
	OrgID     string `gorm:"index"`
	EventID   string `gorm:"uniqueIndex"` // client-supplied uuid (idempotency)
	MachineID string `gorm:"index"`
	SessionID string `gorm:"index"`
	Kind      string `gorm:"index"`
	Message   string
	Severity  string
	Meta      string    // JSON-encoded map
	TS        time.Time `gorm:"index"`
	CreatedAt time.Time
}

// EnrollmentToken is a short-lived, single-use credential a user pastes onto a
// new machine. On successful enrollment it is consumed and a durable AgentKey
// is minted. Bound to the org that created it.
type EnrollmentToken struct {
	Token     string `gorm:"primaryKey"`
	OrgID     string `gorm:"index"`
	Label     string
	CreatedAt time.Time
	ExpiresAt time.Time
	UsedAt    *time.Time
	UsedByID  string
	Revoked   bool
}

// AgentKey is the durable per-machine credential minted at enrollment. Carries
// the org so all data from that agent lands in the right tenant.
type AgentKey struct {
	Key       string `gorm:"primaryKey"`
	OrgID     string `gorm:"index"`
	MachineID string `gorm:"index"`
	Label     string
	CreatedAt time.Time
	Revoked   bool
}

// Store wraps a gorm.DB with domain helpers.
type Store struct {
	db  *gorm.DB
	log *zap.Logger
}

// Open selects a driver based on dsn and migrates the schema. A dsn beginning
// with postgres:// (or postgresql://) uses Postgres; anything else is treated
// as a SQLite file path (the fallback / dev default).
func Open(dsn string, log *zap.Logger) (*Store, error) {
	var dialector gorm.Dialector
	if isPostgresDSN(dsn) {
		dialector = postgres.Open(dsn)
	} else {
		dialector = sqlite.Open(dsn)
	}
	db, err := gorm.Open(dialector, &gorm.Config{
		Logger: glogger.Default.LogMode(glogger.Silent),
	})
	if err != nil {
		return nil, err
	}
	if err := db.AutoMigrate(
		&Org{}, &User{}, &Invite{}, &PublishedFile{},
		&Machine{}, &Session{}, &LogLine{}, &MetricSample{}, &Event{},
		&EnrollmentToken{}, &AgentKey{},
	); err != nil {
		return nil, err
	}
	return &Store{db: db, log: log}, nil
}

func isPostgresDSN(dsn string) bool {
	return strings.HasPrefix(dsn, "postgres://") || strings.HasPrefix(dsn, "postgresql://")
}

// DB exposes the underlying gorm handle (for advanced queries/tests).
func (s *Store) DB() *gorm.DB { return s.db }
