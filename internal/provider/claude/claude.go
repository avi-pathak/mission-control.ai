// Package claude implements the AgentProvider for Claude Code sessions.
package claude

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/v3/process"
	"github.com/avi-pathak/mission-control.ai/internal/gitinfo"
	"github.com/avi-pathak/mission-control.ai/internal/protocol"
	"github.com/avi-pathak/mission-control.ai/internal/provider"
	"github.com/avi-pathak/mission-control.ai/internal/tmux"
)

// Provider discovers Claude Code CLI sessions running on the host.
type Provider struct {
	machineID string
	version   string
	verOnce   sync.Once
}

// New builds a Claude provider. machineID is injected by the agent runtime.
func New(machineID string) *Provider { return &Provider{machineID: machineID} }

// Register wires the Claude provider into the global registry. machineID is
// resolved lazily by the agent, so we register a factory that is completed at
// build time via SetMachineID.
func Register() {
	provider.Register("claude-code", func() (provider.AgentProvider, error) {
		return New(""), nil
	})
}

// SetMachineID injects the host machine id after discovery.
func (p *Provider) SetMachineID(id string) { p.machineID = id }

// Name implements AgentProvider.
func (p *Provider) Name() string { return "claude-code" }

// Launch implements provider.LaunchableProvider — starts the Claude CLI
// interactively in cwd. The PTY manager runs this under a pseudo-terminal.
func (p *Provider) Launch(ctx context.Context, cwd string) (provider.LaunchSpec, error) {
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	if fi, err := os.Stat(cwd); err != nil || !fi.IsDir() {
		return provider.LaunchSpec{}, fmt.Errorf("invalid working directory %q", cwd)
	}
	return provider.LaunchSpec{
		Argv: []string{"claude"},
		CWD:  cwd,
	}, nil
}

// isClaudeProcess heuristically identifies a Claude Code *CLI* process. It must
// exclude (a) our own agent, (b) the Claude **desktop app** and its helper
// processes (renderers, crashpad, the "disclaimer"/updater helpers, and the
// bundled claude.app helper), and (c) tool subprocesses the CLI spawns (grep,
// ugrep, etc.) which may mention a claude path in their args. Without these
// exclusions one logical session shows up as several — e.g. a blocked desktop
// session appears twice (disclaimer helper + app helper).
func isClaudeProcess(name string, cmdline string) bool {
	l := strings.ToLower(cmdline)
	if strings.Contains(l, "mission-control-agent") {
		return false
	}
	// Exclude the macOS Claude desktop app and its Electron/helper processes.
	// Note: the CLI installed *under* the app bundle lives at
	// ".../claude-code/<ver>/claude" — allow that (checked below) but reject the
	// app's own helper executables and support processes.
	if strings.Contains(l, ".app/contents/helpers") ||
		strings.Contains(l, ".app/contents/frameworks") ||
		strings.Contains(l, ".app/contents/macos/claude helper") ||
		strings.Contains(l, "claude helper") ||
		strings.Contains(l, "crashpad") ||
		strings.Contains(l, "--type=renderer") ||
		strings.Contains(l, "--type=utility") ||
		strings.Contains(l, "--type=gpu") ||
		strings.Contains(l, "/helpers/disclaimer") ||
		strings.Contains(l, "sparkle") {
		return false
	}
	// Exclude tool subprocesses spawned by the CLI (they can carry claude paths
	// in --exclude-dir / cwd args but are not sessions themselves).
	if strings.HasPrefix(l, "ugrep") || strings.HasPrefix(l, "grep") ||
		strings.HasPrefix(l, "rg ") || strings.HasPrefix(l, "fd ") {
		return false
	}
	// The process's own executable name is claude (standalone CLI). gopsutil may
	// report it as "claude" or "claude.exe" depending on how it reads the name,
	// so match the base name rather than requiring an exact string.
	n := strings.ToLower(name)
	if n == "claude" || n == "claude.exe" {
		return true
	}
	// Bare "claude" command line (e.g. launched via tmux/shell as just `claude`).
	if l == "claude" {
		return true
	}
	// node-based / bundled CLI: a path ending in "/claude", "claude-code", or the
	// anthropic package — but NOT merely mentioning claude somewhere in args.
	return strings.HasSuffix(l, "/claude") ||
		strings.Contains(l, "/claude ") ||
		strings.Contains(l, "anthropic-ai/claude-code") ||
		strings.Contains(l, "claude-code/") && strings.Contains(l, "/claude")
}

// Discover implements AgentProvider.
func (p *Provider) Discover(ctx context.Context) ([]protocol.Session, error) {
	procs, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil, err
	}
	var sessions []protocol.Session
	for _, pr := range procs {
		name, _ := pr.NameWithContext(ctx)
		cmdline, _ := pr.CmdlineWithContext(ctx)
		if !isClaudeProcess(name, cmdline) {
			continue
		}
		cwd, _ := pr.CwdWithContext(ctx)
		createMs, _ := pr.CreateTimeWithContext(ctx)

		sess := protocol.Session{
			ID:             fmt.Sprintf("%s-%d", p.machineID, pr.Pid),
			MachineID:      p.machineID,
			Provider:       p.Name(),
			Status:         protocol.StatusRunning,
			CWD:            cwd,
			PID:            int(pr.Pid),
			CurrentCommand: truncate(cmdline, 512),
			Version:        p.claudeVersion(ctx),
			StartedAt:      createMs,
			LastActivityAt: time.Now().UnixMilli(),
		}
		if git := gitinfo.Collect(ctx, cwd); git != nil {
			sess.Git = git
			sess.Repo = git.Repo
			sess.Branch = git.Branch
		}
		sess.TmuxSession = tmux.SessionForPID(ctx, int(pr.Pid))
		sess.Tokens = tokenUsageForCWD(cwd)
		sess.Status = p.detectStatus(ctx, sess)
		sessions = append(sessions, sess)
	}
	return sessions, nil
}

// Metrics implements AgentProvider.
func (p *Provider) Metrics(ctx context.Context, s protocol.Session) (protocol.MetricSample, error) {
	pr, err := process.NewProcessWithContext(ctx, int32(s.PID))
	if err != nil {
		return protocol.MetricSample{}, err
	}
	cpu, _ := pr.CPUPercentWithContext(ctx)
	mem, _ := pr.MemoryInfoWithContext(ctx)
	var rss uint64
	if mem != nil {
		rss = mem.RSS
	}
	return protocol.MetricSample{
		SessionID: s.ID,
		CPUPct:    cpu,
		MemBytes:  rss,
		TS:        time.Now().UnixMilli(),
	}, nil
}

// Logs implements AgentProvider. Claude Code does not expose a log socket, so
// we tail its session transcript (~/.claude/projects/<cwd>/<uuid>.jsonl) when
// available, falling back to the tmux pane for sessions running under tmux.
func (p *Provider) Logs(ctx context.Context, s protocol.Session) (<-chan provider.LogLine, error) {
	if path, ok := transcriptFile(s.CWD); ok {
		return tailTranscript(ctx, s, path), nil
	}
	if s.TmuxSession != "" {
		return tailTmuxPaneAsLogs(ctx, s), nil
	}
	return nil, nil
}

// tailTmuxPaneAsLogs adapts the shared tmux pane tail (plain strings) into the
// provider.LogLine channel the runtime expects.
func tailTmuxPaneAsLogs(ctx context.Context, s protocol.Session) <-chan provider.LogLine {
	out := make(chan provider.LogLine, 64)
	go func() {
		defer close(out)
		for line := range tmux.TailPane(ctx, s.TmuxSession) {
			select {
			case out <- provider.LogLine{
				SessionID: s.ID,
				Stream:    protocol.StreamStdout,
				Line:      line,
				TS:        time.Now().UnixMilli(),
			}:
			case <-ctx.Done():
				return
			}
		}
	}()
	return out
}

// Control implements AgentProvider.
func (p *Provider) Control(ctx context.Context, s protocol.Session, action protocol.CommandAction) error {
	pr, err := process.NewProcessWithContext(ctx, int32(s.PID))
	if err != nil {
		return err
	}
	switch action {
	case protocol.ActionStop:
		return pr.TerminateWithContext(ctx) // SIGTERM
	case protocol.ActionRestart:
		// Graceful stop; supervisors/tmux are expected to relaunch.
		_ = pr.SendSignalWithContext(ctx, syscall.SIGINT)
		return nil
	default:
		return fmt.Errorf("unsupported action %q", action)
	}
}

func (p *Provider) claudeVersion(ctx context.Context) string {
	p.verOnce.Do(func() {
		out, err := exec.CommandContext(ctx, "claude", "--version").Output()
		if err == nil {
			p.version = strings.TrimSpace(string(out))
		}
	})
	return p.version
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
