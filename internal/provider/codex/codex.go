package codex

import (
	"context"
	"fmt"
	"os"
	"strings"
	"syscall"
	"time"

	"github.com/shirou/gopsutil/v3/process"

	"github.com/avi-pathak/mission-control.ai/internal/gitinfo"
	"github.com/avi-pathak/mission-control.ai/internal/protocol"
	"github.com/avi-pathak/mission-control.ai/internal/provider"
	"github.com/avi-pathak/mission-control.ai/internal/tmux"
)

// Provider discovers OpenAI Codex CLI sessions running on the host.
type Provider struct {
	machineID string
}

// New builds a Codex provider.
func New(machineID string) *Provider { return &Provider{machineID: machineID} }

// Register wires the Codex provider into the global registry.
func Register() {
	provider.Register("codex", func() (provider.AgentProvider, error) {
		return New(""), nil
	})
}

// SetMachineID implements provider.MachineAware.
func (p *Provider) SetMachineID(id string) { p.machineID = id }

// Name implements AgentProvider.
func (p *Provider) Name() string { return "codex" }

// isCodexProcess heuristically identifies a Codex CLI process. Excludes our own
// agent binary and obvious non-matches.
func isCodexProcess(name, cmdline string) bool {
	l := strings.ToLower(cmdline)
	if strings.Contains(l, "mission-control-agent") {
		return false
	}
	if name == "codex" || name == "codex-tui" {
		return true
	}
	// binary path ending in /codex, or `codex exec`/`codex tui` invocations
	return strings.HasSuffix(l, "/codex") ||
		strings.Contains(l, "/codex ") ||
		strings.Contains(l, "codex exec") ||
		strings.Contains(l, "codex tui") ||
		strings.Contains(l, "codex-rs")
}

// Launch implements provider.LaunchableProvider.
func (p *Provider) Launch(ctx context.Context, cwd string) (provider.LaunchSpec, error) {
	if cwd == "" {
		cwd, _ = os.Getwd()
	}
	if fi, err := os.Stat(cwd); err != nil || !fi.IsDir() {
		return provider.LaunchSpec{}, fmt.Errorf("invalid working directory %q", cwd)
	}
	return provider.LaunchSpec{Argv: []string{"codex"}, CWD: cwd}, nil
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
		if !isCodexProcess(name, cmdline) {
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
			StartedAt:      createMs,
			LastActivityAt: time.Now().UnixMilli(),
		}
		if git := gitinfo.Collect(ctx, cwd); git != nil {
			sess.Git = git
			sess.Repo = git.Repo
			sess.Branch = git.Branch
		}
		sess.TmuxSession = tmux.SessionForPID(ctx, int(pr.Pid))

		// Enrich from the newest rollout for this cwd.
		rollout, hasRollout := newestRolloutForCWD(cwd)
		if hasRollout {
			if meta, ok := readSessionMeta(rollout); ok {
				sess.Version = meta.CLIVersion + " (Codex)"
			}
			sess.Tokens = tokenUsageFromRollout(rollout)
		}
		sess.Status = p.detectStatus(ctx, sess, rollout)
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
	if path, ok := newestRolloutForCWD(s.CWD); ok {
		return tailRollout(ctx, s, path), nil
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

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
