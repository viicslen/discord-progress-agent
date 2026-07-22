# Session Agent

A single-user desktop work-session tracker. It runs in the system tray on a
worker's machine, asks for periodic status updates, takes random screenshots on
a separate schedule, and posts everything to a Discord webhook in real time —
queuing locally (encrypted) and retrying when the connection is spotty. It is the
local companion to `discord-progress-tracker` and replicates its check-in /
warning / late / inactive / auto-end escalation, breaks, and end-of-day flow.

## Design at a glance

- **Compile-time config, no runtime settings file.** Per-worker values (name,
  intervals, thresholds, GitHub token, encryption key) are baked into the binary
  with `-ldflags -X`. A worker cannot change intervals or rename themselves
  without recompiling. Build one binary per worker — stricter (shorter intervals)
  for less-trusted workers, looser for the rest. See `build.sh`.
- **Webhook configured at runtime.** The Discord webhook URL is *not* baked in.
  The app asks for it on first launch and it can be changed from the tray
  ("Change webhook…"). It is stored encrypted in the sealed state, so it is not
  plaintext-editable and survives restarts. Until one is set, activity keeps
  queuing locally.
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

## Build (per worker)

```bash
WORKER_NAME="Alice" ./build.sh
```

The webhook is entered at runtime (first launch), not here. Optional overrides
(else the bot's defaults apply): `CHECKIN_BASE_MIN`,
`CHECKIN_JITTER_MIN`, `SHOT_BASE_MIN`, `SHOT_JITTER_MIN`, `WARNING_BEFORE_MIN`,
`LATE_TIMEOUT_MIN`, `INACTIVE_TIMEOUT_MIN`, `INACTIVE_THRESHOLD`,
`AUTO_END_THRESHOLD`, `BREAK_ALERT_MIN`, `EOD_TIMEOUT_MIN`, and for the optional
GitHub commit evidence `GITHUB_TOKEN` / `GITHUB_USERNAME` / `GITHUB_ORGS`.

`build.sh` generates a unique random AES key per build unless you pass `AES_KEY`
(64 hex chars). A different key per worker means one worker's extracted key can't
open another's files.

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
**Change webhook…**, **Start break**, **End break**, **End session**, **Quit**.
State and the queue live under `os.UserConfigDir()/session-agent/`.

## Test

```bash
go test ./internal/vault/... ./internal/state/... ./internal/queue/... \
        ./internal/session/... ./internal/settings/...
```

These cover the integrity core (seal/open/tamper/not-plaintext), sequence
monotonicity across reloads, offline-queue drain ordering, and the full
escalation-to-auto-end path — none of which need a display or CGO.
