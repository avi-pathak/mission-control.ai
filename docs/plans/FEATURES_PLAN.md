# Plan: Remove Machine · Dashboard Charts · Session Token Usage

Three features. Each is self-contained; I'll build and verify them in order.

## 1. Remove machine from the panel

**Server**
- `DELETE /api/machines/{id}` (user-auth, org-scoped). Deletes the machine and
  its dependent rows (sessions, logs, metrics, events, agent keys, published
  files) **for that org only**. Refuses if the machine is currently online
  (avoid deleting a live agent) unless `?force=true`.
- Store: `DeleteMachine(orgID, machineID)` — a transaction removing the machine +
  cascade across tables (all already have `org_id` + `machine_id`/`session_id`).
- Also remove from the in-memory state manager and revoke its agent keys so a
  killed agent can't silently reappear; broadcast `machine.removed` to the org.
- Add `machine.removed` handling already exists in protocol (TypeMachineRemoved);
  wire the emit.

**Dashboard**
- Machines page: a "⋯" / trash action on each machine card → confirm dialog →
  `api.deleteMachine(id)`; on success remove from the live store. Offline
  machines delete directly; online ones show a "still online" warning.
- Live store: handle `machine.removed` (drop machine + its sessions).

## 2. Charts on the main dashboard (Overview)

Reuse the existing Recharts setup (already used in MetricsTab).

- **Sessions by status** — a small donut/bar (Running / Waiting / Finished /
  Error) from the live `sessions` the page already has.
- **Fleet activity over time** — a compact area chart of recent event volume
  (bucketed from the live `events` ring already in the store), or sessions
  started per hour.
- **Top repositories** — a horizontal bar of session counts per repo.
- All derived from data already in the store (no new endpoints). Placed above
  the session table as a row of chart cards, matching the existing card styling
  + framer-motion entrance.

## 3. Token usage per session

**Agent (ClaudeProvider)**
- While reading the transcript (already tailing `~/.claude/projects/.../*.jsonl`),
  parse the `message.usage` object on assistant lines and **accumulate** per
  session: `inputTokens`, `outputTokens`, `cacheReadTokens`,
  `cacheCreationTokens`, and a derived `totalTokens`.
- Surface these on the `Session` model, refreshed on each discovery/metric tick
  (cheap: read the newest usage or sum across the file, capped).
  - Simplest robust approach: on each poll, scan the session's transcript for
    the **latest** cumulative usage line (Claude reports cumulative
    cache-read/creation) plus **sum** input/output across turns. Store the
    computed totals on the session.

**Protocol** (Go + TS)
- Add to `Session`: `tokensInput`, `tokensOutput`, `tokensCacheRead`,
  `tokensCacheCreation`, `tokensTotal` (all int64, omitempty). Flows through the
  existing `session.upsert` path — no new message type, no server changes beyond
  passing the fields through (already generic).

**Server / store**
- `Session` store row gains the token columns (auto-migrated). `UpsertSession`
  persists them. No new endpoint.

**Dashboard**
- **Overview tab** of a session: a "Token usage" card — total + breakdown
  (input / output / cache-read / cache-creation) with `formatCompact` numbers.
- **Session table**: an optional "Tokens" column (compact total).
- **Metrics tab**: a token bar/area if time allows (nice-to-have; the card is
  the primary deliverable).
- **Overview dashboard**: a "Total tokens (fleet)" stat card summing live
  sessions — ties feature 3 into feature 2.

## Build order & verification
1. **Remove machine** — store delete + cascade, endpoint, state/broadcast,
   Machines-page UI. Test: delete offline machine → gone from list + DB; online
   machine guarded. Go unit test for cascade + org-scope.
2. **Token usage** — protocol fields, agent parsing (unit test on a transcript
   fixture), session card + table column. Live check: real sessions show token
   counts.
3. **Charts** — Overview chart cards from existing store data. Headless render
   check.

Each step: `go build/test`, `pnpm typecheck/build`, then a live check against
the running server. Rebuild + repush Docker images at the end (server bundles
agent binaries, so token parsing must be in the pushed agent).

## Scope guard
- No new heavy deps (Recharts already present).
- Token parsing is best-effort: if a transcript has no `usage`, fields are 0 —
  never breaks discovery.
- Remove-machine is org-scoped and cascade-safe; can't touch another tenant.
