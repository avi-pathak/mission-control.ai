package claude

import "testing"

func TestIsClaudeProcess(t *testing.T) {
	cases := []struct {
		name    string
		cmdline string
		want    bool
		why     string
	}{
		// --- real CLI sessions (should match) ---
		{"claude", "claude", true, "standalone CLI"},
		{"claude.exe", "claude", true, "gopsutil reports name as claude.exe (tmux/shell launched)"},
		{"Claude", "claude", true, "capitalized name from gopsutil"},
		{
			"claude",
			"/Users/x/.vscode/extensions/anthropic.claude-code-2.1.211-darwin-arm64/resources/native-binary/claude --output-format stream-json --verbose",
			true, "VS Code extension native CLI",
		},
		{
			"node",
			"node /usr/local/lib/node_modules/@anthropic-ai/claude-code/cli.js",
			true, "npm-installed CLI via node",
		},
		{"claude", "/opt/homebrew/bin/claude --resume", true, "homebrew CLI with args"},

		// --- desktop app + helpers (must NOT match) ---
		{
			"disclaimer",
			"/Applications/Claude.app/Contents/Helpers/disclaimer /Users/x/Library/Application Support/Claude/...",
			false, "desktop app disclaimer helper",
		},
		{
			"Claude Helper",
			"/Applications/Claude.app/Contents/Frameworks/Claude Helper.app/Contents/MacOS/Claude Helper --type=renderer",
			false, "electron renderer helper",
		},
		{
			"Claude Helper (GPU)",
			"/Applications/Claude.app/Contents/Frameworks/Claude Helper (GPU).app/Contents/MacOS/Claude Helper --type=gpu-process",
			false, "electron gpu helper",
		},
		{
			"chrome_crashpad",
			"/Applications/Claude.app/Contents/Frameworks/... chrome_crashpad_handler",
			false, "crashpad",
		},
		{
			"claude",
			"/Users/x/Library/Application Support/Claude/claude-code/2.1.209/claude.app/Contents/MacOS/claude helper",
			false, "bundled app helper (not the CLI)",
		},

		// --- tool subprocesses spawned by the CLI (must NOT match) ---
		{
			"ugrep",
			"ugrep -G --ignore-files --hidden -I --exclude-dir=.git /Users/x/.claude/whatever",
			false, "ugrep tool subprocess carrying a claude path",
		},
		{"grep", "grep -r claude-code ./src", false, "grep mentioning claude-code"},

		// --- our own agent (must NOT match) ---
		{"mission-control-agent", "/usr/local/bin/mission-control-agent --config x", false, "the agent itself"},

		// --- unrelated ---
		{"vim", "vim notes.md", false, "unrelated process"},
	}

	for _, c := range cases {
		got := isClaudeProcess(c.name, c.cmdline)
		if got != c.want {
			t.Errorf("isClaudeProcess(%q, %q) = %v, want %v — %s", c.name, c.cmdline, got, c.want, c.why)
		}
	}
}

func TestIsClaudeProcess_BundledCLI(t *testing.T) {
	// The real CLI shipped inside the app bundle (NOT a helper) should match.
	if !isClaudeProcess("claude", "/Users/x/Library/Application Support/Claude/claude-code/2.1.209/claude --resume") {
		t.Error("bundled claude-code CLI should be detected as a session")
	}
	// But the same version's app helper should not.
	if isClaudeProcess("claude", "/Users/x/Library/Application Support/Claude/claude-code/2.1.209/claude.app/Contents/MacOS/claude helper") {
		t.Error("bundled app helper must not be detected")
	}
}
