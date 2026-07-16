package claude

import (
	"bufio"
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"

	"github.com/avi-pathak/mission-control.ai/internal/protocol"
	"github.com/avi-pathak/mission-control.ai/internal/provider"
)

// tmuxSessionForPID returns the tmux session name whose active pane runs the
// given PID, or "" if tmux is unavailable or no match is found.
func tmuxSessionForPID(ctx context.Context, pid int) string {
	out, err := exec.CommandContext(ctx, "tmux", "list-panes", "-a",
		"-F", "#{session_name}\x1f#{pane_pid}").Output()
	if err != nil {
		return ""
	}
	target := strconv.Itoa(pid)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		f := strings.Split(line, "\x1f")
		if len(f) == 2 && f[1] == target {
			return f[0]
		}
	}
	return ""
}

// tailTmuxPane polls `tmux capture-pane` and emits changed content as logs.
// This is a pragmatic read-only tail suitable for the MVP.
func tailTmuxPane(ctx context.Context, s protocol.Session) <-chan provider.LogLine {
	ch := make(chan provider.LogLine, 64)
	go func() {
		defer close(ch)
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()
		var lastLine string
		var seq int64
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				out, err := exec.CommandContext(ctx, "tmux", "capture-pane",
					"-t", s.TmuxSession, "-p", "-e").Output()
				if err != nil {
					continue
				}
				sc := bufio.NewScanner(strings.NewReader(string(out)))
				var lines []string
				for sc.Scan() {
					lines = append(lines, sc.Text())
				}
				// Emit only new trailing content since last capture.
				if len(lines) == 0 {
					continue
				}
				last := lines[len(lines)-1]
				if last == lastLine {
					continue
				}
				lastLine = last
				seq++
				select {
				case ch <- provider.LogLine{
					SessionID: s.ID,
					Stream:    protocol.StreamStdout,
					Line:      last,
					TS:        time.Now().UnixMilli(),
				}:
				case <-ctx.Done():
					return
				}
			}
		}
	}()
	return ch
}
