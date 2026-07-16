# Mission Control — SaaS Build Plan

Turn Mission Control into a deployable multi-tenant SaaS. Decisions locked:

- **DB**: Postgres via `DATABASE_URL` (GORM postgres driver); **SQLite fallback**
  when unset. Driver auto-selected by scheme.
- **Tenancy**: **Full multi-tenant** — orgs + users (email/password, JWT),
  invites, and every machine/session/log/event/file scoped to an `org_id`.
  First-run bootstrap admin from env.
- **File publish**: **explicit CLI** — `mission-control-agent publish <path>`.
  Stored **in the database (blob)**, scoped to org, listed + downloadable in the
  dashboard per session/machine.
- **Packaging**: **two Docker Hub images** (`server`, `agent`), multi-arch,
  pushed by CI on tags. All config via **env**.

---

## A. Database layer (`internal/store`)

### Driver selection
- `Open(cfg)` reads `DATABASE_URL`. If it starts with `postgres://`/`postgresql://`
  → GORM postgres driver; else treat as SQLite path (fallback / dev).
- Add `gorm.io/driver/postgres`. Keep sqlite. Auto-migrate on boot for both.
- Server Dockerfile: postgres needs **no CGO**; SQLite still needs CGO — keep
  CGO on for the server image (already the case).

### New tenancy models
- `Org { ID (uuid, pk), Name, Slug (uniq), CreatedAt }`
- `User { ID, OrgID (idx), Email (uniq), PasswordHash, Role (owner|admin|member), CreatedAt }`
- `Invite { Token (pk), OrgID, Email, Role, InvitedBy, ExpiresAt, AcceptedAt }`
- `PublishedFile { ID, OrgID (idx), MachineID, SessionID, Name, Size, ContentType, Data []byte (blob), SHA256, CreatedAt }`

### Add `OrgID` to existing models
`Machine, Session, LogLine, MetricSample, Event, EnrollmentToken, AgentKey`
each gain `OrgID string (index)`. Every query gains an org filter. Enrollment
tokens are minted **within an org**, so the agent key it mints carries the
org — that's how an agent's data lands in the right tenant.

### Store helpers
- Org/User CRUD, `UserByEmail`, invite create/accept, `BootstrapAdmin`.
- `SaveFile`, `ListFiles(org, filter)`, `GetFile(org, id)` (blob fetch).
- All existing helpers get an `orgID` parameter (or a scoped `*Store` view —
  see below).

### Scoping strategy
Introduce `store.Scope(orgID)` returning a lightweight wrapper whose methods
auto-inject `org_id`. Prevents "forgot the filter" bugs and keeps handlers
clean. The raw `*Store` stays for auth/bootstrap (cross-org) operations.

---

## B. Auth (`internal/server` + `internal/auth`)

### User auth (dashboard)
- New `internal/auth`: bcrypt hashing, JWT issue/verify (HS256, `JWT_SECRET`
  env), claims `{userId, orgId, role, exp}`.
- Endpoints (public): `POST /api/auth/register` (creates org + owner on first
  signup, or joins via invite), `POST /api/auth/login`, `POST /api/auth/accept-invite`.
- Middleware `requireUser` → validates JWT, puts `{orgId, userId, role}` in
  request context. Replaces the shared-API-key middleware for dashboard routes.
- Org-admin endpoints: `GET/POST /api/org/users`, `POST /api/org/invites`,
  `GET /api/org/invites`, `DELETE /api/org/invites/:token`.
- **Bootstrap**: on boot, if `ADMIN_EMAIL`/`ADMIN_PASSWORD` set and no users
  exist, create a default org + owner.

### Agent auth (unchanged mechanism, now org-bound)
- Enrollment tokens are created by an authenticated user → carry `org_id`.
- `POST /api/enroll` mints an org-scoped `AgentKey`; the WS `agent.hello` maps
  the key → `orgId`, stamped onto the hub client and all inbound data.
- Enrollment-token admin endpoints move under `requireUser` + org scope.

### Dashboard token
- Dashboard WS connects with the JWT (`?token=` or `Authorization`), server
  resolves `orgId` before upgrade.

---

## C. Org-scoped realtime (`internal/hub` + `internal/state`)

- `hub.Client` gains `OrgID`. `BroadcastDashboards`/`EmitDashboards` become
  **org-scoped**: `EmitToOrg(orgID, type, payload)` sends only to that org's
  dashboards. `SendToAgent` already targets one machine — fine, but verify the
  command's session belongs to the caller's org first.
- `state.Manager` becomes **per-org partitioned**: keyed maps
  `map[orgID]*orgState`. `Snapshot(orgID)`, `UpsertSession(orgID, …)`, etc.
  Agent messages carry the client's `OrgID`; server routes into the right
  partition and broadcasts to that org only.
- Server-generated events (agent connect/disconnect, command issued) include
  the org and go only to that org's dashboards.

---

## D. File publish

### Protocol
- REST (not WS — files can be large): `POST /api/publish` (agent-authed with its
  AgentKey via `X-API-Key`), multipart or raw body + headers
  `X-MC-Session`, `X-MC-Filename`. Server stores blob under the key's org.
- Dashboard: `GET /api/files?session=&machine=` (list, org-scoped),
  `GET /api/files/:id` (download, org-scoped, sets Content-Disposition).

### Agent CLI
- `apps/agent` grows a subcommand: `mission-control-agent publish <path>
  [--session <id>] [--config <path>]`. Reads serverURL+apiKey from config,
  POSTs the file to `/api/publish`. Emits a `file.published` **event** so it
  shows in the activity feed.

### Dashboard
- Session page **Files tab**: list published files (name, size, time) with
  download buttons. Empty state otherwise.

---

## E. Config via env (`internal/config`)

Server env (all overridable):
`DATABASE_URL, JWT_SECRET, ADMIN_EMAIL, ADMIN_PASSWORD, MC_LISTEN_ADDR,
MC_PUBLIC_URL, MC_CORS_ORIGINS, MC_LOG_LEVEL, MC_TLS_*, retention`.
Agent env: `MC_SERVER_URL, MC_API_KEY, MC_ENROLL_TOKEN, MC_AGENT_ID`.
Document every var. YAML stays supported but env wins.

---

## F. Dashboard auth UX

- **Login / Register pages** (unauthenticated route group). Store JWT in
  localStorage; attach to REST (`Authorization: Bearer`) and WS (`?token=`).
- **Auth store** (Zustand): token, current user/org, login/logout; 401 →
  redirect to login.
- **Org settings page**: members list, invite form (email + role), pending
  invites with revoke. Replaces the static Settings "API key" view.
- Router: guard app routes behind auth; redirect to `/login` when no token.

---

## G. Packaging & deploy (two images, env-config)

- `deploy/Dockerfile.server` (exists) — ensure env-only config, expose blob/DB.
- `deploy/Dockerfile.agent` — **new**: multi-stage, CGO-free agent binary.
- `deploy/docker-compose.yml` — add **Postgres** service + server + dashboard;
  server reads `DATABASE_URL=postgres://…`. Volumes for PG.
- `.github/workflows/docker.yml` — buildx multi-arch (amd64/arm64), push
  `${DOCKERHUB_USER}/mission-control-server` and `…-agent` on tags + `latest`.
  Uses `DOCKERHUB_USERNAME`/`DOCKERHUB_TOKEN` secrets.
- Keep the GoReleaser agent-binary release for non-Docker installs.

---

## H. Docs
- `docs/SAAS.md` — architecture of tenancy, auth flow, env reference, deploy.
- Update `DEPLOYMENT.md` (Postgres + compose), `API.md` (auth, org, files,
  publish), `SCHEMA.md` (org/user/invite/file tables + org_id columns),
  `README.md` (SaaS quickstart), `ROADMAP.md`.

---

## Build order
1. **Store**: postgres driver + `DATABASE_URL` selection; add org/user/invite/
   file models + `org_id` columns; scoped store view; migrations. Tests.
2. **Auth core** (`internal/auth`): bcrypt + JWT; unit tests.
3. **Server auth**: register/login/invite endpoints, `requireUser` middleware,
   bootstrap admin, org-admin endpoints. Enrollment moves under user+org.
4. **Org scoping**: thread `orgId` through hub (per-org broadcast), state
   (per-org partitions), all handlers + WS. Integration test: two orgs isolated.
5. **File publish**: store blob helpers, `/api/publish` + `/api/files`,
   agent `publish` subcommand, `file.published` event, Files tab.
6. **Dashboard auth**: login/register pages, auth store, route guard, org
   settings/invites, Files tab UI.
7. **Packaging**: agent Dockerfile, compose w/ Postgres, docker buildx CI.
8. **Docs** + full test pass (Go race + TS typecheck/build) + live smoke
   (two orgs, enroll agent in one, publish a file, confirm isolation).

## Verification
- Unit: JWT round-trip, password hashing, org-scoped store queries, driver
  selection.
- Integration: two-org isolation (org A never sees org B's machines/sessions/
  events/files over REST or WS); enroll→org binding; publish→download.
- Live smoke against **Postgres** (compose) and SQLite (dev): register org,
  invite a user, enroll an agent, publish a file, watch activity — all scoped.

## Compatibility / migration notes
- Existing single-tenant SQLite DBs: on migrate, rows without `org_id` are
  assigned to the bootstrap org (a one-time backfill) so nothing is orphaned.
- Shared-API-key dev mode is **removed for dashboards** (now user-auth); agents
  still use enrollment/agent keys. Document the breaking change.

This is a large multi-phase build; each phase ends compiling + tested before the
next. I'll keep the running dev stack green between phases.
