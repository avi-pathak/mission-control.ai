package protocol

// Interactive terminal payloads. A PTY session is identified by a
// dashboard-generated PTYID. Data is transferred as base64 in JSON.

// TerminalOpen asks an agent to launch a provider session under a PTY.
type TerminalOpen struct {
	PTYID       string `json:"ptyId"`
	MachineID   string `json:"machineId"`
	Provider    string `json:"provider"` // e.g. "claude-code"
	CWD         string `json:"cwd"`      // working directory / repo path
	InitialText string `json:"initialText,omitempty"`
	Cols        int    `json:"cols"`
	Rows        int    `json:"rows"`
}

// TerminalAttach asks an agent to re-attach output for an existing PTY it owns.
type TerminalAttach struct {
	PTYID     string `json:"ptyId"`
	MachineID string `json:"machineId"`
	SessionID string `json:"sessionId"`
	Cols      int    `json:"cols"`
	Rows      int    `json:"rows"`
}

// TerminalInput carries keystrokes from the dashboard to the PTY (base64).
type TerminalInput struct {
	PTYID string `json:"ptyId"`
	Data  string `json:"data"` // base64-encoded bytes
}

// TerminalResize updates the PTY window size.
type TerminalResize struct {
	PTYID string `json:"ptyId"`
	Cols  int    `json:"cols"`
	Rows  int    `json:"rows"`
}

// TerminalClose terminates a PTY session.
type TerminalClose struct {
	PTYID string `json:"ptyId"`
}

// TerminalOutput streams raw PTY bytes to the dashboard (base64).
type TerminalOutput struct {
	PTYID string `json:"ptyId"`
	Data  string `json:"data"` // base64-encoded bytes
}

// TerminalOpened reports the result of a terminal.open/attach.
type TerminalOpened struct {
	PTYID     string `json:"ptyId"`
	SessionID string `json:"sessionId"`
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
}

// TerminalExit reports that a PTY process ended.
type TerminalExit struct {
	PTYID string `json:"ptyId"`
	Code  int    `json:"code"`
}
