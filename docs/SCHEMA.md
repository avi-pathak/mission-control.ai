# Database Schema

SQLite via GORM (`internal/store`). Tables are auto-migrated on server start.
The database holds **history and persistence**; live fleet state is kept in
memory by the state manager and rebuilt from agents on reconnect.

## ERD

```
machines 1───∞ sessions 1───∞ log_lines
                     │
                     └────────∞ metric_samples

events (audit) references machine_id / session_id (loose)
```

## `machines`
| Column | Type | Notes |
|--------|------|-------|
| id | text | PK (stable agent id). |
| hostname | text | |
| os / arch | text | |
| cpu_cores | int | |
| total_mem | int | bytes |
| agent_version | text | |
| last_seen_at | datetime | |
| created_at / updated_at | datetime | |

## `sessions`
| Column | Type | Notes |
|--------|------|-------|
| id | text | PK (`<machine>-<pid>`). |
| machine_id | text | index |
| provider | text | e.g. `claude-code` |
| status | text | index |
| repo | text | index |
| branch, cwd, current_command, claude_version, tmux_session | text | |
| pid | int | |
| started_at, last_activity_at | datetime | |
| ended_at | datetime? | set on finished/error |
| created_at / updated_at | datetime | |

## `log_lines`
| Column | Type | Notes |
|--------|------|-------|
| id | int | PK autoincrement |
| session_id | text | composite index (session_id, seq) |
| seq | int | per-session ordering / cursor |
| stream | text | stdout / stderr / system |
| line | text | may contain ANSI |
| ts | datetime | |

Pruned by the retention loop (`retention.logHours`).

## `metric_samples`
| Column | Type | Notes |
|--------|------|-------|
| id | int | PK autoincrement |
| session_id | text | composite index (session_id, ts) |
| cpu_pct | real | |
| mem_bytes | int | |
| ts | datetime | |

Pruned by the retention loop (`retention.metricHours`).

## `events`
| Column | Type | Notes |
|--------|------|-------|
| id | int | PK autoincrement |
| machine_id | text | index |
| session_id | text | index |
| kind | text | |
| message | text | |
| created_at | datetime | |

## `enrollment_tokens`
Single-use, short-TTL credentials for adding a machine.

| Column | Type | Notes |
|--------|------|-------|
| token | text | PK (random base62) |
| label | text | user note |
| created_at | datetime | |
| expires_at | datetime | TTL cutoff |
| used_at | datetime? | set on consume (single-use) |
| used_by_id | text | machine that consumed it |
| revoked | bool | |

Unused expired tokens are pruned hourly; used tokens are kept for audit.

## `agent_keys`
Durable, per-machine credentials minted at enrollment.

| Column | Type | Notes |
|--------|------|-------|
| key | text | PK (random base62) |
| machine_id | text | index |
| label | text | |
| created_at | datetime | |
| revoked | bool | revoke one machine without affecting others |

The server keeps a non-revoked key set in memory (refreshed on mint/revoke) for
fast agent authentication.
