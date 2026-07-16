# Mission Control as SaaS

Mission Control runs as a multi-tenant, self-hostable SaaS. This document covers
tenancy, authentication, storage, the file-publish feature, environment
configuration, and deployment.

## Tenancy model

- **Org** — a tenant. Every machine, session, log line, metric, event, enrollment
  token, agent key, and published file is scoped to an `org_id`.
- **User** — a dashboard account (email + password) belonging to one org, with a
  role: `owner`, `admin`, or `member`.
- **Invite** — owners/admins invite teammates by email; the invite link lets
  them set a password and join the org.

Isolation is enforced at every layer: the in-memory state manager is partitioned
per org, WebSocket broadcasts are org-scoped, and every store query filters by
`org_id`. A user in org A can never see org B's data over REST or WebSocket.

## Authentication

### Dashboard users (JWT)
- `POST /api/auth/register` — creates a new org + owner (self-serve signup).
- `POST /api/auth/login` — returns a JWT.
- `POST /api/auth/accept-invite` — creates a user from an invite token.
- The dashboard stores the JWT in `localStorage` and sends it as
  `Authorization: Bearer <jwt>` on REST and `?token=<jwt>` on the WebSocket.
- JWTs are HS256, signed with `JWT_SECRET`, 7-day expiry.

### Agents (enrollment + agent keys)
Unchanged mechanism, now org-bound:
1. An authenticated user mints an **enrollment token** (carries their org).
2. The agent exchanges it at `POST /api/enroll` for a durable **agent key**,
   which inherits the token's org.
3. The agent authenticates its WebSocket (`agent.hello`) and file uploads
   (`X-API-Key`) with that key; all its data lands in the right tenant.

### Bootstrap admin
On first run, if `ADMIN_EMAIL` and `ADMIN_PASSWORD` are set and the database has
no users, the server creates a "Default" org and an owner. Any pre-tenancy rows
(from a single-tenant SQLite upgrade) are backfilled into that org.

## Database

Selected by `DATABASE_URL`:
- `postgres://user:pass@host:5432/db?sslmode=disable` → **Postgres** (production).
- Anything else → treated as a **SQLite** file path (dev / fallback).

Schema is auto-migrated on boot for both. See [SCHEMA.md](SCHEMA.md).

## File publish

Agents upload artifacts (logs, build outputs, etc.) that appear per-session in
the dashboard's **Files** tab.

```bash
mission-control-agent publish ./build/report.html --session <sessionId>
```

- The agent POSTs the file to `/api/publish` authenticated with its agent key.
- Files are stored as blobs in the database, scoped to the agent's org
  (32 MiB limit per file).
- Each publish emits a `file.published` activity event.
- Download via the dashboard (`GET /api/files/:id`, org-scoped).

## Environment configuration

### Server
| Var | Purpose |
|-----|---------|
| `DATABASE_URL` | Postgres DSN or SQLite path. |
| `JWT_SECRET` | Signing secret for dashboard tokens (**set in prod**). |
| `ADMIN_EMAIL` / `ADMIN_PASSWORD` | Bootstrap owner (first run only). |
| `MC_LISTEN_ADDR` | Listen address (default `:8080`). |
| `MC_PUBLIC_URL` | External base URL (enrollment commands, invite links). |
| `MC_LOG_LEVEL` | `debug`/`info`/`warn`/`error`. |

### Agent
| Var | Purpose |
|-----|---------|
| `MC_SERVER_URL` | Server ws/http URL. |
| `MC_ENROLL_TOKEN` | One-time enrollment token (first run). |
| `MC_API_KEY` | Durable agent key (after enrollment; usually auto-saved). |
| `MC_AGENT_ID` | Stable agent id override. |

## Deployment (Docker Hub)

Two multi-arch images are published on each `v*` tag by `.github/workflows/docker.yml`:
- `<dockerhub-user>/mission-control-server`
- `avipathak/mission-control-agent`

Set repository secrets `DOCKERHUB_USERNAME` and `DOCKERHUB_TOKEN`.

### Compose (single server + Postgres)

```bash
cd deploy
# edit docker-compose.yml: set JWT_SECRET, ADMIN_EMAIL, ADMIN_PASSWORD
docker compose up -d
```

- Postgres with a persistent volume.
- **One server** container serves the REST API, WebSocket, **and** the dashboard
  SPA on port `:8080` (the Go binary hosts the built SPA with history fallback;
  no separate web server). Set `MC_STATIC_DIR` to the SPA directory — the image
  bakes it in at `/app/web` by default.

Open http://localhost:8080, sign in with your `ADMIN_EMAIL` / `ADMIN_PASSWORD`,
then **Machines → Add Machine** to enroll agents.

### Running an agent (any host)

The dashboard's **Add Machine** dialog generates a ready-to-paste command for
each method below (with a one-time enrollment token baked in). All methods need
two env vars: `MC_SERVER_URL` (this server) and `MC_ENROLL_TOKEN` (one-time).

Agents are served **by the server itself** — no GitHub releases required. The
Docker image cross-compiles the agent for linux/darwin/windows (amd64+arm64) and
serves them at `/download/<name>`; `MC_AGENT_BIN_DIR` controls the directory.

**Install script (Linux/macOS):**
```bash
curl -fsSL https://mc.example.com/install.sh \
  | MC_SERVER_URL="https://mc.example.com" MC_ENROLL_TOKEN="<token>" sh
```

**Windows (PowerShell):**
```powershell
$env:MC_SERVER_URL="https://mc.example.com"; $env:MC_ENROLL_TOKEN="<token>"
irm https://mc.example.com/install.ps1 | iex
```

**Docker:**
```bash
docker run -d --name mission-control-agent \
  -e MC_SERVER_URL="https://mc.example.com" \
  -e MC_ENROLL_TOKEN="<token>" \
  avipathak/mission-control-agent
```

**Binary:** download from `https://mc.example.com/download/mission-control-agent-<os>-<arch>`
(e.g. `-darwin-arm64`, `-linux-amd64`, `-windows-amd64.exe`), then:
```bash
chmod +x mission-control-agent-*
MC_SERVER_URL="https://mc.example.com" MC_ENROLL_TOKEN="<token>" ./mission-control-agent-*
```

On first run the agent exchanges the token for a durable key, saves it to
`~/.mission-control/agent.yaml`, and reconnects with that key on restart (the
token is single-use).

## Breaking change from single-tenant

Dashboards now require **user login** (JWT) instead of a shared API key. Agents
still use enrollment/agent keys. Existing single-tenant SQLite databases are
migrated automatically: legacy rows are backfilled into the bootstrap org on
first run with `ADMIN_EMAIL`/`ADMIN_PASSWORD` set.
