// Package gemini implements the AgentProvider for Google Gemini CLI sessions.
//
// The Gemini CLI writes newline-delimited JSONL chat logs to
// ~/.gemini/tmp/<project>/chats/session-<ts>-<id>.jsonl (honoring $GEMINI_HOME).
// Line 1 is a session header carrying the projectHash (sha256 of the working
// directory) which lets us map a live process's cwd to its session file. Later
// lines are user/gemini messages; gemini messages carry a cumulative token
// count we surface as usage.
package gemini

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// geminiTmpDir returns ~/.gemini/tmp, honoring $GEMINI_HOME.
func geminiTmpDir() string {
	if h := os.Getenv("GEMINI_HOME"); h != "" {
		return filepath.Join(h, "tmp")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".gemini", "tmp")
}

// projectHash mirrors the Gemini CLI: the sha256 hex of the absolute cwd.
func projectHash(cwd string) string {
	sum := sha256.Sum256([]byte(cwd))
	return hex.EncodeToString(sum[:])
}

// sessionHeader is the parsed first line of a chat session file.
type sessionHeader struct {
	SessionID   string `json:"sessionId"`
	ProjectHash string `json:"projectHash"`
}

// readSessionHeader parses a session file's first line.
func readSessionHeader(path string) (sessionHeader, bool) {
	f, err := os.Open(path)
	if err != nil {
		return sessionHeader{}, false
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 256*1024), 4*1024*1024)
	if !sc.Scan() {
		return sessionHeader{}, false
	}
	var h sessionHeader
	if err := json.Unmarshal(sc.Bytes(), &h); err != nil || h.ProjectHash == "" {
		return sessionHeader{}, false
	}
	return h, true
}

// listSessions returns all chat session file paths under the tmp dir, newest
// (by mtime) first.
func listSessions() []string {
	base := geminiTmpDir()
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
		// chats/session-*.jsonl
		if strings.HasPrefix(name, "session-") && strings.HasSuffix(name, ".jsonl") &&
			filepath.Base(filepath.Dir(p)) == "chats" {
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

// newestSessionForCWD finds the most recently modified session file whose
// projectHash matches the given working directory.
func newestSessionForCWD(cwd string) (string, bool) {
	if cwd == "" {
		return "", false
	}
	want := projectHash(cwd)
	for _, path := range listSessions() {
		if h, ok := readSessionHeader(path); ok && h.ProjectHash == want {
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
