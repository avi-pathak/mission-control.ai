package agent

import (
	"strings"
	"testing"
)

func TestDeriveAgentIDForServer(t *testing.T) {
	// Different servers → different ids on the same machine.
	dev := DeriveAgentIDForServer("http://localhost:8080")
	prod := DeriveAgentIDForServer("https://missioncontrol.example.dev")
	if dev == prod {
		t.Fatalf("expected distinct ids per server, both = %s", dev)
	}

	// Same server → stable id.
	if got := DeriveAgentIDForServer("http://localhost:8080"); got != dev {
		t.Fatalf("expected stable id for same server: %s != %s", got, dev)
	}

	// Both share the base host id prefix (same physical machine).
	baseDev := dev[:strings.LastIndex(dev, "-")]
	baseProd := prod[:strings.LastIndex(prod, "-")]
	if baseDev != baseProd {
		t.Fatalf("expected same base host id, got %s vs %s", baseDev, baseProd)
	}

	// Per-server ids carry an 8-hex suffix.
	suffix := dev[strings.LastIndex(dev, "-")+1:]
	if len(suffix) != 8 {
		t.Fatalf("expected 8-char server suffix, got %q", suffix)
	}

	// Empty server → bare host id (no server suffix beyond the base).
	bare := DeriveAgentIDForServer("")
	if bare != baseDev {
		t.Fatalf("empty server should yield bare host id %s, got %s", baseDev, bare)
	}

	// ws:// and http:// to the same host+port → same id (scheme-insensitive host).
	a := DeriveAgentIDForServer("ws://localhost:8080")
	b := DeriveAgentIDForServer("http://localhost:8080")
	if a != b {
		t.Fatalf("ws and http to same host should match: %s vs %s", a, b)
	}
}

func TestServerHost(t *testing.T) {
	cases := map[string]string{
		"https://missioncontrol.example.dev":      "missioncontrol.example.dev",
		"http://localhost:8080":                   "localhost:8080",
		"ws://localhost:8080/ws?role=agent":       "localhost:8080",
		"HTTPS://Example.COM/Path":                "example.com",
		"":                                        "",
		"missioncontrol.example.dev":              "missioncontrol.example.dev",
	}
	for in, want := range cases {
		if got := serverHost(in); got != want {
			t.Errorf("serverHost(%q) = %q, want %q", in, got, want)
		}
	}
}
