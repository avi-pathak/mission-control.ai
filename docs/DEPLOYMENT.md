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
- **server** on `:8080` (REST + WebSocket, SQLite volume `mc-data`)
- **dashboard** (nginx) on `:3000`, proxying `/api` and `/ws` to the server

Open **http://localhost:3000**.

### Configuration

Set these on the server (env or `server.yaml`):

| Setting | Env | Purpose |
|---------|-----|---------|
| `listenAddr` | `MC_LISTEN_ADDR` | Bind address (default `:8080`). |
| `publicUrl` | `MC_PUBLIC_URL` | **Externally reachable base URL** (e.g. `https://mc.example.com`). Used to build the enrollment `curl` command. Set this in production. |
| `apiKeys` | `MC_API_KEY` | Admin/dashboard keys. Leave empty only for local dev. |
| `dbPath` | `MC_DB_PATH` | SQLite path. |

### TLS / production

Terminate TLS at a reverse proxy (Caddy, nginx, Traefik) in front of the
server, or set `tls.enabled` with `certFile`/`keyFile`. When behind a proxy,
ensure it forwards `X-Forwarded-Proto` and upgrades WebSocket connections
(`Upgrade`/`Connection` headers). Set `publicUrl` to the `https://` address.

Example Caddyfile:

```
mc.example.com {
    reverse_proxy /api/*  server:8080
    reverse_proxy /ws     server:8080
    reverse_proxy /install.sh server:8080
    reverse_proxy *       dashboard:80
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
- `POST /api/enroll` is the only unauthenticated write endpoint; it is
  single-use and token-gated.

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

Prebuilt agent binaries for Linux/macOS/Windows are published to GitHub
Releases on each `v*` tag via GoReleaser.
