# Mission Control — Agent

The **host agent** for [Mission Control](https://hub.docker.com/r/avipathak/mission-control-server) — discovers AI coding sessions (Claude Code, Codex CLI, Gemini CLI) on a machine and streams them to your control plane over an outbound WebSocket.

> ⚠️ **Linux hosts only.** A container can only see processes in its own PID
> namespace. On **Linux** you can share the host's process list with
> `--pid=host`. On **macOS/Windows**, Docker runs inside a Linux VM, so a
> container can never see the host's real processes — use the native install
> script instead (`curl -fsSL https://<server>/install.sh | ... sh`).

---

## Run (Linux host)

```bash
docker run -d --name mission-control-agent \
  --pid=host \
  -v "$HOME/.claude:/host/.claude:ro" \
  -e MC_SERVER_URL="https://<your-server>" \
  -e MC_ENROLL_TOKEN="<one-time-token>" \
  avipathak/mission-control-agent:latest
```

Get the `MC_ENROLL_TOKEN` from the dashboard: **Add Machine → Binary/Script**.
The agent exchanges it for a durable per-machine key on first run.

- `--pid=host` lets the agent see the host's agent processes.
- Mount `~/.claude` (and/or `~/.codex`, `~/.gemini`) read-only so it can read
  session transcripts for logs and token usage.

## Configuration (environment)

| Env | Purpose |
|-----|---------|
| `MC_SERVER_URL` | Control-plane URL (http/https or ws/wss). |
| `MC_ENROLL_TOKEN` | One-time enrollment token (first run). |
| `MC_API_KEY` | Durable agent key (alternative to enrolling; e.g. shared-key setups). |
| `MC_AGENT_ID` | Override the machine id (defaults to a stable host fingerprint). |

## Notes

- **One machine = one workspace.** The agent's stable host id binds it to the
  workspace it first enrolled in. Re-enrolling with a token from a *different*
  workspace is rejected; an admin can reassign it in the dashboard.
- Agents connect **outbound** only — monitored hosts need no inbound ports.

## Tags & platforms

- Tags: semver (e.g. `0.1.1`) and `latest`.
- Platforms: `linux/amd64`, `linux/arm64`.

---

Source, docs, and issues: **https://github.com/avi-pathak/mission-control.ai**
