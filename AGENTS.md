# Agent Instructions for session-agent

Guidelines for AI agents working in this repository.

> **Keep this file current.** Treat `AGENTS.md` as part of the change. When you
> add/rename/remove a package, change the build or test commands, alter an
> integrity invariant (§4), add a tray action or setting, or introduce a term
> worth defining, update the relevant section (and the **Vocabulary** in §3) in
> the *same* change. If a change makes something here wrong, fix it — a stale
> instruction is a bug. `CLAUDE.md` is a symlink to this file, so one edit covers
> both.

## 0. What this is (read first)

A **single-user** desktop work-session tracker: it runs in the system tray on a
worker's machine, prompts for periodic status updates, takes random screenshots
on a separate schedule, and posts everything to a Discord **webhook** in real
time via an **encrypted, offline-capable** queue. It is the local companion to
`../discord-progress-tracker` (a multi-user Discord bot) and replicates that
bot's session behavior (check-in → warning → late → inactive → auto-end, breaks,
end-of-day + next-session-plan flow, optional GitHub commit evidence).

The point of the project is a **trustworthy** activity record, so a few
invariants are load-bearing — see §4. Do not weaken them without the user
explicitly asking.

## 1. Build, Lint, and Test

The GUI uses **Fyne**, which needs CGO + native GL/X11 libraries. Use the
provided Nix shell for anything that touches `internal/ui` or `cmd/agent`.

### In the Nix shell (full tree, incl. GUI)
- **Enter shell:** `nix-shell` (or `direnv allow` once, then it's automatic).
- **Build all:** `go build -buildvcs=false ./...`
- **Per-worker binary:** `WORKER_NAME="Alice" ./build.sh` (see §2).

### Without the shell (core logic only — no display/CGO needed)
These packages have no Fyne/CGO dependency and are the fast inner loop:
`vault state queue session settings` (and `discord`, `github` build fine too).
- **Test core:** `go test ./internal/vault/... ./internal/state/... ./internal/queue/... ./internal/session/... ./internal/settings/...`
- **Race:** add `-race` (the engine runs concurrent timers — keep it clean).

### Lint
- **Format:** `gofmt -w ./cmd ./internal` (always before committing).
- **Vet:** `go vet ./...` (run inside the Nix shell so `ui`/`cmd` are covered).

## 2. Architecture & conventions

- **Module name:** `discord-tracker-agent` (use for internal imports, e.g.
  `discord-tracker-agent/internal/vault`).
- **Layout:** standard Go — `cmd/agent` (wiring/main), `internal/*` (everything
  else). One concern per package.
- **No database.** Unlike the bot, there is no SQLite. All persistent state is
  two AES-GCM–sealed files under `os.UserConfigDir()/session-agent/`
  (`state.dat`, `queue.dat`) plus `shots/*.png`.
- **Compile-time settings (`internal/settings`).** Per-worker config (name,
  intervals, thresholds, GitHub token, AES key) is injected at link time via
  `-ldflags -X` from `build.sh`. There is deliberately **no runtime config
  file** — a worker must not be able to edit intervals/name. Numeric knobs are
  strings parsed in `init()` with the bot's defaults as fallback.
- **Webhook is runtime, not compile-time.** It's prompted for on first launch
  and editable from the tray; stored (encrypted) in `state.WebhookURL`. Read it
  live via `Engine.Webhook()` — never cache it across a possible change.
- **The engine is UI- and side-effect-free.** `internal/session` must NOT import
  Fyne, `capture`, or `github`. The UI (`session.UI` interface) and screenshots/
  commits are injected as hooks (`Config.CaptureFn`, `Config.CommitsFn`). This is
  what lets the whole engine be unit-tested with tiny durations and no display —
  **keep it that way.** If the engine needs a new side effect, inject it.
- **Imports:** group stdlib / third-party / internal. Errors: check immediately,
  wrap with `fmt.Errorf("context: %w", err)`. Avoid panics.

## 3. Vocabulary

Terms as used in this codebase (and mirrored from the bot where noted). Keep this
list in sync when you introduce or rename a concept.

- **Session** — one work period, from app launch (after consent) to end-of-day or
  auto-end. `state.SessionID` increments per session; `Updates` is cleared when a
  session finalizes. One active session at a time (single-user).
- **Update / the log** — every session event is a `state.Update` appended to the
  single `State.Updates` slice: status prompts *and* system events (breaks,
  misses, EOD report, next plan). This is "the log". Mirrors the bot's one
  append-only `updates` table.
- **Kind** — an `Update.Kind` tag: `plan | update | break_start | break_end |
  missed_late | missed_inactive | auto_end | eod_report | next_plan | eod_timeout`
  (queue items add `checkin | warning | break_alert | screenshot | report`).
- **Status** — `ontime | late | missed`; drives the report emoji (✅/⚠️/❌).
- **Check-in** — a periodic prompt ("What are you working on?"). Fired by the
  engine on a jittered timer; the worker answers via the input window.
- **Cycle** — one check-in and its escalation window. `Engine.cycle` increments
  each check-in; warning/late/inactive timers capture their cycle number and
  no-op if it's stale (superseded or answered).
- **Escalation** — the unanswered-check-in ladder: **warning** (before the
  deadline) → **late** (`missed_late`) → **inactive** (`missed_inactive` after a
  grace period, once `InactiveThreshold` lates) → **auto-end** (finalize once
  `AutoEndThreshold` misses). Thresholds are compile-time settings.
- **Break** — worker-initiated pause (tray). Stops the check-in *and* screenshot
  timers and emits periodic **break alerts**; ending it resumes both.
- **EOD flow** — the two-step end-of-day exchange: prompt for the end-of-day
  report, then the next-session plan (`none`/`no` skips it), then finalize.
- **Finalize** — build the report embed from `Updates`, enqueue it, reset for the
  next session. `NextSeq` is never reset.
- **Report** — the end-of-day Discord embed assembled from the session's updates;
  optionally appends GitHub commits as corroborating output evidence.
- **Vault / seal / open** — `internal/vault`: AES-GCM `Seal`/`Open` + atomic
  `WriteFile`. "Sealed" = encrypted-and-authenticated on disk. Any edit fails
  `Open`.
- **State / queue** — the two sealed files. **State** = consent + seq counter +
  the log + break/webhook fields. **Queue** = the outbox (`queue.Item`s).
- **Item** — one queued outbound thing (update, system event, screenshot, or the
  report). Carries its `Seq`; screenshots carry `ImageSHA`.
- **Drain** — `Queue.Drain`: send items oldest-first, stop at the first failure,
  retry next tick. This is the offline-recovery mechanism.
- **Seq (sequence number)** — the monotonic `state.NextSeq`, handed out by
  `State.Take()`, stamped on every item and surfaced in Discord so gaps/reorders
  are visible. The anti-replay/anti-delete anchor; never rewound.
- **Hook** — a function injected into the engine (`Config.CaptureFn`,
  `Config.CommitsFn`) or the `session.UI` interface, so `internal/session` stays
  free of Fyne/CGO and is testable headless.
- **Consent gate** — the hard first-run window; declining exits the app before
  anything is captured.
- **Jitter** — randomized ± offset applied to the check-in and screenshot base
  intervals so timings aren't predictable.
- **Compile-time settings vs. runtime webhook** — everything in
  `internal/settings` is baked in via `-ldflags -X`; the webhook URL is the one
  value configured at runtime (first-run prompt / tray) and stored in sealed
  state.

## 4. Integrity invariants — DO NOT break these

The whole value proposition is that the local log is tamper-evident. When
editing, preserve all of:

1. **All persistent writes go through `internal/vault`** (`WriteFile`/`ReadFile`,
   i.e. AES-GCM sealed + atomic rename). Never write session/queue data as
   plaintext, and never bypass the vault. A plaintext test would leak the log.
2. **`state.NextSeq` is monotonic and never rewound**, and it lives *inside* the
   sealed blob. Hand out sequence numbers only via `State.Take()`. Every posted
   item carries its seq, surfaced in Discord so gaps/reorders are visible.
3. **Screenshots are hashed, not encrypted.** The sealed queue stores each PNG's
   `sha256`; the sender verifies it (`verifySHA` in `main.go`) before upload and
   drops on mismatch. Keep that check.
4. **The consent gate is hard.** No consent → the app exits and captures nothing.
   Don't add a path that captures/sends before consent is recorded.
5. The embedded AES key / settings are a **documented, accepted ceiling** (a
   reverse-engineer can extract them). Don't "fix" this by adding real secrecy
   scope — and don't weaken it either.

## 5. Testing

- Standard `testing` package, std-lib assertions only (`t.Fatalf`/`t.Errorf`).
  Tests sit next to source (`*_test.go`).
- Each `_test.go` calls `vault.Init(testKey)` in an `init()` before using sealed
  state/queue.
- The engine is tested with **tiny durations** and a `fakeUI` — no clock library,
  no display. Follow that pattern; call `Engine.fireCheckIn()` directly to drive
  a single cycle deterministically.
- Non-trivial logic leaves one runnable check behind. Key existing coverage:
  seal/open + tamper + not-plaintext (`vault`), seq monotonicity across reload
  (`state`), offline drain ordering + PNG cleanup (`queue`), escalation→auto-end
  and break-pauses-capture and runtime-webhook-persist (`session`).

## 6. Workflow for agents
1. **Understand:** read the touched `internal/` package and check §4 invariants.
2. **Implement:** match the surrounding style; keep the engine hook-based.
3. **Verify:** `gofmt -w`, then `go test` on the core (fast), then — if `ui`/`cmd`
   changed — `nix-shell --run 'go build -buildvcs=false ./... && go vet ./...'`.
4. Add/adjust a `_test.go` case for new non-trivial logic.
5. **Update this file** if the change touched anything it documents (packages,
   commands, invariants, vocabulary, tray actions, settings). See the note at the
   top.
