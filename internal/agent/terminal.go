package agent

import (
	"context"
	"encoding/base64"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/creack/pty"
	"go.uber.org/zap"
)

// ptySession is one live pseudo-terminal running a provider process, OR a
// tmux-backed stream mirroring an existing pane (kind == "tmux").
type ptySession struct {
	id     string
	kind   string // "pty" or "tmux"
	target string // tmux target (kind == "tmux")
	cmd    *exec.Cmd
	ptmx   *os.File
	cancel context.CancelFunc
	once   sync.Once

	// tmux input handler (kind == "tmux").
	sendKeys func(ctx context.Context, data []byte)
	resize   func(ctx context.Context, cols, rows int)
	// desired size the dashboard xterm wants; re-asserted each capture tick so
	// an attached real terminal can't shrink/grow the window out from under us.
	dMu        sync.Mutex
	desiredCol int
	desiredRow int
}

func (p *ptySession) setDesired(cols, rows int) {
	p.dMu.Lock()
	p.desiredCol, p.desiredRow = cols, rows
	p.dMu.Unlock()
}

func (p *ptySession) getDesired() (int, int) {
	p.dMu.Lock()
	defer p.dMu.Unlock()
	return p.desiredCol, p.desiredRow
}

func (p *ptySession) close() {
	p.once.Do(func() {
		if p.cancel != nil {
			p.cancel()
		}
		if p.ptmx != nil {
			_ = p.ptmx.Close()
		}
		// For tmux sessions we do NOT kill the user's process — just stop
		// streaming. Only PTY-owned processes are terminated.
		if p.kind == "pty" && p.cmd != nil && p.cmd.Process != nil {
			_ = p.cmd.Process.Kill()
		}
	})
}

// termManager owns all live PTYs for the agent and streams their output back
// through the provided emit callbacks.
type termManager struct {
	log      *zap.Logger
	mu       sync.Mutex
	sessions map[string]*ptySession // ptyId -> session

	// callbacks wired by the runtime.
	onOutput func(ptyID string, data []byte)
	onOpened func(ptyID, sessionID string, ok bool, errMsg string)
	onExit   func(ptyID string, code int)
}

func newTermManager(log *zap.Logger) *termManager {
	return &termManager{log: log, sessions: make(map[string]*ptySession)}
}

// Open launches argv in cwd under a new PTY sized cols x rows.
func (m *termManager) Open(ptyID, sessionID string, argv []string, cwd string, extraEnv []string, cols, rows int) {
	if len(argv) == 0 {
		m.onOpened(ptyID, sessionID, false, "empty command")
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	cmd.Dir = cwd
	cmd.Env = append(os.Environ(), extraEnv...)

	ws := &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)}
	ptmx, err := pty.StartWithSize(cmd, ws)
	if err != nil {
		cancel()
		m.onOpened(ptyID, sessionID, false, err.Error())
		return
	}

	sess := &ptySession{id: ptyID, kind: "pty", cmd: cmd, ptmx: ptmx, cancel: cancel}
	m.mu.Lock()
	m.sessions[ptyID] = sess
	m.mu.Unlock()

	m.onOpened(ptyID, sessionID, true, "")

	// Stream output.
	go func() {
		buf := make([]byte, 32*1024)
		for {
			n, err := ptmx.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				m.onOutput(ptyID, chunk)
			}
			if err != nil {
				break
			}
		}
		// Process ended.
		code := 0
		if werr := cmd.Wait(); werr != nil {
			if ee, ok := werr.(*exec.ExitError); ok {
				code = ee.ExitCode()
			} else {
				code = 1
			}
		}
		m.remove(ptyID)
		m.onExit(ptyID, code)
	}()
}

// AttachTmux mirrors an existing tmux pane into the terminal channel. Output is
// polled from `capture-pane`; input is applied via `send-keys`. Closing the
// stream never kills the user's tmux session.
func (m *termManager) AttachTmux(ptyID, sessionID, target string, cols, rows int, capture func(ctx context.Context, target string, reassert func(), fn func([]byte)), sendKeys func(ctx context.Context, target string, data []byte) error, resizeWin func(ctx context.Context, target string, cols, rows int), exists func(ctx context.Context, target string) bool) {
	if exists != nil && !exists(context.Background(), target) {
		m.onOpened(ptyID, sessionID, false, "tmux session not found")
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	sess := &ptySession{
		id: ptyID, kind: "tmux", target: target, cancel: cancel,
		sendKeys: func(c context.Context, data []byte) { _ = sendKeys(c, target, data) },
		resize:   func(c context.Context, cw, rw int) { resizeWin(c, target, cw, rw) },
	}
	sess.setDesired(cols, rows)
	m.mu.Lock()
	m.sessions[ptyID] = sess
	m.mu.Unlock()

	// Size the tmux window to the dashboard xterm before streaming.
	if cols > 0 && rows > 0 {
		resizeWin(ctx, target, cols, rows)
	}
	m.onOpened(ptyID, sessionID, true, "")
	go func() {
		// reassert re-applies the desired size, throttled to ~1/sec, so an
		// attached real terminal can't win the size fight without constant churn.
		var lastReassert time.Time
		reassert := func() {
			if time.Since(lastReassert) < time.Second {
				return
			}
			lastReassert = time.Now()
			if cw, rw := sess.getDesired(); cw > 0 && rw > 0 {
				resizeWin(ctx, target, cw, rw)
			}
		}
		capture(ctx, target, reassert, func(b []byte) { m.onOutput(ptyID, b) })
	}()
}

// Input writes base64-encoded keystrokes to a PTY or tmux session.
func (m *termManager) Input(ptyID, b64 string) {
	m.mu.Lock()
	sess := m.sessions[ptyID]
	m.mu.Unlock()
	if sess == nil {
		return
	}
	data, err := base64.StdEncoding.DecodeString(b64)
	if err != nil {
		return
	}
	if sess.kind == "tmux" {
		if sess.sendKeys != nil {
			sess.sendKeys(context.Background(), data)
		}
		return
	}
	_, _ = sess.ptmx.Write(data)
}

// Resize updates a PTY window size.
func (m *termManager) Resize(ptyID string, cols, rows int) {
	m.mu.Lock()
	sess := m.sessions[ptyID]
	m.mu.Unlock()
	if sess == nil {
		return
	}
	if sess.kind == "tmux" {
		sess.setDesired(cols, rows) // remembered + re-asserted each capture tick
		if sess.resize != nil {
			sess.resize(context.Background(), cols, rows)
		}
		return
	}
	_ = pty.Setsize(sess.ptmx, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}

// Close terminates a PTY.
func (m *termManager) Close(ptyID string) {
	sess := m.remove(ptyID)
	if sess != nil {
		sess.close()
	}
}

func (m *termManager) remove(ptyID string) *ptySession {
	m.mu.Lock()
	defer m.mu.Unlock()
	sess := m.sessions[ptyID]
	delete(m.sessions, ptyID)
	return sess
}

// CloseAll terminates every PTY (agent shutdown).
func (m *termManager) CloseAll() {
	m.mu.Lock()
	all := make([]*ptySession, 0, len(m.sessions))
	for _, s := range m.sessions {
		all = append(all, s)
	}
	m.sessions = make(map[string]*ptySession)
	m.mu.Unlock()
	for _, s := range all {
		s.close()
	}
}
