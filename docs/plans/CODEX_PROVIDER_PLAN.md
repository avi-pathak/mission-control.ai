# Plan: Codex CLI provider (`internal/provider/codex`)

Built against the **real** rollout schema from `~/.codex/sessions/` on this
machine (not assumptions). A self-contained package implementing `AgentProvider`
+ `LaunchableProvider` + `MachineAware`, reusing `internal/tmux`, `gitinfo`,
gopsutil metrics, and signal-based control — **zero edits to agent core /
protocol / server**, plus one `codex.Register()` line in main.go.

## Confirmed schema (from live data)
- Rollout files: `~/.codex/sessions/YYYY/MM/DD/rollout-<ts>-<uuid>.jsonl`.
- **session_meta** (line 1): `payload.cwd`, `payload.cli_version`, `payload.git`,
  `payload.originator`. This maps a rollout → working dir.
- **event_msg / token_count**: `payload.info.total_token_usage` is **cumulative**
  `{input_tokens, cached_input_tokens, output_tokens, reasoning_output_tokens,
  total_tokens}` — no summing needed (cleaner than Claude).
- **event_msg** subtypes: `task_started`, `task_complete`, `agent_message`,
  `user_message`, `patch_apply_end`, `mcp_tool_call_end`, `token_count`.
- **response_item** payload types: `message`, `function_call`,
  `function_call_output`, `reasoning`, `custom_tool_call(_output)`.

## Package files

### `codex.go` — Provider + AgentProvider
- `Name() → "codex"`.
- `Register()` + factory; `SetMachineID` (MachineAware).
- `Discover`: enumerate host processes (gopsutil), match `isCodexProcess`
  (process name/cmdline containing `codex`, excluding our own agent). For each
  match: resolve `cwd` from the process; enrich with `gitinfo.Collect`,
  `tmux.SessionForPID`, token usage + version + status from the newest rollout
  whose `session_meta.cwd == process cwd`. Session id = `<machineID>-<pid>`.
- `Metrics`: gopsutil CPU/RSS by PID (copy Claude's — identical).
- `Control`: SIGTERM (stop) / SIGINT (restart) — copy Claude's.
- `Launch` (LaunchableProvider): `Argv: ["codex"]`, validate cwd.

### `rollout.go` — find + parse rollout files
- `newestRolloutForCWD(cwd) (path, ok)`: scan `~/.codex/sessions/**/rollout-*.jsonl`
  (honor `CODEX_HOME`), read each file's **first line** (session_meta), match
  `payload.cwd == cwd`, pick the newest by file mtime. Cache dir listing briefly
  to avoid rescanning every poll (small LRU / mtime check).
- `sessionMeta(path)`: parse line 1 → `{cwd, cliVersion, git}`.
- Reuse for version + token + status extraction (single tail read).

### `tokens.go` — token usage
- `tokenUsageFromRollout(path) *protocol.TokenUsage`: scan for the **last**
  `event_msg` with `payload.type == "token_count"`, read
  `info.total_token_usage`. Map: input→Input, output→Output,
  cached_input→CacheRead, reasoning_output→(fold into Output or a note),
  total→Total. (No cache-creation concept in Codex → leave 0.)

### `logs.go` — log streaming
- `Logs`: tail the rollout JSONL and render human-readable lines
  (agent_message → "● codex: …", function_call → "⚙ tool: <name>",
  user_message → "▸ user: …"), ANSI-colored like Claude's transcript renderer.
  Reuse the offset-polling tail pattern. Fall back to `tmux.TailPane` when the
  session runs under tmux and no rollout is found.

### `status.go` — waiting-for-approval detection
- **tmux sessions**: scan pane via tmux for Codex's approval UI (patterns like
  `Allow command?`, `approve`, `y/n`, numbered choices) — reuse the regex
  approach; Codex-specific prompt strings.
- **non-tmux**: heuristic from rollout tail — if the last meaningful event is a
  `function_call` / tool call with **no following `function_call_output`**, the
  turn is paused (awaiting approval/exec), → `waiting_approval`. If last is
  `task_complete` → idle/finished-ish (keep `running` to be safe unless clearly
  ended). Pure, unit-testable `classifyRolloutTail([]event)`.

## Wiring (tiny)
- `apps/agent/main.go`: add `codex.Register()` next to `claude.Register()`.
- `internal/config/agent.go`: leave default providers `["claude-code"]`; users
  add `"codex"` to `providers:` in agent.yaml to enable. (Enabling both is fine —
  discovery just runs both.)

## Dashboard (small, provider-aware)
- **New Session dialog**: add a provider dropdown (Claude Code / Codex) →
  passes `provider` to `terminal.open` (the field already flows through).
- Everything else already generic (`session.provider`, `version`, token cards,
  status badges) after the refactor — no per-provider UI code needed.

## Build order
1. `rollout.go` (find + session_meta parse) — unit-tested against a fixture
   copied from a real rollout (sanitized).
2. `tokens.go` + test (parse `total_token_usage`).
3. `status.go` classifier + test (dangling function_call → waiting).
4. `logs.go` renderer + test (event → rendered line).
5. `codex.go` provider assembling it; `Register()`.
6. main.go wiring + dashboard provider picker.
7. `go build/test`, `pnpm typecheck/build`. Commit.

## Verification
- **Unit (fully local, no codex binary needed):** rollout parsing, token
  extraction, status classification, log rendering — all tested against the real
  JSONL shapes captured from `~/.codex/sessions/`.
- **Live (partial):** `codex` is **not installed** on this machine, so
  process-discovery and interactive launch/approve can't be exercised here. I'll
  verify the log/token/status pipeline by pointing the parser at your existing
  rollout files (feed a real session_meta cwd) and confirm it produces correct
  Session data. Full discovery/interactive verification needs codex installed —
  I'll document exactly how to test it once it is.

## Honest scope note
- **Fully testable now:** log parsing, token usage, status heuristic, version —
  because your real rollout files exist.
- **Not testable now (no `codex` binary):** live process discovery, interactive
  launch, tmux approve. The code paths reuse the proven Claude/tmux
  infrastructure, but I can't run them end-to-end until codex is installed. This
  is the one caveat — the parsing half is solid; the process half is
  "implemented + unit-tested, pending a live codex to confirm the process-match
  predicate."
