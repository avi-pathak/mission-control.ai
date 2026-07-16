# Activity Feed — agent-detected events → control panel timeline

## Goal
The agent, during its existing polling, detects **everything happening** across
sessions/machines and reports each as a structured **event**. The server
persists events and broadcasts them live; the dashboard shows a real-time
**Activity** timeline (global + per-session).

This is additive and reuses infrastructure that already exists but is unused:
the `store.Event` model + `RecordEvent` helper.

## What counts as an "activity" (event kinds)
Detected by diffing each poll against the agent's last-known state:

| Kind | Trigger (agent-side) |
|------|----------------------|
| `session.started` | a session ID appears that wasn't seen before |
| `session.ended` | a session ID disappears |
| `status.changed` | `status` field differs (e.g. running → waiting_approval → error) |
| `branch.changed` | git branch differs |
| `commit.created` | newest git commit hash differs (new commit detected) |
| `command.changed` | `currentCommand` changes materially |
| `command.issued` | a stop/restart control was executed (already have result path) |
| `agent.connected` / `agent.disconnected` | server-side, on hello/unregister |

Each event: `{id, machineId, sessionId, kind, message, severity, ts, meta}`.

## Design

### 1. Protocol (`internal/protocol` + `packages/protocol`)
- New envelope type `event.report` (agent→server) and `event.append`
  (server→dashboard).
- `Event` payload struct: `machineId, sessionId, kind, message, severity
  (info|success|warn|error), ts, meta map[string]string`.
- Add `Events []Event` (recent tail) to the `Snapshot` so a freshly loaded
  dashboard shows history immediately.

### 2. Agent (`internal/agent/runtime.go` + new `events.go`)
- In `discover()`/`upsertSession()`, compare the new session against the stored
  previous one and emit `event.report` for each detected transition. The agent
  already holds `r.sessions[id]` (previous) — this is a pure diff, no extra
  polling.
- Session start/end already have hook points (the `existed` flag and the
  `removed` loop) — emit `session.started` / `session.ended` there.
- Helper `emitEvent(kind, session, message, severity, meta)` that stamps ts +
  machineId and sends over the WS.
- Debounce noisy fields (e.g. only emit `command.changed` when the command
  actually changes, not on every metric tick).

### 3. Server (`internal/server`)
- Handle `event.report`: persist via `store.RecordEvent`, then broadcast
  `event.append` to dashboards. (Mirrors the existing log/metric handlers.)
- New REST: `GET /api/events` (global, paginated, filter by `machine`,
  `session`, `kind`, `since`) and include recent events in the snapshot.
- Store: add `ListEvents(filter, limit)` query + retention pruning of old events
  (fold into the existing retention loop with an `EventHours` setting).

### 4. Dashboard
- Live store: accumulate `events` (capped ring) from snapshot + `event.append`,
  same pattern as logs.
- **New "Activity" sidebar page** — global reverse-chron timeline with
  kind icons, severity colors, machine/repo/branch context, relative time, and
  filters (machine / kind / search). Framer-motion entry animation.
- **Session page**: add an **Activity** tab (or a timeline card in Overview)
  showing only that session's events.
- Small `EventRow` component + `eventMeta` (icon + color per kind) in the ui/
  shared layer.

## Verification
- Go unit test: agent event diffing (start, status change, branch change, end)
  from a sequence of discovered snapshots.
- Server integration test: `event.report` → persisted + broadcast → appears in
  a dashboard connection and in `GET /api/events`.
- Live: start/stop a Claude session and change branches; confirm events stream
  into the Activity page in real time and survive a refresh (REST backfill).

## Scope / non-goals
- Pure polling-diff detection (no OS-level inotify) — consistent with the
  current architecture and cross-platform.
- Reuses the existing WS + retention + snapshot machinery; no new transport.
- This plan is **just the activity feed**. The broader SaaS items you mentioned
  earlier (Postgres DSN, multi-tenant orgs, file publish, Docker Hub packaging)
  are tracked separately — say the word and I'll fold them into a combined plan,
  but this delivers the "agent informs the control panel of all activity" piece
  end-to-end on its own.
