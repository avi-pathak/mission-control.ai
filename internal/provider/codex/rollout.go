// Package codex implements the AgentProvider for OpenAI Codex CLI sessions.
//
// Codex writes newline-delimited "rollout" files to
// ~/.codex/sessions/YYYY/MM/DD/rollout-<ts>-<uuid>.jsonl (honoring $CODEX_HOME).
// Line 1 is a session_meta record with the working directory + version; later
// lines carry token_count events and response items we use for logs and status.
package codex

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// codexSessionsDir returns ~/.codex/sessions, honoring $CODEX_HOME.
func codexSessionsDir() string {
	if h := os.Getenv("CODEX_HOME"); h != "" {
		return filepath.Join(h, "sessions")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".codex", "sessions")
}

// sessionMeta is the parsed first line of a rollout file.
type sessionMeta struct {
	CWD        string
	CLIVersion string
	SessionID  string
}

type rawMeta struct {
	Type    string `json:"type"`
	Payload struct {
		SessionID  string          `json:"session_id"`
		CWD        string          `json:"cwd"`
		CLIVersion string          `json:"cli_version"`
		Git        json.RawMessage `json:"git"`
	} `json:"payload"`
}

// readSessionMeta parses a rollout file's first line into sessionMeta.
func readSessionMeta(path string) (sessionMeta, bool) {
	f, err := os.Open(path)
	if err != nil {
		return sessionMeta{}, false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 256*1024), 4*1024*1024)
	if !sc.Scan() {
		return sessionMeta{}, false
	}
	var m rawMeta
	if err := json.Unmarshal(sc.Bytes(), &m); err != nil || m.Type != "session_meta" {
		return sessionMeta{}, false
	}
	return sessionMeta{
		CWD:        m.Payload.CWD,
		CLIVersion: m.Payload.CLIVersion,
		SessionID:  m.Payload.SessionID,
	}, true
}

// listRollouts returns all rollout file paths under the sessions dir, newest
// (by mtime) first.
func listRollouts() []string {
	base := codexSessionsDir()
	if base == "" {
		return nil
	}
	type ent struct {
		path string
		mod  int64
	}
	var ents []ent
	_ = filepath.Walk(base, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		name := info.Name()
		if strings.HasPrefix(name, "rollout-") && strings.HasSuffix(name, ".jsonl") {
			ents = append(ents, ent{p, info.ModTime().UnixNano()})
		}
		return nil
	})
	sort.Slice(ents, func(i, j int) bool { return ents[i].mod > ents[j].mod })
	out := make([]string, len(ents))
	for i, e := range ents {
		out[i] = e.path
	}
	return out
}

// newestRolloutForCWD finds the most recently modified rollout whose
// session_meta cwd matches the given working directory.
func newestRolloutForCWD(cwd string) (string, bool) {
	if cwd == "" {
		return "", false
	}
	for _, path := range listRollouts() {
		if meta, ok := readSessionMeta(path); ok && meta.CWD == cwd {
			return path, true
		}
	}
	return "", false
}

// tailLines returns up to the last n non-empty lines of a file.
func tailLines(path string, n int) []string {
	f, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 256*1024), 8*1024*1024)
	var all []string
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) != "" {
			all = append(all, sc.Text())
		}
	}
	if len(all) > n {
		all = all[len(all)-n:]
	}
	return all
}
