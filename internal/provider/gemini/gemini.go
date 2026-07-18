package gemini

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

// Provider discovers Google Gemini CLI sessions running on the host.
type Provider struct {
	machineID string
	version   string
	verOnce   sync.Once
}

// New builds a Gemini provider.
func New(machineID string) *Provider { return &Provider{machineID: machineID} }

// Register wires the Gemini provider into the global registry.
func Register() {
	provider.Register("gemini", func() (provider.AgentProvider, error) {
		return New(""), nil
	})
}

// SetMachineID implements provider.MachineAware.
func (p *Provider) SetMachineID(id string) { p.machineID = id }

// Name implements AgentProvider.
func (p *Provider) Name() string { return "gemini" }

// isGeminiProcess heuristically identifies a Gemini CLI process. Excludes our
// own agent and the Electron Gemini desktop app helpers.
func isGeminiProcess(name, cmdline string) bool {
	l := strings.ToLower(cmdline)
	if strings.Contains(l, "mission-control-agent") {
		return false
	}
	// Exclude Electron/desktop bundles and helper processes.
	if strings.Contains(l, ".app/contents") ||
		strings.Contains(l, "crashpad") ||
		strings.Contains(l, "renderer") ||
		strings.Contains(l, "(service)") {
		return false
	}
	if name == "gemini" {
		return true
	}
	// node-based CLI: "node .../gemini" or "@google/gemini-cli".
	return strings.Contains(l, "gemini-cli") ||
		strings.Contains(l, "/gemini ") ||
		strings.HasSuffix(l, "/gemini") ||
		strings.Contains(l, "google/gemini")
}

// Launch implements provider.LaunchableProvider.
func (p *Provider) Launch(ctx context.Context, cwd string) (provider.LaunchSpec, error) {
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	if fi, err := os.Stat(cwd); err != nil || !fi.IsDir() {
		return provider.LaunchSpec{}, fmt.Errorf("invalid working directory %q", cwd)
	}
	return provider.LaunchSpec{Argv: []string{"gemini"}, CWD: cwd}, nil
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
		if !isGeminiProcess(name, cmdline) {
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
			Version:        p.geminiVersion(ctx),
			StartedAt:      createMs,
			LastActivityAt: time.Now().UnixMilli(),
		}
		if git := gitinfo.Collect(ctx, cwd); git != nil {
			sess.Git = git
			sess.Repo = git.Repo
			sess.Branch = git.Branch
		}
		sess.TmuxSession = tmux.SessionForPID(ctx, int(pr.Pid))

		// Enrich from the newest session file for this cwd.
		sessFile, hasSession := newestSessionForCWD(cwd)
		if hasSession {
			sess.Tokens = tokenUsageFromSession(sessFile)
		}
		sess.Status = p.detectStatus(ctx, sess, sessFile)
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
	return protocol.MetricSample{SessionID: s.ID, CPUPct: cpu, MemBytes: rss, TS: time.Now().UnixMilli()}, nil
}

// Logs implements AgentProvider.
func (p *Provider) Logs(ctx context.Context, s protocol.Session) (<-chan provider.LogLine, error) {
	if path, ok := newestSessionForCWD(s.CWD); ok {
		return tailSession(ctx, s, path), nil
	}
	if s.TmuxSession != "" {
		return tailTmuxAsLogs(ctx, s), nil
	}
	return nil, nil
}

func tailTmuxAsLogs(ctx context.Context, s protocol.Session) <-chan provider.LogLine {
	out := make(chan provider.LogLine, 64)
	go func() {
		defer close(out)
		for line := range tmux.TailPane(ctx, s.TmuxSession) {
			select {
			case out <- provider.LogLine{SessionID: s.ID, Stream: protocol.StreamStdout, Line: line, TS: time.Now().UnixMilli()}:
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
		return pr.TerminateWithContext(ctx)
	case protocol.ActionRestart:
		return pr.SendSignalWithContext(ctx, syscall.SIGINT)
	default:
		return fmt.Errorf("unsupported action %q", action)
	}
}

func (p *Provider) geminiVersion(ctx context.Context) string {
	p.verOnce.Do(func() {
		out, err := exec.CommandContext(ctx, "gemini", "--version").Output()
		if err == nil {
			p.version = strings.TrimSpace(string(out)) + " (Gemini)"
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
