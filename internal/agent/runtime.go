package agent

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"net/url"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/host"
	"github.com/shirou/gopsutil/v3/load"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/avi-pathak/mission-control.ai/internal/config"
	"github.com/avi-pathak/mission-control.ai/internal/protocol"
	"github.com/avi-pathak/mission-control.ai/internal/provider"
	"github.com/avi-pathak/mission-control.ai/internal/tmux"
	"go.uber.org/zap"
)

// Version is the agent build version.
const Version = "0.1.2"

// Runtime is the agent daemon.
type Runtime struct {
	cfg       config.Agent
	log       *zap.Logger
	ws        *wsClient
	providers []provider.AgentProvider
	term      *termManager

	mu       sync.RWMutex
	sessions map[string]protocol.Session // id -> last known
	logCtx   map[string]context.CancelFunc
}

// New constructs the agent runtime.
func New(cfg config.Agent, log *zap.Logger, providers []provider.AgentProvider) *Runtime {
	if cfg.AgentID == "" {
		cfg.AgentID = DeriveAgentIDForServer(cfg.ServerURL)
	}
	hostname := cfg.HostnameOverride
	if hostname == "" {
		hostname, _ = os.Hostname()
	}
	vmem, _ := mem.VirtualMemory()
	var totalMem uint64
	if vmem != nil {
		totalMem = vmem.Total
	}
	hello := protocol.AgentHello{
		APIKey:       cfg.APIKey,
		AgentID:      cfg.AgentID,
		Hostname:     hostname,
		OS:           runtime.GOOS,
		Arch:         runtime.GOARCH,
		CPUCores:     runtime.NumCPU(),
		TotalMem:     totalMem,
		AgentVersion: Version,
	}
	r := &Runtime{
		cfg:       cfg,
		log:       log,
		providers: providers,
		sessions:  make(map[string]protocol.Session),
		logCtx:    make(map[string]context.CancelFunc),
	}
	r.ws = newWSClient(cfg.ServerURL, cfg.APIKey, cfg.InsecureTLS, hello, log)
	r.ws.onMessage = r.handleServerMessage

	// Interactive terminal: stream PTY output + lifecycle back to the server.
	r.term = newTermManager(log)
	r.term.onOutput = func(ptyID string, data []byte) {
		_ = r.ws.send(protocol.TypeTerminalOutput, protocol.TerminalOutput{
			PTYID: ptyID, Data: base64.StdEncoding.EncodeToString(data),
		})
	}
	r.term.onOpened = func(ptyID, sessionID string, ok bool, errMsg string) {
		_ = r.ws.send(protocol.TypeTerminalOpened, protocol.TerminalOpened{
			PTYID: ptyID, SessionID: sessionID, OK: ok, Error: errMsg,
		})
	}
	r.term.onExit = func(ptyID string, code int) {
		_ = r.ws.send(protocol.TypeTerminalExit, protocol.TerminalExit{PTYID: ptyID, Code: code})
	}
	return r
}

// AgentID returns the resolved stable agent identifier.
func (r *Runtime) AgentID() string { return r.cfg.AgentID }

// Run starts all loops until ctx is cancelled.
func (r *Runtime) Run(ctx context.Context) {
	go r.ws.run(ctx)
	go r.discoverLoop(ctx)
	go r.heartbeatLoop(ctx)
	go r.metricsLoop(ctx)
	<-ctx.Done()
	r.term.CloseAll()
	r.log.Info("agent stopping")
}

func (r *Runtime) discoverLoop(ctx context.Context) {
	tick := time.NewTicker(time.Duration(r.cfg.DiscoverEvery) * time.Second)
	defer tick.Stop()
	r.discover(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			r.discover(ctx)
		}
	}
}

func (r *Runtime) discover(ctx context.Context) {
	seen := make(map[string]bool)
	for _, p := range r.providers {
		sessions, err := p.Discover(ctx)
		if err != nil {
			r.log.Debug("discover error", zap.String("provider", p.Name()), zap.Error(err))
			continue
		}
		for _, s := range sessions {
			s.MachineID = r.cfg.AgentID
			seen[s.ID] = true
			r.upsertSession(ctx, p, s)
		}
	}
	// Emit removals for sessions that vanished.
	r.mu.Lock()
	var removed []protocol.Session
	for id, sess := range r.sessions {
		if !seen[id] {
			removed = append(removed, sess)
		}
	}
	for _, sess := range removed {
		delete(r.sessions, sess.ID)
		if cancel, ok := r.logCtx[sess.ID]; ok {
			cancel()
			delete(r.logCtx, sess.ID)
		}
	}
	r.mu.Unlock()
	for _, sess := range removed {
		r.emitEvent(protocol.EventSessionEnded, sess.ID,
			"Session ended · "+sessionLabel(sess),
			protocol.SeverityInfo,
			map[string]string{"repo": sess.Repo, "branch": sess.Branch})
		_ = r.ws.send(protocol.TypeSessionRemoved, protocol.SessionRemoved{SessionID: sess.ID})
	}
}

func (r *Runtime) upsertSession(ctx context.Context, p provider.AgentProvider, s protocol.Session) {
	r.mu.Lock()
	prev, existed := r.sessions[s.ID]
	r.sessions[s.ID] = s
	r.mu.Unlock()

	// Report activity transitions detected during this poll.
	r.detectSessionEvents(prev, s, existed)

	_ = r.ws.send(protocol.TypeSessionUpsert, protocol.SessionUpsert{Session: s})

	if !existed {
		r.startLogStream(ctx, p, s)
	}
}

func (r *Runtime) startLogStream(ctx context.Context, p provider.AgentProvider, s protocol.Session) {
	ch, err := p.Logs(ctx, s)
	if err != nil || ch == nil {
		return
	}
	logCtx, cancel := context.WithCancel(ctx)
	r.mu.Lock()
	r.logCtx[s.ID] = cancel
	r.mu.Unlock()
	var seq int64
	go func() {
		for {
			select {
			case <-logCtx.Done():
				return
			case line, ok := <-ch:
				if !ok {
					return
				}
				seq++
				_ = r.ws.send(protocol.TypeLogAppend, protocol.LogAppend{
					SessionID: line.SessionID,
					Seq:       seq,
					Stream:    line.Stream,
					Line:      line.Line,
					TS:        line.TS,
				})
			}
		}
	}()
}

func (r *Runtime) metricsLoop(ctx context.Context) {
	tick := time.NewTicker(time.Duration(r.cfg.MetricsEvery) * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			r.mu.RLock()
			snapshot := make([]protocol.Session, 0, len(r.sessions))
			for _, s := range r.sessions {
				snapshot = append(snapshot, s)
			}
			r.mu.RUnlock()
			for _, s := range snapshot {
				for _, p := range r.providers {
					if p.Name() != s.Provider {
						continue
					}
					if sample, err := p.Metrics(ctx, s); err == nil {
						_ = r.ws.send(protocol.TypeMetricSample, sample)
					}
				}
			}
		}
	}
}

func (r *Runtime) heartbeatLoop(ctx context.Context) {
	tick := time.NewTicker(time.Duration(r.cfg.HeartbeatEvery) * time.Second)
	defer tick.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			hb := protocol.AgentHeartbeat{AgentID: r.cfg.AgentID}
			if pcts, err := cpu.PercentWithContext(ctx, 0, false); err == nil && len(pcts) > 0 {
				hb.CPUPct = pcts[0]
			}
			if vmem, err := mem.VirtualMemory(); err == nil {
				hb.MemUsedBytes = vmem.Used
			}
			if avg, err := load.Avg(); err == nil {
				hb.Load = avg.Load1
			}
			_ = r.ws.send(protocol.TypeAgentHeartbeat, hb)
		}
	}
}

// handleServerMessage processes server->agent messages.
func (r *Runtime) handleServerMessage(env *protocol.Envelope) {
	switch env.Type {
	case protocol.TypeCommand:
		r.handleCommand(env)
	case protocol.TypeTerminalOpen:
		r.handleTerminalOpen(env)
	case protocol.TypeTerminalAttach:
		r.handleTerminalAttach(env)
	case protocol.TypeTerminalInput:
		if p, err := protocol.As[protocol.TerminalInput](env); err == nil {
			r.term.Input(p.PTYID, p.Data)
		}
	case protocol.TypeTerminalResize:
		if p, err := protocol.As[protocol.TerminalResize](env); err == nil {
			r.log.Info("terminal resize", zap.String("pty", p.PTYID), zap.Int("cols", p.Cols), zap.Int("rows", p.Rows))
			r.term.Resize(p.PTYID, p.Cols, p.Rows)
		}
	case protocol.TypeTerminalClose:
		if p, err := protocol.As[protocol.TerminalClose](env); err == nil {
			r.term.Close(p.PTYID)
		}
	}
}

// handleCommand processes stop/restart control commands.
func (r *Runtime) handleCommand(env *protocol.Envelope) {
	cmd, err := protocol.As[protocol.Command](env)
	if err != nil {
		return
	}
	_ = r.ws.send(protocol.TypeCommandAck, protocol.CommandAck{RequestID: cmd.RequestID})

	r.mu.RLock()
	s, ok := r.sessions[cmd.SessionID]
	r.mu.RUnlock()
	res := protocol.CommandResult{RequestID: cmd.RequestID, SessionID: cmd.SessionID}
	if !ok {
		res.Error = "session not found"
		_ = r.ws.send(protocol.TypeCommandResult, res)
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	for _, p := range r.providers {
		if p.Name() != s.Provider {
			continue
		}
		if err := p.Control(ctx, s, cmd.Action); err != nil {
			res.Error = err.Error()
		} else {
			res.OK = true
		}
	}
	_ = r.ws.send(protocol.TypeCommandResult, res)
}

// handleTerminalOpen launches an interactive provider session under a PTY and
// registers it as a session so it appears in the dashboard.
func (r *Runtime) handleTerminalOpen(env *protocol.Envelope) {
	p, err := protocol.As[protocol.TerminalOpen](env)
	if err != nil {
		return
	}
	// Find a launchable provider.
	var spec provider.LaunchSpec
	var found bool
	for _, prov := range r.providers {
		if prov.Name() != p.Provider {
			continue
		}
		lp, ok := prov.(provider.LaunchableProvider)
		if !ok {
			r.term.onOpened(p.PTYID, "", false, "provider does not support interactive launch")
			return
		}
		spec, err = lp.Launch(context.Background(), p.CWD)
		if err != nil {
			r.term.onOpened(p.PTYID, "", false, err.Error())
			return
		}
		found = true
		break
	}
	if !found {
		r.term.onOpened(p.PTYID, "", false, "unknown provider "+p.Provider)
		return
	}

	cols, rows := p.Cols, p.Rows
	if cols == 0 {
		cols = 80
	}
	if rows == 0 {
		rows = 24
	}
	// Use the ptyId as the interactive session id so the dashboard can route.
	sessionID := "pty-" + p.PTYID
	r.term.Open(p.PTYID, sessionID, spec.Argv, spec.CWD, spec.Env, cols, rows)

	// Optionally type an initial prompt once the shell is ready.
	if p.InitialText != "" {
		go func() {
			time.Sleep(800 * time.Millisecond)
			r.term.Input(p.PTYID, base64.StdEncoding.EncodeToString([]byte(p.InitialText)))
		}()
	}
}

// handleTerminalAttach mirrors an existing tmux-backed session into the
// terminal channel so the dashboard can drive it (e.g. approve prompts).
func (r *Runtime) handleTerminalAttach(env *protocol.Envelope) {
	p, err := protocol.As[protocol.TerminalAttach](env)
	if err != nil {
		return
	}
	r.mu.RLock()
	sess, ok := r.sessions[p.SessionID]
	r.mu.RUnlock()
	if !ok || sess.TmuxSession == "" {
		r.term.onOpened(p.PTYID, p.SessionID, false,
			"session is not interactive (only tmux-backed sessions can be driven)")
		return
	}
	r.log.Info("terminal attach", zap.String("pty", p.PTYID), zap.String("tmux", sess.TmuxSession), zap.Int("cols", p.Cols), zap.Int("rows", p.Rows))
	r.term.AttachTmux(p.PTYID, p.SessionID, sess.TmuxSession, p.Cols, p.Rows,
		tmux.CaptureLoop, tmux.SendKeys, tmux.ResizeWindow, tmux.SessionExists)
}

// DeriveAgentID returns a stable machine fingerprint based on the host id,
// falling back to a random uuid if unavailable. Used both at runtime and at
// enroll time so the server can recognize the same physical machine.
func DeriveAgentID() string {
	return DeriveAgentIDForServer("")
}

// DeriveAgentIDForServer returns a stable machine fingerprint scoped to a
// server: base host id plus a short hash of the server's host. This lets ONE
// physical machine enroll into multiple servers (e.g. dev + prod) with a
// distinct identity per server, so each server's one-machine-one-workspace rule
// holds independently. An empty serverURL yields the bare host id.
func DeriveAgentIDForServer(serverURL string) string {
	base := "mc-" + uuid.NewString()
	if info, err := host.Info(); err == nil && info.HostID != "" {
		base = "mc-" + info.HostID
	}
	h := serverHost(serverURL)
	if h == "" {
		return base
	}
	sum := sha256.Sum256([]byte(h))
	return base + "-" + hex.EncodeToString(sum[:])[:8]
}

// serverHost extracts a normalized host[:port] from a ws(s)/http(s) URL. Returns
// "" if it can't be parsed or is empty.
func serverHost(serverURL string) string {
	s := strings.TrimSpace(serverURL)
	if s == "" {
		return ""
	}
	if u, err := url.Parse(s); err == nil && u.Host != "" {
		return strings.ToLower(u.Host)
	}
	// Fall back: strip scheme manually.
	if i := strings.Index(s, "://"); i >= 0 {
		s = s[i+3:]
	}
	if i := strings.IndexAny(s, "/?"); i >= 0 {
		s = s[:i]
	}
	return strings.ToLower(s)
}
