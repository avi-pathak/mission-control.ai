# REST API

Base path: `/api`. All endpoints require authentication unless the server runs
with no configured keys (development mode).

## Authentication

Provide the API key one of three ways:
- `Authorization: Bearer <key>`
- `X-API-Key: <key>`
- `?key=<key>` query parameter (used by the dashboard WS)

Unauthorized requests receive `401` with `{"error": {"code, message}}`.

## Endpoints

### `GET /healthz`
Unauthenticated liveness probe. → `{"status":"ok","ts":<ms>}`

### `GET /api/machines`
List all known machines (live state). → `Machine[]`

### `GET /api/sessions`
List live sessions. Optional query filters: `status`, `machine`, `repo`.
→ `Session[]`

### `GET /api/sessions/:id`
Fetch one session. → `Session` | `404`

### `POST /api/sessions/:id/stop`
Send a stop command to the owning agent.
→ `202 {"requestId","action":"stop"}` | `404` | `503 agent_offline`

### `POST /api/sessions/:id/restart`
Send a restart command. Same response shape as stop.

### `GET /api/logs/:id`
Paginated historical logs. Query: `after` (seq cursor, default 0),
`limit` (default 500, max 2000).
→ `{"lines": LogLine[], "nextCursor": <seq>}`

### `GET /api/metrics/:id`
Historical metric samples. Query: `windowMinutes` (default 60, max 1440).
→ `MetricSample[]`

### `GET /api/events`
Activity feed. Optional filters: `machine`, `session`, `kind`; `limit`
(default 200, max 1000). Returns events newest-first.
→ `Event[]` where `Event = {id, machineId, sessionId?, kind, message, severity, ts, meta?}`.

Event kinds: `session.started`, `session.ended`, `status.changed`,
`branch.changed`, `commit.created`, `command.issued`, `agent.connected`,
`agent.disconnected`. Agents detect session transitions during polling and
report them; the server also generates connect/disconnect and command events.
Recent events are included in the WebSocket `snapshot` and streamed live via
`event.append`.

## Enrollment

### `POST /api/enroll-tokens` (admin)
Mint a single-use enrollment token. Body: `{label?, ttlMinutes?}` (default TTL
30 min).
→ `201 {"token", "expiresAt", "command"}` where `command` is the ready-to-paste
`curl … | sh` one-liner.

### `GET /api/enroll-tokens` (admin)
List tokens with derived `status` (`active|used|expired|revoked`).
→ `EnrollToken[]`

### `DELETE /api/enroll-tokens/:token` (admin)
Revoke a token. → `204`

### `POST /api/enroll` (public, single-use)
Exchange a valid token for a durable agent key. Body:
`{token, hostname, os, arch}`.
→ `200 {"agentId", "agentKey", "serverUrl"}` | `401 invalid_token`

This is the only unauthenticated write endpoint; it is token-gated and
single-use. See [DEPLOYMENT.md](DEPLOYMENT.md) for the full flow.

### `GET /install.sh` (public)
Serves the POSIX agent installer. Combined with the enrollment one-liner:
```
curl -fsSL <server>/install.sh | MC_SERVER_URL=<server> MC_ENROLL_TOKEN=<token> sh
```

## WebSocket

`GET /ws?role=agent` — agent channel (auth via `agent.hello`).
`GET /ws?role=dashboard&key=<key>` — dashboard channel (auth before upgrade).

See [PROTOCOL.md](PROTOCOL.md) for message shapes.

## Error format

```json
{ "error": { "code": "not_found", "message": "session not found" } }
```

| Code | Status |
|------|--------|
| `unauthorized` | 401 |
| `not_found` | 404 |
| `agent_offline` | 503 |
| `bad_role` | 400 |
| `db` / `encode` | 500 |
