# Fix: Logs & Terminal are empty

## Root cause
Both the **Logs** and **Terminal** tabs render `store.logs[session.id]`, which is
populated only by `log.append` messages from the agent. The ClaudeProvider's
`Logs()` returns `nil` unless the session runs inside **tmux** — but the Claude
Code sessions on this machine (and most users') run in a plain terminal, so
`TmuxSession == ""` and **no logs are ever produced**. Terminal is empty for the
same reason (same data source).

The real log source is Claude Code's **session transcript**: newline-delimited
JSON at `~/.claude/projects/<encoded-cwd>/<sessionUuid>.jsonl`, appended live as
the conversation progresses. `<encoded-cwd>` replaces `/` and `.` with `-`
(e.g. `/Users/avinash/work/persnal` → `-Users-avinash-work-persnal`).

## Approach
Add a **transcript log source** to the ClaudeProvider and make `Logs()` prefer
it, falling back to tmux.

### New file: `internal/provider/claude/transcript.go`
- `transcriptFile(cwd string) (string, bool)` — encode cwd → project dir under
  `~/.claude/projects/`; pick the **most recently modified** `*.jsonl` (that's
  the active session for that working directory). Returns "" if none.
- `tailTranscript(ctx, s, path) <-chan provider.LogLine` — a goroutine that:
  1. On start, reads the existing file and emits the **last N** (e.g. 200)
     rendered lines as backfill (so opening Logs shows history immediately).
  2. Then polls for growth (size increases) every ~1s and emits new lines as
     they're appended — a simple, dependency-free tail that survives the file
     being truncated/rotated (reset offset if size shrinks).
  3. Parses each JSON line and renders a compact, human-readable log line:
     - `user` → `▸ user: <text>` (green-ish via ANSI)
     - `assistant` text block → `● assistant: <text>`
     - `assistant` tool_use block → `⚙ tool: <name> <compact input>`
     - tool results / `system` → `  <summary>`
     - skip noise types (`mode`, `permission-mode`, `file-history-snapshot`,
       `summary`, snapshots).
   Emit ANSI color codes so the existing `ansiToHtml` renderer (Logs tab) and
   xterm (Terminal tab) both show color. Long text is wrapped/truncated per line.

### Edit: `internal/provider/claude/claude.go`
- `Logs()`: if a transcript file exists for `s.CWD`, return
  `tailTranscript(...)`; else if tmux, tail the pane; else `nil`.
- Discovery already has `cwd`; no session-shape change needed.

### Matching session → transcript
Discovery keys sessions by PID, not the Claude session UUID, so I map by **cwd**
→ newest `.jsonl`. This is correct for the common case (one active Claude per
repo). Edge case (two Claudes in the same cwd) both tail the same newest file —
acceptable for the MVP; noted in a comment. (A future refinement could match the
UUID via the process's open file descriptors.)

## Why this is the right fix
- Makes Logs **and** Terminal work for *all* Claude sessions, not just tmux ones
  — the actual reported bug.
- No new dependencies; pure stdlib file tail + JSON.
- Reuses the existing `provider.LogLine` channel + `log.append` pipeline and the
  dashboard's ANSI renderer — no server/dashboard changes required.

## Verification
- Unit test: `transcriptFile` encoding + newest-file selection against a temp
  `HOME` with fixture `.jsonl` files.
- Unit test: transcript line rendering for user/assistant/tool_use fixtures.
- Live: restart the agent, open a session's **Logs** tab → see recent
  conversation history stream in with color; **Terminal** shows the same;
  append to a real transcript and confirm new lines appear within ~1s.

## Scope guard
Read-only. The Terminal stays read-only (xterm `disableStdin`). No change to the
protocol, server, store, or dashboard code — this is contained to the agent's
Claude provider.
