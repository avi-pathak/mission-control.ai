package agent

import (
	"encoding/base64"
	"runtime"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
)

// TestPTYEchoRoundTrip opens a `cat` PTY, writes bytes, and verifies they echo
// back through the output callback.
func TestPTYEchoRoundTrip(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("pty not supported on windows")
	}
	m := newTermManager(zap.NewNop())

	var mu sync.Mutex
	var out []byte
	opened := make(chan bool, 1)

	m.onOutput = func(_ string, data []byte) {
		mu.Lock()
		out = append(out, data...)
		mu.Unlock()
	}
	m.onOpened = func(_, _ string, ok bool, _ string) { opened <- ok }
	m.onExit = func(_ string, _ int) {}

	// `cat` echoes stdin back to stdout.
	m.Open("pty1", "sess1", []string{"cat"}, "", nil, 80, 24)
	if ok := <-opened; !ok {
		t.Fatal("pty failed to open")
	}

	m.Input("pty1", base64.StdEncoding.EncodeToString([]byte("hello\n")))

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		got := string(out)
		mu.Unlock()
		if len(got) > 0 && contains(got, "hello") {
			m.Close("pty1")
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	m.Close("pty1")
	t.Fatalf("did not see echoed input; got %q", string(out))
}

func TestPTYOpenInvalidCommand(t *testing.T) {
	m := newTermManager(zap.NewNop())
	res := make(chan bool, 1)
	m.onOutput = func(string, []byte) {}
	m.onOpened = func(_, _ string, ok bool, _ string) { res <- ok }
	m.onExit = func(string, int) {}

	m.Open("p", "s", []string{"this-command-does-not-exist-xyz"}, "", nil, 80, 24)
	if ok := <-res; ok {
		t.Fatal("expected open to fail for missing command")
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
