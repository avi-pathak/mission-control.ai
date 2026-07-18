# Mission Control — Server

**Self-hosted control plane for monitoring and managing AI coding agent sessions** (Claude Code, Codex CLI, Gemini CLI) across all your machines — live over WebSocket.

A single container that hosts **everything**: the REST API, the WebSocket hub, the agent binary downloads, **and** the dashboard SPA. One port, no separate web server.

Think *Linear + Vercel, for your AI agents.*

---

## Quick start

Uses **SQLite** on a mounted volume (zero external dependencies). `DATABASE_URL`
defaults to `/data/mission-control.db` inside the image, so the `-v mc-data:/data`
volume persists it. Override `DATABASE_URL` to point at Postgres instead.

```bash
docker run -d --name mission-control \
  -p 8080:8080 \
  -v mc-data:/data \
  -e DATABASE_URL="/data/mission-control.db" \
  -e JWT_SECRET="change-me-to-a-long-random-value" \
  -e ADMIN_EMAIL="admin@example.com" \
  -e ADMIN_PASSWORD="change-me-please" \
  -e MC_PUBLIC_URL="http://localhost:8080" \
  avipathak/mission-control-server:latest
```

To use **Postgres**, swap the DB env for a DSN (and drop the volume):

```bash
  -e DATABASE_URL="postgres://user:pass@host:5432/mission_control?sslmode=disable"
```

Open **http://localhost:8080**, sign in with the admin credentials, and use
**Add Machine** to enroll an agent with a one-time token.

### Docker Compose (with Postgres)

```yaml
services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: mission
      POSTGRES_PASSWORD: mission
      POSTGRES_DB: mission_control
    volumes: [pg-data:/var/lib/postgresql/data]

  server:
    image: avipathak/mission-control-server:latest
    environment:
      DATABASE_URL: "postgres://mission:mission@postgres:5432/mission_control?sslmode=disable"
      JWT_SECRET: "change-me-in-production"
      ADMIN_EMAIL: "admin@example.com"
      ADMIN_PASSWORD: "change-me-please"
      MC_PUBLIC_URL: "http://localhost:8080"
    ports: ["8080:8080"]
    depends_on: [postgres]

volumes: { pg-data: {} }
```

---

## What it does

- **Live fleet view** — every AI coding session across local & remote machines, updating instantly over WebSocket.
- **Rich session pages** — logs (ANSI, search), read-only terminal, CPU/RAM metrics, git state.
- **Control plane** — stop / restart sessions remotely.
- **Multi-tenant** — workspaces with admin/member roles, admin-approved signups, per-workspace machine isolation (one machine = one workspace).
- **Self-contained** — serves the dashboard SPA and cross-compiled agent binaries itself; no external hosting, no CDN.

## Configuration (environment)

Everything is configured via environment variables. Every value has a sensible
fallback **except the database** — if you don't set `DATABASE_URL`, the image
falls back to SQLite on the `/data` volume (so it still works out of the box).

| Env | Fallback | Purpose |
|-----|----------|---------|
| `DATABASE_URL` | `/data/mission-control.db` (SQLite, image default) | Postgres DSN **or** a SQLite file path. |
| `JWT_SECRET` | insecure dev secret (logs a warning) | Signs dashboard sessions. **Set a strong, stable value in production.** |
| `ADMIN_EMAIL` | _(none)_ | Bootstrap platform-admin email (created on first run). |
| `ADMIN_PASSWORD` | _(none)_ | Bootstrap platform-admin password. |
| `MC_PUBLIC_URL` | derived from request host | Externally reachable base URL, used in enrollment commands + invite links. |
| `MC_LISTEN_ADDR` | `:8080` | Bind address. |
| `MC_LOG_LEVEL` | `info` | `debug` / `info` / `warn` / `error`. |
| `MC_CORS_ORIGINS` | `*` | Comma-separated allowed origins. |
| `MC_RETENTION_LOG_HOURS` | `72` | How long to keep log lines. |
| `MC_RETENTION_METRIC_HOURS` | `72` | How long to keep metric samples. |
| `MC_SMTP_HOST` | _(empty → email disabled)_ | SMTP host for invite emails. |
| `MC_SMTP_PORT` | `587` | SMTP port. |
| `MC_SMTP_USER` / `MC_SMTP_PASS` | _(none)_ | SMTP auth. |
| `MC_SMTP_FROM` | _(none)_ | From address (required to enable email). |
| `MC_STATIC_DIR` | `/app/web` (image default) | Dashboard SPA directory. |
| `MC_AGENT_BIN_DIR` | `/app/agents` (image default) | Prebuilt agent binaries served at `/download`. |

> If `ADMIN_EMAIL` / `ADMIN_PASSWORD` are unset, no admin is bootstrapped — you
> can register the first account through the UI instead. If `JWT_SECRET` is
> unset, a dev secret is used and sessions won't survive a restart — always set
> it in production.

### Full example (all env vars)

```bash
docker run -d --name mission-control \
  -p 8080:8080 \
  -v mc-data:/data \
  -e DATABASE_URL="/data/mission-control.db" \
  -e JWT_SECRET="change-me-to-a-long-random-value" \
  -e ADMIN_EMAIL="admin@example.com" \
  -e ADMIN_PASSWORD="change-me-please" \
  -e MC_PUBLIC_URL="https://mc.example.com" \
  -e MC_LISTEN_ADDR=":8080" \
  -e MC_LOG_LEVEL="info" \
  -e MC_CORS_ORIGINS="https://mc.example.com" \
  -e MC_RETENTION_LOG_HOURS="72" \
  -e MC_RETENTION_METRIC_HOURS="72" \
  -e MC_SMTP_HOST="smtp.example.com" \
  -e MC_SMTP_PORT="587" \
  -e MC_SMTP_USER="apikey" \
  -e MC_SMTP_PASS="secret" \
  -e MC_SMTP_FROM="Mission Control <no-reply@example.com>" \
  avipathak/mission-control-server:latest
```

Data is stored in the `/data` volume (SQLite) or your Postgres.

## Tags & platforms

- Tags: semver (e.g. `0.1.1`) and `latest`.
- Platforms: `linux/amd64`, `linux/arm64`.

## Enrolling machines

From the dashboard, **Add Machine** generates a one-time install command:

```bash
curl -fsSL https://<your-server>/install.sh \
  | MC_SERVER_URL="https://<your-server>" MC_ENROLL_TOKEN="<token>" sh
```

The agent downloads itself from the server, self-enrolls, and connects. See the
companion image **`avipathak/mission-control-agent`** for containerized Linux hosts.

---

Source, docs, and issues: **https://github.com/avi-pathak/mission-control.ai**
