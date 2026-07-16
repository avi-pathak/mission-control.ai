// Package provider defines the AgentProvider extension interface and a
// registry. ClaudeProvider is the first implementation; additional providers
// (Codex, Gemini, Aider, OpenHands, Roo, Continue) implement the same
// interface and register themselves.
package provider

import (
	"context"
	"fmt"
	"sync"

	"github.com/avi-pathak/mission-control.ai/internal/protocol"
)

// LogLine is a discovered log line from a provider.
type LogLine struct {
	SessionID string
	Stream    protocol.LogStream
	Line      string
	TS        int64
}

// AgentProvider discovers and controls sessions for one kind of AI coding tool.
type AgentProvider interface {
	// Name is the stable provider identifier, e.g. "claude-code".
	Name() string
	// Discover returns the currently live sessions on this host.
	Discover(ctx context.Context) ([]protocol.Session, error)
	// Metrics samples resource usage for a session.
	Metrics(ctx context.Context, s protocol.Session) (protocol.MetricSample, error)
	// Logs streams log lines for a session until ctx is cancelled. Providers
	// that cannot stream logs may return a nil channel.
	Logs(ctx context.Context, s protocol.Session) (<-chan LogLine, error)
	// Control performs a control action (stop/restart) on a session.
	Control(ctx context.Context, s protocol.Session, action protocol.CommandAction) error
}

// LaunchSpec describes how to start an interactive provider session under a PTY.
type LaunchSpec struct {
	Argv []string // command + args, e.g. ["claude"]
	CWD  string   // working directory
	Env  []string // extra environment (appended to os.Environ())
}

// LaunchableProvider is optionally implemented by providers that can start a
// new interactive session (driven from the dashboard via a PTY).
type LaunchableProvider interface {
	// Launch returns the command spec to run for an interactive session in cwd.
	Launch(ctx context.Context, cwd string) (LaunchSpec, error)
}

// MachineAware is optionally implemented by providers that need the host's
// stable machine id injected after the agent resolves it (used to build
// deterministic session ids).
type MachineAware interface {
	SetMachineID(string)
}

// Factory constructs a provider instance.
type Factory func() (AgentProvider, error)

var (
	mu        sync.RWMutex
	factories = map[string]Factory{}
)

// Register makes a provider factory available by name.
func Register(name string, f Factory) {
	mu.Lock()
	defer mu.Unlock()
	factories[name] = f
}

// Build instantiates the providers named in `names`. Unknown names error.
func Build(names []string) ([]AgentProvider, error) {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]AgentProvider, 0, len(names))
	for _, n := range names {
		f, ok := factories[n]
		if !ok {
			return nil, fmt.Errorf("unknown provider %q", n)
		}
		p, err := f()
		if err != nil {
			return nil, fmt.Errorf("build provider %q: %w", n, err)
		}
		out = append(out, p)
	}
	return out, nil
}
