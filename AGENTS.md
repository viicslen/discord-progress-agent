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
provided Nix shell for anything that touches `internal/ui` or `cmd/session-agent`.

### In the Nix shell (full tree, incl. GUI)
- **Enter shell:** `nix develop` (flake) or `nix-shell` (`shell.nix`); `direnv
  allow` once picks up `.envrc`. Both provide Go + the Fyne CGO/X11 deps.
- **Build all:** `go build -buildvcs=false ./...`
- **Nix package:** `nix build .#default` (whole-app Fyne build via `flake.nix`).
- **Per-worker binary:** `WORKER_NAME="Alice" ./build.sh` (see §2).

### Without the shell (core logic only — no display/CGO needed)
These packages have no Fyne/CGO dependency and are the fast inner loop:
`vault state queue session settings` (and `discord`, `github` build fine too).
- **Test core:** `go test ./internal/vault/... ./internal/state/... ./internal/queue/... ./internal/session/... ./internal/settings/...`
- **Race:** add `-race` (the engine runs concurrent timers — keep it clean).

### Lint
- **Format:** `gofmt -w ./cmd ./internal` (always before committing).
- **Vet:** `go vet ./...` (run inside the Nix shell so `ui`/`cmd` are covered).

### CI/CD (`.github/workflows/`)
- `ci.yml` — on push/PR: gofmt check + `go vet` + `go test -race` on the **core**
  packages only (no Fyne/CGO, so it's fast), plus a full compile matrix on
  Linux/macOS/Windows (the matrix is where ui/cmd + Fyne actually get built and
  vetted). Keep it green.
- `release.yml` — **release-please** driven. On push to main it maintains a
  release PR (version bump + `CHANGELOG.md`) from Conventional Commits; merging it
  creates the tag + GitHub Release, and the gated `build` job then compiles a
  generic binary per OS and uploads them as assets. The build runs in the *same*
  workflow (a GITHUB_TOKEN-created tag can't trigger a separate tag listener).
  Config: `release-please-config.json` + `.release-please-manifest.json`. The
  config's `extra-files` makes release-please also bump the `version` in
  `flake.nix` (the `# x-release-please-version` marker line) on each release —
  keep that marker intact. `vendorHash` is **not** auto-updated: refresh it by
  hand when `go.sum` changes (`nix build .#default 2>&1 | grep got:`).
- **Commit messages must be Conventional Commits** (`feat:`, `fix:`, `feat!:`/
  `BREAKING CHANGE:` for majors, `chore:`/`docs:`/`refactor:` for no bump) so
  release-please can compute the version. If you change build flags, Fyne deps, or
  the module path, update **both** workflows.

## 2. Architecture & conventions

- **Module name:** `discord-tracker-agent` (use for internal imports, e.g.
  `discord-tracker-agent/internal/vault`).
- **Layout:** standard Go — `cmd/session-agent` (wiring/main; binary name avoids
  colliding with other `agent` CLIs), `internal/*` (everything
  else). One concern per package.
- **No database.** Unlike the bot, there is no SQLite. All persistent state is
  two AES-GCM–sealed files under `os.UserConfigDir()/session-agent/`
  (`state.dat`, `queue.dat`) plus `shots/*.png`.
- **Compile-time settings (`internal/settings`).** Tunable config (intervals,
  thresholds, GitHub token) is injected at link time via `-ldflags -X`. There is
  no runtime config file for these — a worker must not edit intervals/thresholds.
  Numeric knobs are strings parsed in `init()` with the bot's defaults as
  fallback. `Version` is baked by the release workflow.
- **Two build flavors.** `build.sh` makes a **per-worker** binary with a stable
  baked `AESKeyHex` (stronger tamper model; reuse the same key across versions
  or existing sealed files become unreadable). CI `release.yml` makes **generic**
  binaries with nothing per-worker baked: the AES key is provisioned per machine
  on first run (`key.bin`, 0600 — an accepted weakening, see §4.5) and the worker
  name defaults at runtime.
- **Runtime-configurable (not compile-time): webhook and worker name.** Both are
  stored encrypted in state (`state.WebhookURL`, `state.WorkerName`) and edited
  from the tray. `WorkerName` compiled in is only a *default*; `settings.WebhookURL`
  is an optional default. Read live via `Engine.Webhook()` / `Engine.Name()` —
  never cache across a possible change. Titles use `Engine.nameLocked()` under mu.
- **Capture is display-server aware (`internal/capture`).** `All()` →
  `platformCapture`: on Linux, Wayland uses the XDG Screenshot portal over D-Bus
  (`capture_linux.go`, `godbus/dbus/v5`), X11 and macOS/Windows use kbinani
  (`capture.go` / `capture_other.go`). Keep the D-Bus/portal code behind the
  `//go:build linux` tag so non-Linux builds don't pull it in.
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
- **Compile-time settings vs. runtime values** — tunables (intervals,
  thresholds, GitHub token) are baked via `-ldflags -X`; the **webhook** and
  **worker name** are configured at runtime (tray / first-run) and stored in
  sealed state. Compiled-in name/webhook are only *defaults*.
- **Per-worker vs. generic build** — `build.sh` bakes a stable AES key per
  worker; CI `release.yml` ships generic binaries that provision a per-machine
  key (`key.bin`) on first run.
- **Version** — `settings.Version` (baked in CI from the tag); `agent --version`
  prints it.

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
   reverse-engineer can extract them). Generic release binaries weaken this
   further on purpose: with no baked key they store a per-machine `key.bin` on
   disk, so those files are only casual-edit-proof, not owner-proof. Per-worker
   `build.sh` binaries keep the stronger baked-key model. Don't "fix" either into
   real secrecy scope, and don't weaken the per-worker path.

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
