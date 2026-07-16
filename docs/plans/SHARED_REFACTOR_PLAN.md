# Plan: Shared multi-provider refactor (behavior-preserving)

Goal: remove the three Claude-specific couplings so a new provider (Codex next)
is a self-contained package with zero edits to the agent core, protocol, or
server. **No behavior change** — pure restructuring, verified by the existing
test suite staying green.

## A. Extract the tmux layer into a shared package

The tmux functions in `internal/provider/claude/{sendkeys.go,tmux.go}` are
already provider-agnostic (they only touch `protocol` + `provider.LogLine`);
they're just misplaced.

**New package `internal/tmux`** (imports only stdlib — no `protocol`/`provider`,
so zero import-cycle risk) with the moved, exported functions:
- `SendKeysArgs`, `SendKeys`, `CaptureLoop`, `ResizeWindow`, `SessionExists`
  (renamed from `TmuxSessionExists`), plus the private helpers
  (`cursorPos`, `renderFrame`, `splitLines`, `bytesEqual`).
- `SessionForPID(ctx, pid) string` (moved from `tmuxSessionForPID`).
- `TailPane(ctx, sessionID string) <-chan string` — returns raw new pane lines
  as plain strings (no `provider.LogLine` dependency). The **claude** package
  wraps each string into `provider.LogLine` (SessionID/Stream/TS) — that
  adapter is the only place LogLine is constructed, keeping `internal/tmux`
  dependency-free.

**Claude package changes (thin adapters, same behavior):**
- `claude.go`: `tmuxSessionForPID` → `tmux.SessionForPID`; `tailTmuxPane` →
  wrap `tmux.TailPane`.
- Delete `sendkeys.go`/`tmux.go` bodies; keep `sendkeys_test.go` moved to
  `internal/tmux/tmux_test.go` (tests move with the code).
- The Claude-specific `SendKeysArgs` test stays valid (pure function).

**Runtime change (`internal/agent/runtime.go:383`):**
- `r.term.AttachTmux(..., claude.CaptureLoop, claude.SendKeys,
  claude.ResizeWindow, claude.TmuxSessionExists)` →
  `..., tmux.CaptureLoop, tmux.SendKeys, tmux.ResizeWindow, tmux.SessionExists`.
- Removes the agent runtime's dependency on the `claude` package for tmux.

## B. Generalize machineID injection (`apps/agent/main.go`)

Replace the Claude type-assertion:
```go
if cp, ok := p.(*claude.Provider); ok { cp.SetMachineID(rt.AgentID()) }
```
with a shared optional interface in `internal/provider`:
```go
type MachineAware interface { SetMachineID(string) }
```
Loop becomes:
```go
for _, p := range providers {
    if ma, ok := p.(provider.MachineAware); ok { ma.SetMachineID(rt.AgentID()) }
}
```
Claude's existing `SetMachineID(string)` already satisfies it — no change there.
Any future provider that needs the id just implements the method.

`claude.Register()` stays in main.go; future providers add their own
`Register()` call (one line each). Optionally add a small
`registerProviders()` helper listing all built-ins.

## C. Generalize `ClaudeVersion` → `Version` in the protocol

- `internal/protocol/messages.go:45`: `ClaudeVersion string` →
  `Version string \`json:"version"\``.
- `internal/provider/claude/claude.go:97`: `ClaudeVersion:` → `Version:`
  (keep `p.claudeVersion(ctx)` — it's the Claude impl of "get version").
- `packages/protocol/src/index.ts:94`: `claudeVersion` → `version`.
- `packages/protocol/src/index.test.ts:23`: update fixture key.
- Dashboard `apps/dashboard/src/pages/session/OverviewTab.tsx:34`: relabel the
  row to the provider — show `session.version` with a generic label, e.g.
  `<Row label="Version" value={session.version || '—'} />`, and keep the
  existing `Provider` row (line 18) so the tool name is still shown.

This is a breaking JSON field rename, but the server just passes it through and
the only consumers are our own dashboard + tests — all updated in this change.

## D. Optional token-usage loosening (defer unless trivial)

`TokenUsage.CacheRead/CacheCreation` already `omitempty`-friendly on the wire;
no struct change strictly required — a provider with no cache tokens just leaves
them 0. **Skip for now** (no code change needed); revisit when Codex lands if
its token shape differs.

## Build order
1. Create `internal/tmux` package; move functions + tests; adjust exports
   (`SessionExists`, `SessionForPID`, `TailPane`).
2. Rewire `claude` package (adapters) and `runtime.go` to use `internal/tmux`.
3. `provider.MachineAware` interface + `main.go` loop change.
4. `ClaudeVersion` → `Version` across Go protocol, claude provider, TS protocol,
   TS test, dashboard OverviewTab.
5. `go build ./... && go test ./...` (race) + `pnpm -r typecheck` +
   `pnpm --filter @mc/dashboard build`. All must stay green.
6. Rebuild agent binary, restart, live-verify sessions + tmux terminal still
   work identically. Commit.

## Verification (behavior-preserving proof)
- Full Go suite green (esp. `internal/provider/claude` tests that move to
  `internal/tmux`, and agent/server tests).
- TS typecheck + build green.
- Live: Overview still lists sessions; the tmux `work` session's interactive
  terminal + Approve/Deny still work (proves the moved tmux funcs are wired);
  the session detail shows a "Version" row.

## Scope guard
- **No new provider in this change** — refactor only. Codex comes next as a
  clean, self-contained package that needs zero edits to the touched core.
- No server/DB/API surface changes; the only wire change is the
  `claudeVersion` → `version` field rename (internal consumers only).
- tmux functions keep identical logic (cursor restore, flicker-free repaint,
  size re-assertion) — moved, not rewritten.
