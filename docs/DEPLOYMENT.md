# Deployment

Mission Control has two deployable roles:

- **Control plane** (server + dashboard) — runs once, centrally.
- **Agent** — runs on every machine you want to monitor.

Agents connect **outbound** to the server over WebSocket, so monitored machines
need no inbound ports — only network access to the server.

---

## 1. Deploy the control plane

### Docker Compose (recommended)

```bash
cd deploy
docker compose up -d
```

This starts:
- **postgres** (data store)
- **server** on `:8080` — a **single service** that hosts the REST API
  (`/api/v1`), the WebSocket (`/ws`), the agent binary downloads (`/download`),
  **and the dashboard SPA** itself. No separate web container.

Open **http://localhost:8080** and sign in with `ADMIN_EMAIL` / `ADMIN_PASSWORD`
from `docker-compose.yml`.

The compose file uses the published image `avipathak/mission-control-server:latest`.
To build from source instead, uncomment the `build:` block in `docker-compose.yml`.

### Configuration

Set these on the server (env or `server.yaml`). Env wins over YAML. Every value
has a fallback **except `DATABASE_URL`**, which defaults to SQLite on the
`/data` volume in the Docker image (so the container still runs with zero config).

| Setting | Env | Fallback | Purpose |
|---------|-----|----------|---------|
| `databaseUrl` | `DATABASE_URL` | `/data/mission-control.db` (image) | Postgres DSN, or a SQLite file path. |
| `jwtSecret` | `JWT_SECRET` | insecure dev secret (warns) | Signs dashboard sessions. **Required in production.** |
| `adminEmail` / `adminPassword` | `ADMIN_EMAIL` / `ADMIN_PASSWORD` | none (no admin bootstrapped) | Bootstrap platform admin (first run). |
| `publicUrl` | `MC_PUBLIC_URL` | derived from request | Externally reachable base URL for enrollment commands + invite links. |
| `listenAddr` | `MC_LISTEN_ADDR` | `:8080` | Bind address. |
| `logLevel` | `MC_LOG_LEVEL` | `info` | Log verbosity. |
| `corsOrigins` | `MC_CORS_ORIGINS` | `*` | Comma-separated allowed origins. |
| `retention.logHours` | `MC_RETENTION_LOG_HOURS` | `72` | Log retention. |
| `retention.metricHours` | `MC_RETENTION_METRIC_HOURS` | `72` | Metric retention. |
| `staticDir` | `MC_STATIC_DIR` | `/app/web` (image) | Dashboard SPA dir. |
| `agentBinDir` | `MC_AGENT_BIN_DIR` | `/app/agents` (image) | Agent binaries served at `/download`. |
| `smtp.*` | `MC_SMTP_HOST` / `_PORT` / `_USER` / `_PASS` / `_FROM` | disabled | Optional invite emails; if unset, links are shown in the UI. |
| `webPush.*` | `MC_VAPID_PUBLIC_KEY` / `_PRIVATE_KEY` / `MC_VAPID_SUBJECT` | disabled | Web Push (VAPID) keys for blocked-session notifications. Generate with `make vapid`. |
| `blockedNotifySeconds` | `MC_BLOCKED_NOTIFY_SECONDS` | `30` | Notify when a session waits for approval this long (0 = off). |

### TLS / production

Terminate TLS at a reverse proxy (Caddy, nginx, Traefik) in front of the
server, or set `tls.enabled` with `certFile`/`keyFile`. When behind a proxy,
ensure it forwards `X-Forwarded-Proto` and upgrades WebSocket connections
(`Upgrade`/`Connection` headers). Set `publicUrl` to the `https://` address.

Because the server hosts everything on one port, the proxy just forwards all
traffic to it:

```
mc.example.com {
    reverse_proxy server:8080
}
```

---

## 2. Add a machine (enrollment)

The polished flow — no manual key copying:

1. In the dashboard, go to **Machines → Add Machine**.
2. A one-time command is generated (valid 30 min). Copy it.
3. Run it on the machine you want to monitor:

   ```bash
   curl -fsSL https://mc.example.com/install.sh \
     | MC_SERVER_URL="https://mc.example.com" MC_ENROLL_TOKEN="<token>" sh
   ```

4. The installer downloads the agent, writes `~/.mission-control/agent.yaml`,
   and the agent **self-enrolls**: it exchanges the one-time token for a
   durable, per-machine key, saves it, and connects. The machine appears in the
   dashboard live — the Add Machine dialog auto-confirms when it arrives.

### How enrollment works

- **Enrollment tokens** are single-use and short-TTL. Consuming one mints a
  durable **agent key** bound to that machine.
- The agent persists its key to `agent.yaml` and clears the token, so restarts
  reuse the key without re-enrolling.
- Revoking one machine's key (or an unused token) never affects other machines.
- `POST /api/v1/enroll` is the only unauthenticated write endpoint; it is
  single-use and token-gated.
- **One machine = one workspace.** A machine's stable host id binds it to the
  workspace it first enrolled in. Enrolling the same machine with a token from a
  *different* workspace is rejected (`409 machine_already_registered`); a
  platform admin can move it via **Admin → Machines → reassign**.
- **Multiple servers, same machine.** A machine can run agents for several
  servers at once (e.g. a dev and a prod control plane). The installer writes a
  **per-server** config (`~/.mission-control/agent-<serverhost>.yaml`) and the
  agent derives a **per-server identity**, so each server sees the machine
  independently. The install guard only blocks a duplicate agent for the *same*
  server, not a different one.

### Windows

```powershell
$env:MC_SERVER_URL="https://mc.example.com"; $env:MC_ENROLL_TOKEN="<token>"
irm https://mc.example.com/deploy/install.ps1 | iex
```

### Run the agent as a service (optional)

For auto-start on boot, wrap the agent in your init system:

- **systemd** (Linux): create `/etc/systemd/system/mission-control-agent.service`
  running `mission-control-agent --config /root/.mission-control/agent.yaml`,
  then `systemctl enable --now mission-control-agent`.
- **launchd** (macOS): a `LaunchAgent` plist invoking the same command.
- **Windows**: register with `nssm` or a scheduled task at logon.

---

## 3. Manual / air-gapped enrollment

If you prefer not to use tokens, set a shared `apiKey` in both `server.yaml`
(`apiKeys`) and each `agent.yaml` (`apiKey`). This bypasses enrollment entirely
and remains fully supported.

---

## 4. Building from source

```bash
# Server (needs CGO for SQLite)
CGO_ENABLED=1 go build -o mission-control-server ./apps/server

# Agent (no CGO)
go build -o mission-control-agent ./apps/agent

# Dashboard
pnpm --filter @mc/dashboard build   # outputs apps/dashboard/dist
```

Prebuilt agent binaries for Linux/macOS/Windows are served directly by the
control plane at `/download/mission-control-agent-<os>-<arch>` — the install
scripts use these, so no GitHub Releases or external hosting is required.

---

## 5. Blocked-session notifications (PWA / Web Push)

The dashboard is an installable PWA. When a session stays **blocked**
(`waiting_approval`) longer than `MC_BLOCKED_NOTIFY_SECONDS` (default 30), the
server sends a **Web Push** notification to every subscribed device in the
workspace — **even when the dashboard tab is closed**.

Setup:

1. **Generate VAPID keys** on the server host:
   ```bash
   make vapid          # or: go run ./cmd/vapidgen
   ```
2. Set the printed values as env (or in `server.yaml` under `webPush`):
   `MC_VAPID_PUBLIC_KEY`, `MC_VAPID_PRIVATE_KEY`, `MC_VAPID_SUBJECT`.
   Push is disabled if the keys are empty.
3. In the dashboard: **Settings → Notifications → Enable notifications**, grant
   the browser permission, and (optionally) **Send test**. Install the app
   ("Add to Home Screen" / install icon) for the best mobile experience.

Notes:
- Requires **HTTPS** in production (service workers only run on secure origins;
  `localhost` is exempt for local testing).
- Each session is notified **once per blocked episode**; it re-arms if the
  session unblocks and blocks again.
- Dead subscriptions (uninstalled/expired) are pruned automatically.

---

## 6. Published Docker images

Images are on Docker Hub:

- **`avipathak/mission-control-server`** — API + WebSocket + dashboard SPA +
  agent downloads (single container). Multi-arch: `linux/amd64`, `linux/arm64`.
- **`avipathak/mission-control-agent`** — the host agent (for Linux hosts; on
  macOS/Windows run the native install script instead, since a container can't
  see host processes).

Tags: a semver tag (e.g. `0.1.1`) plus `latest`.

### Publishing a new version

CI builds and pushes on any `v*` git tag (`.github/workflows/docker.yml`).
To publish manually:

```bash
# 1. Bump the version constant in internal/agent/runtime.go
# 2. Multi-arch build + push both images
docker buildx build --platform linux/amd64,linux/arm64 \
  -f deploy/Dockerfile.server \
  -t avipathak/mission-control-server:<version> \
  -t avipathak/mission-control-server:latest --push .

docker buildx build --platform linux/amd64,linux/arm64 \
  -f deploy/Dockerfile.agent \
  -t avipathak/mission-control-agent:<version> \
  -t avipathak/mission-control-agent:latest --push .
```
