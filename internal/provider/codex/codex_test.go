package codex

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/avi-pathak/mission-control.ai/internal/protocol"
)

// A minimal but realistic rollout fixture mirroring ~/.codex/sessions shapes.
const rolloutFixture = `{"type":"session_meta","payload":{"session_id":"abc","cwd":"/work/proj","cli_version":"0.144.0","git":{}}}
{"type":"turn_context","payload":{}}
{"type":"event_msg","payload":{"type":"user_message","message":"build the thing"}}
{"type":"response_item","payload":{"type":"function_call","name":"exec_command","arguments":"{}"}}
{"type":"response_item","payload":{"type":"function_call_output","output":"ok"}}
{"type":"event_msg","payload":{"type":"agent_message","message":"done building"}}
{"type":"event_msg","payload":{"type":"token_count","info":{"total_token_usage":{"input_tokens":1000,"cached_input_tokens":800,"output_tokens":50,"reasoning_output_tokens":10,"total_tokens":1060}}}}
`

func writeFixture(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	sessions := filepath.Join(dir, "sessions", "2026", "07", "12")
	if err := os.MkdirAll(sessions, 0o755); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(sessions, "rollout-x-abc.jsonl")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODEX_HOME", dir)
	return path
}

func TestReadSessionMeta(t *testing.T) {
	path := writeFixture(t, rolloutFixture)
	m, ok := readSessionMeta(path)
	if !ok || m.CWD != "/work/proj" || m.CLIVersion != "0.144.0" {
		t.Fatalf("bad meta: %+v ok=%v", m, ok)
	}
}

func TestNewestRolloutForCWD(t *testing.T) {
	writeFixture(t, rolloutFixture)
	path, ok := newestRolloutForCWD("/work/proj")
	if !ok || path == "" {
		t.Fatal("expected to find rollout for cwd")
	}
	if _, ok := newestRolloutForCWD("/nope"); ok {
		t.Fatal("should not match a different cwd")
	}
}

func TestTokenUsageFromRollout(t *testing.T) {
	path := writeFixture(t, rolloutFixture)
	u := tokenUsageFromRollout(path)
	if u == nil {
		t.Fatal("expected token usage")
	}
	if u.Input != 1000 || u.CacheRead != 800 || u.Output != 60 /*50+10*/ || u.Total != 1060 {
		t.Fatalf("bad usage: %+v", u)
	}
}

func TestClassifyRolloutTail(t *testing.T) {
	call := `{"type":"response_item","payload":{"type":"function_call","name":"x"}}`
	out := `{"type":"response_item","payload":{"type":"function_call_output"}}`
	msg := `{"type":"response_item","payload":{"type":"message"}}`

	if got := classifyRolloutTail([]string{msg, call}); got != protocol.StatusWaitingApproval {
		t.Fatalf("dangling call should be waiting, got %s", got)
	}
	if got := classifyRolloutTail([]string{call, out}); got != protocol.StatusRunning {
		t.Fatalf("completed call should be running, got %s", got)
	}
	if got := classifyRolloutTail([]string{call, msg}); got != protocol.StatusRunning {
		t.Fatalf("trailing message should be running, got %s", got)
	}
}

func TestClassifyPaneText(t *testing.T) {
	if classifyPaneText("Allow command? (y/n)") != protocol.StatusWaitingApproval {
		t.Fatal("approval prompt not detected")
	}
	if classifyPaneText("running tests\nediting file") != protocol.StatusRunning {
		t.Fatal("working output misclassified")
	}
}

func TestRenderRolloutLine(t *testing.T) {
	cases := map[string]string{
		`{"type":"event_msg","payload":{"type":"user_message","message":"hi"}}`:            "hi",
		`{"type":"event_msg","payload":{"type":"agent_message","message":"ok"}}`:           "ok",
		`{"type":"response_item","payload":{"type":"function_call","name":"exec"}}`:        "tool: exec",
		`{"type":"turn_context","payload":{}}`:                                             "",
		`garbage`:                                                                          "",
	}
	for in, want := range cases {
		got := renderRolloutLine(in)
		if want == "" {
			if got != "" {
				t.Fatalf("expected skip for %q, got %q", in, got)
			}
			continue
		}
		if !contains(got, want) {
			t.Fatalf("render(%q) = %q, want substring %q", in, got, want)
		}
	}
}

func TestIsCodexProcess(t *testing.T) {
	if !isCodexProcess("codex", "/usr/local/bin/codex") {
		t.Fatal("codex binary not detected")
	}
	if !isCodexProcess("node", "codex exec --json") {
		t.Fatal("codex exec not detected")
	}
	if isCodexProcess("mission-control-agent", "/tmp/mission-control-agent --config x") {
		t.Fatal("should not match our own agent")
	}
	if isCodexProcess("bash", "echo hello") {
		t.Fatal("unrelated process matched")
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
