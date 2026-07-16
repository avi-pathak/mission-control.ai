# Roadmap

## Status: MVP (v0.1)

Delivered:
- ✅ Monorepo (pnpm workspaces + single Go module).
- ✅ Versioned JSON protocol (Go + TS mirrors, zod validation).
- ✅ Server: Chi REST API, WebSocket hub, in-memory state, GORM/SQLite store,
  retention, API-key auth, TLS-ready, graceful shutdown.
- ✅ Agent: `AgentProvider` interface + registry, `ClaudeProvider` (process
  discovery, git, tmux, per-PID + host metrics), reconnecting WS client.
- ✅ Dashboard: live Zustand store, Overview (stat cards + filterable table),
  Machines, Sessions, Session detail (Overview/Logs/Terminal/Metrics/Git).
- ✅ Docker Compose, install scripts, CI (Go + TS lint/test/build).
- ✅ Unit + integration tests (agent→server→dashboard).

## v0.2 — Depth

- [ ] Persisted historical session browser (ended sessions, search by repo).
- [x] Dashboard log/metric hydration from REST on session open (backfill).
- [ ] Per-machine and per-project pages with aggregate metrics.
- [ ] Approval workflow surfacing (`waiting_approval`) with one-click actions.
- [x] **Machine enrollment**: dashboard "Add Machine" → one-time token →
  `curl … | sh` → agent self-enrolls with a durable per-machine key.
- [x] **Release binaries**: GoReleaser publishes agent binaries for
  Linux/macOS/Windows on each `v*` tag; `install.sh`/`install.ps1` fetch them.
- [x] **Activity feed**: agents report session/git/status events; live timeline.
- [x] **Multi-tenant SaaS**: orgs + users (email/password, JWT) + invites; all
  data scoped to `org_id`. See [SAAS.md](SAAS.md).
- [x] **Postgres**: `DATABASE_URL` selects Postgres (SQLite fallback).
- [x] **File publish**: `agent publish <path>` uploads artifacts (DB blobs),
  downloadable per-session.
- [x] **Docker Hub**: two multi-arch images (`server`, `agent`) via CI.
- [ ] Auth: SSO/OAuth, per-key scopes, token rotation.
- [ ] Agent service installers (systemd/launchd/Windows service).
- [ ] Object storage (S3) backend for published files at scale.

## v0.3 — More providers

Implement `AgentProvider` for:
- [ ] Codex CLI
- [ ] Gemini CLI
- [ ] Aider
- [ ] OpenHands
- [ ] Roo Code
- [ ] Continue

Each provider lives in `internal/provider/<name>` and calls `provider.Register`.

## v0.4 — Scale & transport

- [ ] Optional gRPC transport alongside WebSocket (protocol is already flat).
- [ ] Postgres store driver behind the GORM abstraction.
- [ ] Horizontal server scaling with a shared pub/sub (NATS/Redis) for hub fan-out.
- [ ] Interactive terminal (PTY passthrough) behind an explicit opt-in.

## Writing a provider

```go
type MyProvider struct{}

func (p *MyProvider) Name() string { return "my-tool" }
func (p *MyProvider) Discover(ctx context.Context) ([]protocol.Session, error) { ... }
func (p *MyProvider) Metrics(ctx context.Context, s protocol.Session) (protocol.MetricSample, error) { ... }
func (p *MyProvider) Logs(ctx context.Context, s protocol.Session) (<-chan provider.LogLine, error) { ... }
func (p *MyProvider) Control(ctx context.Context, s protocol.Session, a protocol.CommandAction) error { ... }

func init() {
    provider.Register("my-tool", func() (provider.AgentProvider, error) { return &MyProvider{}, nil })
}
```

Enable it in `agent.yaml` under `providers:`.
