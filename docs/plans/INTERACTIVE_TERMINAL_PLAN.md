# Plan: Interactive Terminal (fully interactive xterm)

Goal: from the dashboard, **start a new Claude session** in a chosen repo and
**drive it live** (type prompts, answer permission approvals) through a fully
interactive xterm — plus attach interactively to sessions the agent owns.

Decisions locked:
- **Both**: launch new interactive sessions from the dashboard **and** interact
  with existing agent-owned/tmux sessions.
- **Fully interactive xterm**: keystrokes stream to the session PTY in real time.
- **This first**, before remove-machine / charts / token-usage.

## Why a PTY
To make xterm truly interactive (arrow keys, Ctrl-C, Claude's y/n approval
prompts, full TUI redraw), the agent must run Claude under a **pseudo-terminal**
it owns. The agent then:
- streams raw PTY output → dashboard (bytes, not parsed log lines),
- writes dashboard keystrokes → PTY stdin,
- forwards resize events.

Existing non-PTY sessions (the 20 plain ones) stay read-only — no OS hook exists
to inject into them. Interactive = agent-launched (or tmux, via send-keys).

## Protocol (Go + TS) — new message types
A dedicated terminal channel, separate from `log.append` (which stays for
passive logs):

Dashboard → Server → Agent:
- `terminal.open`   {ptyId, provider, cwd/repoPath, cols, rows}  → launch Claude under a PTY
- `terminal.attach` {ptyId, sessionId}                          → attach to an existing agent PTY
- `terminal.input`  {ptyId, data}                               → keystrokes (base64/raw)
- `terminal.resize` {ptyId, cols, rows}
- `terminal.close`  {ptyId}

Agent → Server → Dashboard:
- `terminal.output` {ptyId, data}     → raw PTY bytes
- `terminal.opened` {ptyId, sessionId, ok, error}
- `terminal.exit`   {ptyId, code}

`ptyId` is a dashboard-generated uuid. The server routes by `ptyId` → owning
agent, and fans `terminal.output` back only to the requesting org's dashboards
(scoped like everything else). Bytes are base64 in JSON (keeps the envelope
simple; a later gRPC/binary frame is possible).

## Server
- Hub: dashboards can already send (`OnDashboardMessage`) — wire it. Maintain a
  `ptyId → (agentMachineId, requesting dashboard org)` map so output routes back
  and is org-scoped. Authorize: the dashboard's org must own the target agent.
- New handler `onDashboardMessage`: validate org owns the machine, forward
  terminal.* to the agent via `SendToAgent`; forward agent terminal.output to
  the org's dashboards.
- No DB persistence for the live stream (ephemeral); optionally tee output into
  the existing log pipeline so the Logs tab still has history.

## Agent
- Add dep `github.com/creack/pty`.
- New `internal/agent/terminal.go`: a PTY manager keyed by ptyId.
  - `terminal.open`: resolve repo path, `exec.Command("claude", ...)` (provider-
    supplied argv), start under `pty.Start`, register a new Session (so it shows
    in the table with a "interactive" flag), stream `pty` output as
    `terminal.output`, apply `terminal.input`/`terminal.resize`, emit
    `terminal.exit` on process end.
  - `terminal.attach`: for a session the agent launched, re-subscribe output.
  - tmux fallback: if the target session has a tmuxSession, `terminal.input`
    maps to `tmux send-keys` and output continues via the existing pane capture
    (covers "approve existing tmux session" without a PTY).
- Provider gains an optional `Launch(ctx, opts) (argv, cwd, error)` so
  ClaudeProvider defines how to start Claude; keeps it extensible to Codex/etc.
- Security: only allow launching the configured provider binary; cwd must be a
  real dir; never exec arbitrary strings from the dashboard.

## Dashboard
- **Terminal tab → interactive mode** when the session is agent-owned/PTY:
  - xterm with `disableStdin: false`; `term.onData` → `terminal.input`;
    `term.onResize`/FitAddon → `terminal.resize`; render `terminal.output` bytes
    via `term.write`.
  - A small banner: "Interactive — you're driving this session."
  - Read-only sessions keep the current passive view (with a note).
- **New Session flow**: a "New Session" button (Overview/Machines) → dialog:
  pick machine (online, in org) + repo path (+ optional initial prompt) →
  sends `terminal.open` → on `terminal.opened` navigate to the session's
  Terminal tab, already live.
- Live store: a terminal slice keyed by ptyId (or sessionId) with an event
  emitter the TerminalTab subscribes to (raw bytes, not the logs array).
- WS client (`@mc/shared`) already supports `send`; expose typed helpers.

## Build order
1. Protocol types (Go + TS) + WS `send` plumbing dashboard→server→agent.
2. Agent PTY manager + ClaudeProvider `Launch`; unit-test argv/cwd resolution;
   local manual PTY echo test.
3. Server routing + org authorization; integration test (dashboard opens pty →
   receives output → input echoes back) using a fake `cat` PTY.
4. Dashboard: interactive xterm wiring + New Session dialog + terminal store.
5. tmux send-keys path for existing tmux sessions (approvals).
6. Rebuild+push Docker images (agent PTY code must ship in the bundled binaries).

## Verification
- Go: PTY manager unit test (open echo cmd, write "hi", read "hi"); server
  routing/authorization test (org B cannot open a pty on org A's agent).
- Live: from the dashboard, start a Claude session in a repo, type a prompt,
  see it respond; trigger a permission prompt and approve it with `y` from the
  dashboard; confirm org isolation (another org can't attach).

## Honest constraints
- Only **agent-launched** (PTY) or **tmux** sessions are interactive. The 20
  existing plain sessions remain read-only — unchanged OS limitation.
- Raw bytes over JSON base64 is slightly heavy but simple and correct; fine for
  interactive typing volumes. Binary WS frames are a later optimization.
- macOS agent must run **natively** (already true) — PTY under Docker-on-Mac
  wouldn't see host repos anyway.

After this ships, I'll do the other three (remove-machine, charts, token usage)
from the previous plan.
