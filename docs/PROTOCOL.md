# Wire Protocol

Version: **1**. Transport: WebSocket at `/ws`. Every message is a JSON
**envelope**. The Go definition (`internal/protocol`) and the TypeScript mirror
(`packages/protocol`) are the two sources of truth and must stay in sync.

The envelope is deliberately flat and behaviour-free so it can be re-expressed
as protobuf/gRPC without semantic change.

## Envelope

```json
{
  "v": 1,
  "type": "session.upsert",
  "ts": 1736899200000,
  "id": "optional-correlation-id",
  "payload": { }
}
```

| Field | Type | Notes |
|-------|------|-------|
| `v` | int | Protocol version. |
| `type` | string | Message type (see below). |
| `ts` | int64 | Unix milliseconds. |
| `id` | string? | Optional correlation id (commands). |
| `payload` | object | Type-specific body. |

## Connection & auth

- Agents connect to `/ws?role=agent` and MUST send `agent.hello` as the first
  message. The server validates `apiKey` and closes on failure.
- Dashboards connect to `/ws?role=dashboard&key=<API_KEY>`; the key is validated
  before the upgrade. On success the server immediately sends a `snapshot`.
- If no API keys are configured, the server runs open (development only).

## Messages

### Agent → Server

| Type | Payload | Purpose |
|------|---------|---------|
| `agent.hello` | `{apiKey, agentId, hostname, os, arch, cpuCores, totalMem, agentVersion}` | Handshake + identity. |
| `agent.heartbeat` | `{agentId, cpuPct, memUsedBytes, load}` | Host metrics. |
| `session.upsert` | `{session}` | New/updated session. |
| `session.removed` | `{sessionId}` | Session ended/vanished. |
| `log.append` | `{sessionId, seq, stream, line, ts}` | One log line (may contain ANSI). |
| `metric.sample` | `{sessionId, cpuPct, memBytes, ts}` | Per-session sample. |
| `command.ack` | `{requestId}` | Command received. |
| `command.result` | `{requestId, sessionId, ok, error}` | Command outcome. |

### Server → Agent

| Type | Payload | Purpose |
|------|---------|---------|
| `command` | `{requestId, sessionId, action}` | `action` ∈ `stop`, `restart`. |

### Server → Dashboard

| Type | Payload | Purpose |
|------|---------|---------|
| `snapshot` | `{machines[], sessions[]}` | Full state on connect. |
| `machine.upsert` | `{machine}` | Machine added/updated. |
| `session.upsert` | `{session}` | Session added/updated. |
| `session.removed` | `{sessionId}` | Session removed. |
| `metric.sample` | `{sessionId, cpuPct, memBytes, ts}` | Live metric. |
| `log.append` | `{sessionId, seq, stream, line, ts}` | Live log line. |
| `agent.status` | `{machineId, online}` | Agent connectivity. |

### Either

| Type | Payload |
|------|---------|
| `error` | `{code, message}` |

## Core objects

### Session
```
id, machineId, provider, status, repo, branch, cwd, pid,
currentCommand, claudeVersion, tmuxSession?, cpuPct, memBytes,
startedAt, lastActivityAt, git?
```
`status` ∈ `running | waiting_approval | idle | finished | error`.

### GitInfo
```
repo, remoteUrl, branch, dirty, modifiedFiles[], ahead, behind, recentCommits[]
```

### Machine
```
id, hostname, os, arch, cpuCores, totalMem, agentVersion,
online, cpuPct, memUsedBytes, load, lastSeenAt
```

## Liveness

The hub pings every ~54s and expects a pong within 60s. Agents reconnect with
exponential backoff (1s → 30s). On agent disconnect the server marks the machine
offline and emits `session.removed` for its live sessions.
