# Plan: Detect & highlight blocked (waiting-for-approval) sessions

## The gap
Every session is hardcoded `StatusRunning` in the Claude provider — the
`waiting_approval` status already exists end-to-end (protocol, StatusBadge,
FilterBar, status chart) but is **never set**. So a session blocked on a
permission prompt looks identical to a working one.

## Detection (agent, `internal/provider/claude`)
Determine a session's real status during discovery. Two complementary signals:

**A. tmux-backed sessions — scan the live pane** (authoritative):
- `tmux capture-pane` and look for Claude's approval UI: lines matching
  `Do you want to proceed?`, `Do you want to`, numbered choices `❯ 1. Yes` /
  `1. Yes` / `2. No`, `(y/n)`, `Allow this`, etc. → `waiting_approval`.
- If the pane shows the idle input prompt (`❯ ` with an empty/edited line and no
  question) → `running` (or `idle`).

**B. non-tmux sessions — inspect the transcript tail** (heuristic):
- If the **last** message is an assistant `tool_use` block with **no following
  `user`/`tool_result`** line, the turn is paused — likely awaiting permission →
  `waiting_approval`.
- If the last message is a completed exchange or the process is actively writing
  → `running`.
- Add a short debounce (only flip to waiting if the tail has been unchanged for
  ~2s) to avoid flagging mid-stream tool calls.

Wrap this in `detectStatus(ctx, session) SessionStatus` with a small,
well-tested pure core (`classifyPaneText`, `classifyTranscriptTail`) so the
matching logic is unit-testable without tmux/processes.

## Status transitions & events
- The agent already emits `status.changed` activity events when a session's
  status differs between polls (via `diffSessionEvents`). Setting the real status
  means these now fire for running→waiting_approval and back — so the **Activity
  feed automatically logs "waiting for approval"** with the right severity
  (warn). No new event type needed.
- Bump severity: a session entering `waiting_approval` is a `warn` event (already
  mapped in `severityForStatus`).

## Dashboard highlighting
Make blocked sessions visually pop (the ask):
- **Session table row**: when `status === 'waiting_approval'`, add a subtle amber
  left-accent + a pulsing amber dot, and sort/keep them visible (optionally pin
  waiting sessions to the top).
- **Overview**: the "Waiting" stat card already exists; add a subtle amber ring
  when count > 0 so it draws the eye.
- **A fleet banner**: if any session is waiting, show a dismissible amber strip
  at the top of Overview: "N session(s) waiting for your approval →" linking to
  a filtered view.
- **Session page header**: an amber "Waiting for approval" badge + (for tmux
  sessions) a prominent Approve/Deny — reuse the existing InteractiveTerminal
  toolbar; for tmux sessions the user can approve inline.
- **Sidebar/Sessions nav**: a small amber count badge when sessions are waiting.
- **Favicon/title (optional, nice)**: prefix the tab title with `(N) ` when
  sessions are waiting, so it's visible even when the tab is backgrounded.

## Notifications (optional, small)
- Browser `Notification` (with permission opt-in from Settings) when a session
  transitions into `waiting_approval`, so you're pinged even if the tab is
  hidden. Gate behind a toggle; skip if not granted.

## Build order
1. Agent: `detectStatus` + pure classifiers (pane + transcript-tail) with unit
   tests; wire into `Discover` replacing the hardcoded `StatusRunning`. Debounce.
2. Verify `status.changed` events flow and the existing badge/filter render the
   waiting state (no protocol change needed).
3. Dashboard: table row emphasis + waiting pinned-to-top, Overview banner + stat
   ring, session-page badge, nav count.
4. Optional: title-bar count + browser notifications behind a Settings toggle.
5. Rebuild + (later) push images; commit.

## Verification
- Unit: `classifyPaneText` (proceed prompt, y/n, numbered choices, idle prompt),
  `classifyTranscriptTail` (dangling tool_use → waiting; completed → running).
- Live: trigger a real permission prompt in the tmux `work` session → its row
  turns amber, the Overview banner appears, Activity logs "waiting", and the
  count badge shows; approve it → returns to running.

## Scope guard
- Detection is best-effort and conservative (debounced) to avoid false
  "waiting" flags mid-stream.
- Non-tmux detection is heuristic (transcript tail) — documented as such; tmux
  sessions get authoritative pane-based detection.
- No protocol/DB changes required — reuses the existing status field + events.
