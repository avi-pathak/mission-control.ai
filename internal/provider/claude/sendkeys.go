package claude

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// SendKeysArgs translates raw terminal input bytes into a sequence of
// `tmux send-keys -t <target> ...` argument lists. Printable runs are sent
// literally (-l); recognized control sequences are sent as tmux key names so
// Enter, Ctrl-C, arrows, etc. behave correctly (this is what makes y/Enter
// approvals work). Exposed for testing.
func SendKeysArgs(target string, data []byte) [][]string {
	var out [][]string
	var lit strings.Builder

	flush := func() {
		if lit.Len() > 0 {
			out = append(out, []string{"send-keys", "-t", target, "-l", lit.String()})
			lit.Reset()
		}
	}
	special := func(key string) {
		flush()
		out = append(out, []string{"send-keys", "-t", target, key})
	}

	for i := 0; i < len(data); i++ {
		b := data[i]
		switch {
		case b == '\r' || b == '\n':
			special("Enter")
		case b == 0x03:
			special("C-c")
		case b == 0x04:
			special("C-d")
		case b == 0x09:
			special("Tab")
		case b == 0x7f || b == 0x08:
			special("BSpace")
		case b == 0x1b: // ESC — check for arrow/escape sequences
			if i+2 < len(data) && data[i+1] == '[' {
				switch data[i+2] {
				case 'A':
					special("Up")
				case 'B':
					special("Down")
				case 'C':
					special("Right")
				case 'D':
					special("Left")
				default:
					special("Escape")
					continue
				}
				i += 2
			} else {
				special("Escape")
			}
		default:
			if b >= 0x20 { // printable
				lit.WriteByte(b)
			}
		}
	}
	flush()
	return out
}

// SendKeys applies translated input to a tmux session/pane.
func SendKeys(ctx context.Context, target string, data []byte) error {
	for _, args := range SendKeysArgs(target, data) {
		if err := exec.CommandContext(ctx, "tmux", args...).Run(); err != nil {
			return err
		}
	}
	return nil
}

// CaptureLoop mirrors a tmux pane into fn as a terminal byte stream. On each
// change it repaints without a full-screen clear (which flickers): it homes the
// cursor, overwrites every line clearing to end-of-line, then clears any
// trailing rows. UTF-8 box-drawing and ANSI colors are preserved as raw bytes.
// The caller sizes the tmux window to the xterm via ResizeWindow.
func CaptureLoop(ctx context.Context, target string, reassert func(), fn func([]byte)) {
	// Poll fast so typing feels responsive (echo + cursor track the input).
	ticker := time.NewTicker(50 * time.Millisecond)
	defer ticker.Stop()

	var last []byte
	var lastCx, lastCy = -2, -2
	emit := func() {
		// Re-assert the desired window size first so an attached real terminal
		// can't resize the pane out from under the dashboard.
		if reassert != nil {
			reassert()
		}
		out, err := exec.CommandContext(ctx, "tmux",
			"capture-pane", "-t", target, "-p", "-e").Output()
		if err != nil {
			return
		}
		cx, cy := cursorPos(ctx, target)
		// Repaint when the content OR the cursor position changes (typing often
		// moves only the cursor, which must still be reflected live).
		if bytesEqual(out, last) && cx == lastCx && cy == lastCy {
			return
		}
		last = out
		lastCx, lastCy = cx, cy
		fn(renderFrame(out, cx, cy))
	}

	emit() // immediate first frame
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			emit()
		}
	}
}

// cursorPos returns the pane's cursor column/row (0-based) from tmux, or
// (-1,-1) if unavailable.
func cursorPos(ctx context.Context, target string) (int, int) {
	out, err := exec.CommandContext(ctx, "tmux", "display-message", "-p", "-t", target,
		"#{cursor_x} #{cursor_y}").Output()
	if err != nil {
		return -1, -1
	}
	parts := strings.Fields(strings.TrimSpace(string(out)))
	if len(parts) != 2 {
		return -1, -1
	}
	x, e1 := strconv.Atoi(parts[0])
	y, e2 := strconv.Atoi(parts[1])
	if e1 != nil || e2 != nil {
		return -1, -1
	}
	return x, y
}

// renderFrame turns a captured pane buffer into a flicker-free repaint: cursor
// home, then each source line followed by "clear to end of line" and an
// explicit CRLF, then "clear to end of screen" to wipe any leftover rows, and
// finally moves the cursor to where tmux actually has it (cx,cy 0-based).
func renderFrame(buf []byte, cx, cy int) []byte {
	const (
		home   = "\x1b[H"  // cursor to row 1 col 1
		clrEOL = "\x1b[K"  // clear from cursor to end of line
		clrEOS = "\x1b[J"  // clear from cursor to end of screen
		reset  = "\x1b[0m" // reset attributes
	)
	out := make([]byte, 0, len(buf)+256)
	out = append(out, home...)
	// Split on newlines; capture-pane uses \n between rows.
	lines := splitLines(buf)
	for i, ln := range lines {
		out = append(out, ln...)
		out = append(out, clrEOL...)
		if i < len(lines)-1 {
			out = append(out, '\r', '\n')
		}
	}
	out = append(out, reset...)
	out = append(out, clrEOS...)
	// Restore the real cursor position (ANSI escapes are 1-based).
	if cx >= 0 && cy >= 0 {
		out = append(out, []byte("\x1b["+strconv.Itoa(cy+1)+";"+strconv.Itoa(cx+1)+"H")...)
	}
	return out
}

// splitLines splits on '\n' without allocating substrings' escapes wrong.
func splitLines(b []byte) [][]byte {
	var lines [][]byte
	start := 0
	for i := 0; i < len(b); i++ {
		if b[i] == '\n' {
			lines = append(lines, b[start:i])
			start = i + 1
		}
	}
	lines = append(lines, b[start:])
	return lines
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// ResizeWindow resizes the tmux window so its pane matches the dashboard xterm.
// This is what stops line-wrapping artifacts: the pane renders at the browser's
// column width instead of the size of whatever real terminal is attached.
func ResizeWindow(ctx context.Context, target string, cols, rows int) {
	if cols <= 0 || rows <= 0 {
		return
	}
	// window-size manual lets us force a size independent of attached clients.
	_ = exec.CommandContext(ctx, "tmux", "set-window-option", "-t", target,
		"window-size", "manual").Run()
	_ = exec.CommandContext(ctx, "tmux", "resize-window", "-t", target,
		"-x", strconv.Itoa(cols), "-y", strconv.Itoa(rows)).Run()
}

// TmuxSessionExists reports whether a tmux session/target is live.
func TmuxSessionExists(ctx context.Context, target string) bool {
	// target may be "session" or "session:win.pane"; has-session wants the name.
	name := target
	if i := strings.IndexByte(name, ':'); i >= 0 {
		name = name[:i]
	}
	return exec.CommandContext(ctx, "tmux", "has-session", "-t", name).Run() == nil
}
