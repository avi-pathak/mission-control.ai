package gemini

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/avi-pathak/mission-control.ai/internal/protocol"
)

// A minimal but realistic chat session fixture mirroring ~/.gemini/tmp shapes.
// projectHash is sha256("/work/proj"), injected at write time.
func sessionFixture(hash string) string {
	return `{"sessionId":"abc","projectHash":"` + hash + `","startTime":"2026-07-17T03:57:52.781Z","kind":"main"}
{"$set":{"messages":[]}}
{"id":"m1","timestamp":"2026-07-17T03:57:52.783Z","type":"user","content":[{"text":"<session_context>bootstrap</session_context>"}]}
{"id":"m2","timestamp":"2026-07-17T03:57:56.753Z","type":"user","content":[{"text":"say hello"}]}
{"id":"m3","timestamp":"2026-07-17T03:57:59.662Z","type":"gemini","content":"Hello!","thoughts":[],"tokens":{"input":10221,"output":12,"cached":100,"thoughts":214,"tool":0,"total":10447},"model":"gemini-2.5-pro"}
`
}

func writeFixture(t *testing.T, cwd string) string {
	t.Helper()
	dir := t.TempDir()
	chats := filepath.Join(dir, "tmp", "persnal", "chats")
	if err := os.MkdirAll(chats, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(chats, "session-2026-07-17T03-57-abc.jsonl")
	if err := os.WriteFile(path, []byte(sessionFixture(projectHash(cwd))), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("GEMINI_HOME", dir)
	return path
}

func TestNewestSessionForCWD(t *testing.T) {
	writeFixture(t, "/work/proj")
	path, ok := newestSessionForCWD("/work/proj")
	if !ok {
		t.Fatalf("expected to find session for cwd")
	}
	if h, ok := readSessionHeader(path); !ok || h.ProjectHash != projectHash("/work/proj") {
		t.Fatalf("bad header: %+v ok=%v", h, ok)
	}
	// Non-matching cwd should not resolve.
	if _, ok := newestSessionForCWD("/other/dir"); ok {
		t.Fatalf("did not expect a session for unrelated cwd")
	}
}

func TestTokenUsageFromSession(t *testing.T) {
	path := writeFixture(t, "/work/proj")
	u := tokenUsageFromSession(path)
	if u == nil {
		t.Fatal("expected token usage")
	}
	if u.Input != 10221 || u.Output != 12+214 || u.CacheRead != 100 || u.Total != 10447 {
		t.Fatalf("bad usage: %+v", u)
	}
}

func TestRenderSessionLine(t *testing.T) {
	cases := []struct {
		raw  string
		want string // substring; "" means skipped
	}{
		{`{"type":"user","content":[{"text":"say hello"}]}`, "say hello"},
		{`{"type":"gemini","content":"Hi there"}`, "Hi there"},
		{`{"type":"user","content":[{"text":"<session_context>x</session_context>"}]}`, ""},
		{`{"$set":{"lastUpdated":"x"}}`, ""},
	}
	for _, c := range cases {
		got := renderSessionLine(c.raw)
		if c.want == "" {
			if got != "" {
				t.Fatalf("expected skip for %q, got %q", c.raw, got)
			}
			continue
		}
		if !contains(got, c.want) {
			t.Fatalf("render %q = %q, want substring %q", c.raw, got, c.want)
		}
	}
}

func TestClassifyPaneText(t *testing.T) {
	if got := classifyPaneText("some output\nDo you want to run this command? (y/n)"); got != protocol.StatusWaitingApproval {
		t.Fatalf("expected waiting-approval, got %v", got)
	}
	if got := classifyPaneText("just thinking...\n● working"); got != protocol.StatusRunning {
		t.Fatalf("expected running, got %v", got)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
