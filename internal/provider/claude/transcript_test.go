package claude

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestEncodeCWD(t *testing.T) {
	cases := map[string]string{
		"/Users/a/work/persnal":  "-Users-a-work-persnal",
		"/Users/a/work/x.y":      "-Users-a-work-x-y",
		"/tmp/repo":              "-tmp-repo",
	}
	for in, want := range cases {
		if got := encodeCWD(in); got != want {
			t.Errorf("encodeCWD(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestTranscriptFileNewest(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	cwd := "/Users/a/proj"
	dir := filepath.Join(home, ".claude", "projects", encodeCWD(cwd))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	old := filepath.Join(dir, "old.jsonl")
	newf := filepath.Join(dir, "new.jsonl")
	if err := os.WriteFile(old, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newf, []byte("{}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Make new.jsonl clearly newer.
	future := time.Now().Add(time.Hour)
	_ = os.Chtimes(newf, future, future)

	got, ok := transcriptFile(cwd)
	if !ok || filepath.Base(got) != "new.jsonl" {
		t.Fatalf("expected new.jsonl, got %q ok=%v", got, ok)
	}

	// Unknown cwd → not found.
	if _, ok := transcriptFile("/Users/a/nope"); ok {
		t.Fatal("expected no transcript for unknown cwd")
	}
}

func TestRenderTranscriptLine(t *testing.T) {
	cases := []struct {
		name string
		raw  string
		want string // substring expected in output ("" means skipped)
	}{
		{"mode skipped", `{"type":"mode","mode":"x"}`, ""},
		{"snapshot skipped", `{"type":"file-history-snapshot","snapshot":{}}`, ""},
		{"user string", `{"type":"user","message":{"role":"user","content":"hello there"}}`, "hello there"},
		{
			"assistant text",
			`{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"working on it"}]}}`,
			"working on it",
		},
		{
			"assistant tool_use",
			`{"type":"assistant","message":{"role":"assistant","content":[{"type":"tool_use","name":"Bash","input":{"cmd":"ls"}}]}}`,
			"tool: Bash",
		},
		{"garbage skipped", `not json`, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := renderTranscriptLine(c.raw)
			if c.want == "" {
				if got != "" {
					t.Fatalf("expected skip, got %q", got)
				}
				return
			}
			if !strings.Contains(got, c.want) {
				t.Fatalf("output %q missing %q", got, c.want)
			}
		})
	}
}

func TestRenderTailAndReadFrom(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "t.jsonl")
	content := `{"type":"user","message":{"role":"user","content":"one"}}
{"type":"mode","mode":"x"}
{"type":"assistant","message":{"role":"assistant","content":[{"type":"text","text":"two"}]}}
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	lines, end := renderTail(path, 10)
	if len(lines) != 2 { // mode line skipped
		t.Fatalf("expected 2 rendered lines, got %d: %v", len(lines), lines)
	}
	if end != int64(len(content)) {
		t.Fatalf("end offset = %d, want %d", end, len(content))
	}

	// Append and read from the offset.
	extra := `{"type":"user","message":{"role":"user","content":"three"}}` + "\n"
	f, _ := os.OpenFile(path, os.O_APPEND|os.O_WRONLY, 0o644)
	_, _ = f.WriteString(extra)
	_ = f.Close()

	newLines, _ := readFrom(path, end)
	if len(newLines) != 1 || !strings.Contains(newLines[0], "three") {
		t.Fatalf("expected the appended line, got %v", newLines)
	}
}
