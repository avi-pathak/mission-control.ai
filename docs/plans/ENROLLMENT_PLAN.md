# Machine Enrollment + Releases — Implementation Plan

Goal: let a user click **Add Machine** in the dashboard, copy a one-line
`curl … | sh`, run it on any host, and watch that machine appear live — backed
by short-lived enrollment tokens (not the shared key) and real prebuilt agent
binaries.

Two workstreams:
- **A. Enrollment tokens + Add-Machine UX** (server, agent, dashboard)
- **B. Release binaries** (GoReleaser + CI) so `install.sh` has something to pull

---

## A. Enrollment tokens

### Model & storage (`internal/store`)
New GORM model `EnrollmentToken`:
```
Token      string  (PK, random 32-byte base62)
Label      string  (optional, user note e.g. "prod-box-1")
CreatedAt  time.Time
ExpiresAt  time.Time
UsedAt     *time.Time      // set on first successful enroll (single-use)
UsedByID   string          // machineId that consumed it
Revoked    bool
```
Store helpers: `CreateEnrollToken`, `GetEnrollToken`, `ConsumeEnrollToken`
(atomic: mark used + return the minted long-lived agent key), `ListEnrollTokens`,
`RevokeEnrollToken`, `PruneExpiredTokens` (fold into existing retention loop).

Also a new `AgentKey` model (the durable per-machine credential minted at
enrollment, so revoking one machine doesn't affect others):
```
Key        string (PK)
MachineID  string (index)
Label      string
CreatedAt  time.Time
Revoked    bool
```

### Auth layer (`internal/server`)
- Extend `hasKey` → `authAgent(key)` / `authDashboard(key)`. Both accept:
  - any key in `cfg.APIKeys` (backward-compatible shared key / dev mode), OR
  - a non-revoked `AgentKey` from the store (agents only).
- Keep the "no keys configured ⇒ open dev mode" behavior for `APIKeys`, but once
  any enrollment happens, minted keys are always checked.
- Load valid agent keys into an in-memory set on boot; refresh on mint/revoke.

### REST endpoints (`internal/server/handlers.go`, admin-authed by shared key)
- `POST /api/enroll-tokens {label?, ttlMinutes?}` → `{token, expiresAt, command}`
  where `command` is the ready-to-paste `curl -fsSL <serverBase>/install.sh | sh`
  with `MC_ENROLL_TOKEN` + `MC_SERVER_URL` env prefixed. Server infers its own
  public base URL from config (`publicUrl`) or the request Host.
- `GET /api/enroll-tokens` → list (status: active/used/expired/revoked).
- `DELETE /api/enroll-tokens/:token` → revoke.

### Enrollment exchange (public, no admin key)
- `POST /api/enroll {token, hostname, os, arch}` →
  validates token (exists, not expired/used/revoked), **mints an AgentKey**,
  consumes the token, returns `{agentId, agentKey, serverUrl}`.
- This is the only unauthenticated write endpoint; it's rate-limited and the
  token is single-use + short-TTL.

### Protocol (`internal/protocol` + `packages/protocol`)
- Add `EnrollRequest`/`EnrollResponse` types (REST DTOs, documented in API.md).
- No WS envelope change needed — agents still send `agent.hello`, now carrying
  the minted `AgentKey` as `apiKey`. `authAgent` accepts it.

### Agent (`internal/agent`, `internal/config`, `apps/agent`)
- New bootstrap path: if `apiKey` is empty but `MC_ENROLL_TOKEN` (or
  `enrollToken` in yaml) is set, the agent first calls `POST /api/enroll`,
  persists the returned `agentKey` + `agentId` back into `agent.yaml` (or a
  sibling `credentials.yaml`), then connects normally. On next start it uses the
  saved key and ignores the (now-consumed) token.
- Derive `serverUrl` http(s) base for the enroll POST from the ws URL.

---

## B. Release binaries (GoReleaser + CI)

- `.goreleaser.yaml`: build `apps/agent` (and `apps/server`) for
  linux/darwin/windows × amd64/arm64. Binaries named
  `mission-control-agent-<os>-<arch>` to match `install.sh`/`install.ps1`
  (already coded to that convention). CGO note: SQLite (server) needs CGO;
  the **agent has no CGO deps**, so agent cross-compiles cleanly. Server release
  can stay Docker-only initially; agent gets the matrix.
- `.github/workflows/release.yml`: on tag `v*`, run GoReleaser → publish GitHub
  Release with the agent binaries + checksums.
- Update `install.sh`/`install.ps1`: already point at
  `releases/latest/download/...`; add `MC_ENROLL_TOKEN` pass-through into the
  generated `agent.yaml`.

---

## C. Dashboard — Add Machine

- **MachinesPage**: add an **"Add Machine"** button → modal (`AddMachineDialog`).
  - Calls `POST /api/enroll-tokens` (via TanStack Query mutation), shows the
    generated one-line command with a **Copy** button, TTL countdown, and a note
    "run this on the machine you want to add."
  - Live feedback: since the store is already WS-live, when the new agent
    enrolls and connects, its card appears automatically — the dialog can show
    "Waiting for machine…" and auto-close on arrival.
- **Settings / Tokens** (optional, small): list active enrollment tokens with
  revoke buttons (`GET`/`DELETE /api/enroll-tokens`).
- New UI bits: a minimal `Dialog` primitive in `@mc/ui` (Radix-free, focus-trap
  + overlay) to avoid a new dep, plus `CopyButton`.

---

## D. Docs
- `docs/DEPLOYMENT.md`: host the control plane (compose + reverse proxy + TLS),
  set `publicUrl`, then "Add Machine" flow per-OS. Security notes: tokens are
  single-use/short-TTL, agent keys are per-machine + revocable, TLS for
  production, outbound-only agents (no inbound ports on hosts).
- Update `API.md` (new endpoints), `PROTOCOL.md` (auth note), `SCHEMA.md`
  (new tables), `ROADMAP.md` (mark enrollment done).

---

## Build order
1. Store: `EnrollmentToken` + `AgentKey` models, helpers, migration, prune.
2. Server auth: `authAgent`/`authDashboard`, in-memory key set, refresh.
3. Server REST: enroll-token CRUD + public `/api/enroll`; wire ws + hello to accept minted keys.
4. Protocol DTOs (Go + TS) + tests.
5. Agent bootstrap: enroll-on-first-run, persist credentials.
6. Dashboard: `Dialog`/`CopyButton` in `@mc/ui`, `AddMachineDialog`, MachinesPage button, tokens list.
7. GoReleaser + release workflow; update install scripts.
8. Docs + tests (unit for token lifecycle, integration for enroll→hello→appear).

## Verification
- Go: unit tests for token consume/expire/revoke; integration test:
  create token → `POST /api/enroll` → agent `hello` with minted key → machine
  appears in snapshot; revoked key rejected.
- Live smoke: generate token in UI, run agent with `MC_ENROLL_TOKEN`, confirm
  it self-enrolls, writes its key, and shows up — then restart agent to confirm
  it reuses the saved key.
- Keep existing shared-key/dev-mode path working (back-comp test).

## Backward compatibility
- Shared `apiKey` in `server.yaml` keeps working (admin + agents).
- Existing `agent.yaml` with a static `apiKey` is untouched.
- Enrollment is purely additive.
