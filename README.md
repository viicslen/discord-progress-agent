# Session Agent

A single-user desktop work-session tracker. It runs in the system tray on a
worker's machine, asks for periodic status updates, takes random screenshots on
a separate schedule, and posts everything to a Discord webhook in real time —
queuing locally (encrypted) and retrying when the connection is spotty. It is the
local companion to `discord-progress-tracker` and replicates its check-in /
warning / late / inactive / auto-end escalation, breaks, and end-of-day flow.

## Design at a glance

- **Tunables baked in, identity configured at runtime.** Intervals, thresholds,
  and the GitHub token are baked with `-ldflags -X` (no runtime config file — a
  worker can't loosen their own intervals). The **webhook URL** and **worker
  name** are configured at runtime instead: asked for / defaulted on first launch
  and changeable from the tray ("Change webhook…", "Change name…"), stored
  encrypted in sealed state.
- **Two build flavors.** *Generic* release binaries (from CI, one per OS) bake
  nothing per-worker — they provision a per-machine AES key on first run. *Per-
  worker* binaries from `build.sh` bake a stable AES key for a stronger tamper
  model. See **Build & release** below.
- **Tamper-resistant local files.** Session state and the offline queue are
  sealed with **AES-GCM**: not human-readable, and any edit fails the auth tag,
  so casual "open the file and change the log" is impossible. Each item carries a
  **monotonic sequence number** surfaced in Discord, so deleted/reordered/replayed
  items are visible to a reviewer. (Ceiling: a determined reverse-engineer can
  extract the embedded key — this stops casual editing, not a motivated attacker.)
- **Hard consent gate.** First run shows a notice-and-consent window. Decline or
  close it and the app exits having captured nothing.
- **Offline-first.** Updates, screenshots, and the report all flow through the
  encrypted queue and drain to the webhook when online, in order, surviving
  restarts.

## Build & release

### Generic release binaries (CI)

Push a version tag and GitHub Actions (`.github/workflows/release.yml`) builds a
binary for Linux, macOS, and Windows and attaches them to a GitHub Release:

```bash
git tag v0.1.0 && git push origin v0.1.0
```

These are generic: no name/key baked in. Each machine provisions its own AES key
on first launch, and the worker name defaults to the OS user (changeable in the
tray). `agent --version` prints the tag. `ci.yml` runs gofmt/vet/`go test -race`
and a compile matrix on every push/PR.

### Per-worker binary (hardened, optional)

For a stronger tamper model, bake a **stable** AES key per worker with `build.sh`:

```bash
WORKER_NAME="Alice" AES_KEY="<same hex as last build>" ./build.sh
```

Reuse the same `AES_KEY` across versions, or that worker's existing sealed files
become unreadable. `AES_KEY` is auto-generated if omitted. `WORKER_NAME` is only
the default (still changeable at runtime). Optional overrides (else the bot's
defaults apply): `CHECKIN_BASE_MIN`, `CHECKIN_JITTER_MIN`, `SHOT_BASE_MIN`,
`SHOT_JITTER_MIN`, `WARNING_BEFORE_MIN`, `LATE_TIMEOUT_MIN`,
`INACTIVE_TIMEOUT_MIN`, `INACTIVE_THRESHOLD`, `AUTO_END_THRESHOLD`,
`BREAK_ALERT_MIN`, `EOD_TIMEOUT_MIN`, and for the optional GitHub commit evidence
`GITHUB_TOKEN` / `GITHUB_USERNAME` / `GITHUB_ORGS`.

### Build prerequisites (Fyne + CGO)

The GUI uses Fyne, which needs CGO and native GL/X11 dev libraries, so it must be
built **on each target OS** (no clean cross-compile). Install per platform:

- **Linux:** a C toolchain + `libgl1-mesa-dev xorg-dev` (and an AppIndicator
  extension for the tray on GNOME).
- **macOS:** Xcode command-line tools. For notifications and Screen-Recording
  permission to work, ship a signed `.app` bundle (`fyne package`), not a bare
  binary.
- **Windows:** MinGW-w64.

Screenshots use X11/XShm and will fail on pure Wayland; the app treats
"capture unavailable" as a normal state (logged, never a crash).

## Run

Launch the binary. First run → consent window, then a prompt for the Discord
webhook URL. After that it lives in the tray with: **Add update…**,
**Change webhook…**, **Change name…**, **Start break**, **End break**,
**End session**, **Quit**. State and the queue live under
`os.UserConfigDir()/session-agent/`.

## Test

```bash
go test ./internal/vault/... ./internal/state/... ./internal/queue/... \
        ./internal/session/... ./internal/settings/...
```

These cover the integrity core (seal/open/tamper/not-plaintext), sequence
monotonicity across reloads, offline-queue drain ordering, and the full
escalation-to-auto-end path — none of which need a display or CGO.
