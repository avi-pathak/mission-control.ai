# Architecture

Mission Control is a three-tier control plane for observing and managing AI
coding agent sessions.

```
        ┌─────────────────────────── Machine A ───────────────────────────┐
        │  claude ▸ repo/branch   tmux   git   metrics                     │
        │                     ▲                                            │
        │            discover │                                           │
        │              ┌──────┴───────┐                                    │
        │              │    Agent     │  Go daemon                         │
        │              └──────┬───────┘                                    │
        └─────────────────────┼────────────────────────────────────────────┘
                              │ WebSocket (JSON envelopes, API-key auth)
                              ▼
                   ┌────────────────────┐
                   │       Server       │  Go / Chi
                   │  ┌──────────────┐  │
                   │  │  WS Hub      │  │  fan-out agents ↔ dashboards
                   │  ├──────────────┤  │
                   │  │ State (RAM)  │  │  live index of machines+sessions
                   │  ├──────────────┤  │
                   │  │ Store (GORM) │  │  SQLite history: logs, metrics, events
                   │  └──────────────┘  │
                   │   REST API + /ws   │
                   └─────────┬──────────┘
                             │ WebSocket snapshot + diffs / REST
                             ▼
                   ┌────────────────────┐
                   │     Dashboard      │  React SPA
                   │  Zustand live store│
                   └────────────────────┘
```

## Components

### Agent (`apps/agent`, `internal/agent`, `internal/provider`)
- Runs on every machine. Stateless; the server is the source of truth.
- Discovers sessions via `AgentProvider` implementations (Claude Code first).
- Streams `session.upsert`, `log.append`, `metric.sample`, and `agent.heartbeat`.
- Executes `command` (stop/restart) messages from the server.
- Reconnects automatically with exponential backoff.

### Server (`apps/server`, `internal/server`, `internal/hub`, `internal/state`, `internal/store`)
- **Hub** — WebSocket registry. Routes agent messages, fans out to dashboards,
  and delivers commands to the owning agent. Drops slow consumers.
- **State manager** — in-memory, concurrency-safe live index. Serves dashboard
  snapshots and produces diffs. Rebuilt from agents on reconnect.
- **Store** — GORM + SQLite. Persists machines, sessions, logs, metrics, events.
  A retention loop prunes old logs/metrics.
- **HTTP (Chi)** — REST API + `/ws` upgrade. API-key middleware; TLS-ready.

### Dashboard (`apps/dashboard`)
- React + TanStack Router/Query + Zustand + Tailwind + shadcn-style UI.
- A single WebSocket connection hydrates a normalized live store from a
  `snapshot`, then applies diffs. REST is used for history (logs/metrics) and
  mutations (stop/restart).

## Data flow

1. Agent connects to `/ws?role=agent`, sends `agent.hello` (API key + identity).
2. Server registers the machine, persists it, and broadcasts `machine.upsert`.
3. Agent periodically discovers sessions → `session.upsert`; server updates
   state + store and broadcasts to dashboards.
4. Dashboard connects to `/ws?role=dashboard`, receives a full `snapshot`, then
   live diffs.
5. Dashboard issues `POST /api/sessions/:id/stop` → server sends `command` to
   the owning agent → agent runs `Provider.Control` → `command.result`.

## Extensibility — providers

`internal/provider.AgentProvider` is the plugin seam. Each provider discovers
and controls one kind of tool. `ClaudeProvider` is the reference implementation;
Codex CLI, Gemini CLI, Aider, OpenHands, Roo Code and Continue can be added by
implementing the interface and calling `provider.Register`.

See [PROTOCOL.md](PROTOCOL.md), [API.md](API.md), and [SCHEMA.md](SCHEMA.md).
