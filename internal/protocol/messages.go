package protocol

// SessionStatus is the lifecycle state of an agent session.
type SessionStatus string

const (
	StatusRunning         SessionStatus = "running"
	StatusWaitingApproval SessionStatus = "waiting_approval"
	StatusIdle            SessionStatus = "idle"
	StatusFinished        SessionStatus = "finished"
	StatusError           SessionStatus = "error"
)

// GitInfo captures the repository state for a session.
type GitInfo struct {
	Repo          string   `json:"repo"`          // repo name (basename of toplevel)
	RemoteURL     string   `json:"remoteUrl"`     // origin url if any
	Branch        string   `json:"branch"`        // current branch
	Dirty         bool     `json:"dirty"`         // uncommitted changes present
	ModifiedFiles []string `json:"modifiedFiles"` // paths (porcelain)
	Ahead         int      `json:"ahead"`
	Behind        int      `json:"behind"`
	RecentCommits []Commit `json:"recentCommits,omitempty"`
}

// Commit is a compact git log entry.
type Commit struct {
	Hash    string `json:"hash"`
	Author  string `json:"author"`
	Message string `json:"message"`
	TS      int64  `json:"ts"`
}

// Session is the canonical representation of one AI coding session.
type Session struct {
	ID             string        `json:"id"`
	MachineID      string        `json:"machineId"`
	Provider       string        `json:"provider"` // "claude-code", ...
	Status         SessionStatus `json:"status"`
	Repo           string        `json:"repo"`
	Branch         string        `json:"branch"`
	CWD            string        `json:"cwd"`
	PID            int           `json:"pid"`
	CurrentCommand string        `json:"currentCommand"`
	Version        string        `json:"version"`
	TmuxSession    string        `json:"tmuxSession,omitempty"`
	CPUPct         float64       `json:"cpuPct"`
	MemBytes       uint64        `json:"memBytes"`
	StartedAt      int64         `json:"startedAt"`      // unix millis
	LastActivityAt int64         `json:"lastActivityAt"` // unix millis
	Git            *GitInfo      `json:"git,omitempty"`
	Tokens         *TokenUsage   `json:"tokens,omitempty"`
}

// TokenUsage is the cumulative Claude token consumption for a session.
type TokenUsage struct {
	Input         int64 `json:"input"`
	Output        int64 `json:"output"`
	CacheRead     int64 `json:"cacheRead"`
	CacheCreation int64 `json:"cacheCreation"`
	Total         int64 `json:"total"`
}

// Machine describes a host running an agent.
type Machine struct {
	ID           string  `json:"id"`
	Hostname     string  `json:"hostname"`
	OS           string  `json:"os"`
	Arch         string  `json:"arch"`
	CPUCores     int     `json:"cpuCores"`
	TotalMem     uint64  `json:"totalMem"`
	AgentVersion string  `json:"agentVersion"`
	Online       bool    `json:"online"`
	CPUPct       float64 `json:"cpuPct"`
	MemUsedBytes uint64  `json:"memUsedBytes"`
	Load         float64 `json:"load"`
	LastSeenAt   int64   `json:"lastSeenAt"`
}

// --- Payloads ---

// AgentHello is sent by an agent immediately after connecting.
type AgentHello struct {
	APIKey       string `json:"apiKey"`
	AgentID      string `json:"agentId"`
	Hostname     string `json:"hostname"`
	OS           string `json:"os"`
	Arch         string `json:"arch"`
	CPUCores     int    `json:"cpuCores"`
	TotalMem     uint64 `json:"totalMem"`
	AgentVersion string `json:"agentVersion"`
}

// DashboardHello is sent by a dashboard client after connecting.
type DashboardHello struct {
	APIKey string `json:"apiKey"`
}

// AgentHeartbeat carries live host metrics.
type AgentHeartbeat struct {
	AgentID      string  `json:"agentId"`
	CPUPct       float64 `json:"cpuPct"`
	MemUsedBytes uint64  `json:"memUsedBytes"`
	Load         float64 `json:"load"`
}

// SessionUpsert announces a new or updated session.
type SessionUpsert struct {
	Session Session `json:"session"`
}

// SessionRemoved announces a session that no longer exists.
type SessionRemoved struct {
	SessionID string `json:"sessionId"`
}

// LogStream identifies the source of a log line.
type LogStream string

const (
	StreamStdout LogStream = "stdout"
	StreamStderr LogStream = "stderr"
	StreamSystem LogStream = "system"
)

// LogAppend is a single log line for a session.
type LogAppend struct {
	SessionID string    `json:"sessionId"`
	Seq       int64     `json:"seq"`
	Stream    LogStream `json:"stream"`
	Line      string    `json:"line"` // may contain ANSI escapes
	TS        int64     `json:"ts"`
}

// MetricSample is a per-session resource sample.
type MetricSample struct {
	SessionID string  `json:"sessionId"`
	CPUPct    float64 `json:"cpuPct"`
	MemBytes  uint64  `json:"memBytes"`
	TS        int64   `json:"ts"`
}

// CommandAction is a control action targeting a session.
type CommandAction string

const (
	ActionStop    CommandAction = "stop"
	ActionRestart CommandAction = "restart"
)

// Command is sent from server to agent to control a session.
type Command struct {
	RequestID string        `json:"requestId"`
	SessionID string        `json:"sessionId"`
	Action    CommandAction `json:"action"`
}

// CommandAck acknowledges receipt of a command.
type CommandAck struct {
	RequestID string `json:"requestId"`
}

// CommandResult reports the outcome of a command.
type CommandResult struct {
	RequestID string `json:"requestId"`
	SessionID string `json:"sessionId"`
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
}

// MachineUpsert announces machine state to dashboards.
type MachineUpsert struct {
	Machine Machine `json:"machine"`
}

// MachineRemoved announces a machine was deleted.
type MachineRemoved struct {
	MachineID string `json:"machineId"`
}

// AgentStatus notifies dashboards of agent connectivity changes.
type AgentStatus struct {
	MachineID string `json:"machineId"`
	Online    bool   `json:"online"`
}

// Snapshot is the full current state sent to a dashboard on connect.
type Snapshot struct {
	Machines []Machine `json:"machines"`
	Sessions []Session `json:"sessions"`
	Events   []Event   `json:"events"`
}

// EventSeverity classifies an activity event for display.
type EventSeverity string

const (
	SeverityInfo    EventSeverity = "info"
	SeveritySuccess EventSeverity = "success"
	SeverityWarn    EventSeverity = "warn"
	SeverityError   EventSeverity = "error"
)

// Event kinds reported by agents (and the server) as activities happen.
const (
	EventSessionStarted = "session.started"
	EventSessionEnded   = "session.ended"
	EventStatusChanged  = "status.changed"
	EventBranchChanged  = "branch.changed"
	EventCommitCreated  = "commit.created"
	EventCommandChanged = "command.changed"
	EventCommandIssued  = "command.issued"
	EventFilePublished  = "file.published"
	EventAgentConnected = "agent.connected"
	EventAgentOffline   = "agent.disconnected"
)

// Event is a single activity in the timeline.
type Event struct {
	ID        string            `json:"id"`
	MachineID string            `json:"machineId"`
	SessionID string            `json:"sessionId,omitempty"`
	Kind      string            `json:"kind"`
	Message   string            `json:"message"`
	Severity  EventSeverity     `json:"severity"`
	TS        int64             `json:"ts"`
	Meta      map[string]string `json:"meta,omitempty"`
}

// EventReport is sent by an agent when it detects an activity.
type EventReport struct {
	Event Event `json:"event"`
}

// EventAppend announces a new event to dashboards.
type EventAppend struct {
	Event Event `json:"event"`
}

// ErrorPayload carries a protocol-level error.
type ErrorPayload struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// --- Enrollment REST DTOs (not WebSocket envelopes) ---

// EnrollRequest is posted by an agent to exchange an enrollment token for a
// durable agent key.
type EnrollRequest struct {
	Token string `json:"token"`
	// MachineID is the agent's stable machine fingerprint (host id). Lets the
	// server enforce one-machine-per-workspace and recognize re-installs.
	MachineID string `json:"machineId"`
	Hostname  string `json:"hostname"`
	OS        string `json:"os"`
	Arch      string `json:"arch"`
}

// EnrollResponse is returned on successful enrollment.
type EnrollResponse struct {
	AgentID   string `json:"agentId"`
	AgentKey  string `json:"agentKey"`
	ServerURL string `json:"serverUrl"`
}
