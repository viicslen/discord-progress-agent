# Session Agent

Desktop work-session tracking for one worker.

It runs in the system tray, asks for periodic status updates, takes screenshots on
a separate schedule, and posts everything to a Discord webhook. If the network is
down, it queues items locally and retries later.

It is the local companion to `discord-progress-tracker` and mirrors the same
session flow:

- check-in
- warning
- late
- inactive
- auto-end
- breaks
- end-of-day report
- next-session plan

## Quick Start

### Install the latest release

```bash
curl -fsSL https://raw.githubusercontent.com/viicslen/discord-progress-agent/main/install.sh | sh
```

By default the script installs `session-agent` to `/usr/local/bin` when writable,
otherwise to `~/.local/bin`. Override with `INSTALL_DIR=/path/to/bin`.

### Run with Nix

```bash
nix run github:viicslen/discord-progress-agent
```

### Install with Nix

```bash
nix profile install github:viicslen/discord-progress-agent
```

### Open the dev shell

```bash
nix develop
```

## What It Does

- Lives in the system tray
- Prompts for status updates on a schedule
- Takes screenshots on a separate schedule
- Sends updates and screenshots to a Discord webhook
- Queues data locally when offline
- Retries queued items in order
- Pauses screenshots and check-ins during breaks
- Requires first-run consent before capturing anything

## Runtime Behavior

### First launch

1. You see a consent window.
2. If you decline, the app exits and records nothing.
3. If no webhook is configured yet, the app asks for one.

### Tray actions

- `Add update…`
- `Change webhook…`
- `Change name…`
- `Start break`
- `End break`
- `End session`
- `Quit`

### Local files

State lives under `os.UserConfigDir()/session-agent/`:

- `state.dat`
- `queue.dat`
- `shots/*.png`
- `key.bin` for generic builds only

## Security Model

### Protected locally

- Session state and the outbound queue are sealed with AES-GCM.
- Files are not human-readable.
- Manual edits fail authentication.
- Every queued item carries a monotonic sequence number.
- Sequence numbers are surfaced in Discord so gaps and reordering are visible.

### Important limit

This is tamper-resistant, not tamper-proof.

- Generic builds store a per-machine key on disk.
- Per-worker builds bake a stable key into the binary.
- A determined machine owner can still reverse-engineer or interfere.

The goal is to stop casual editing and make missing or reordered evidence visible.

## Configuration Model

### Runtime-configurable

These are set at runtime and stored in sealed state:

- worker name
- Discord webhook URL

### Build-time configurable

These are compiled in with `-ldflags -X`:

- check-in timing
- screenshot timing
- escalation thresholds
- break alert timing
- end-of-day timeout
- optional GitHub commit settings

There is no runtime config file for those tunables.

## Build Options

### Option 1: Generic build

Used by:

- `install.sh`
- GitHub releases
- `nix run`
- `nix build`

Behavior:

- no worker name baked in
- no AES key baked in
- provisions `key.bin` on first run
- defaults the worker name from the OS user

Check the version with:

```bash
session-agent --version
```

### Option 2: Per-worker build

Use `build.sh` when you want a stronger per-worker tamper model.

```bash
WORKER_NAME="Alice" AES_KEY="<same hex as last build>" ./build.sh
```

Notes:

- Reuse the same `AES_KEY` across versions for that worker.
- If you change the key, that worker's old sealed files become unreadable.
- `WORKER_NAME` is only a default and can still be changed at runtime.
- `AES_KEY` is auto-generated if omitted.

Optional build-time overrides:

- `CHECKIN_BASE_MIN` default: `60`
- `CHECKIN_JITTER_MIN` default: `15`
- `SHOT_BASE_MIN` default: `50`
- `SHOT_JITTER_MIN` default: `10`
- `WARNING_BEFORE_MIN` default: `3`
- `LATE_TIMEOUT_MIN` default: `30`
- `INACTIVE_TIMEOUT_MIN` default: `10`
- `INACTIVE_THRESHOLD` default: `2`
- `AUTO_END_THRESHOLD` default: `3`
- `BREAK_ALERT_MIN` default: `30`
- `EOD_TIMEOUT_MIN` default: `5`
- `GITHUB_TOKEN` default: empty
- `GITHUB_USERNAME` default: empty
- `GITHUB_ORGS` default: empty

## Platform Notes

The GUI uses Fyne, so it needs CGO and native desktop libraries.

### Linux

- use the provided Nix shell, or
- install a C toolchain plus `libgl1-mesa-dev` and `xorg-dev`
- GNOME may also need an AppIndicator extension for tray support

### macOS

- install Xcode command-line tools
- use a signed `.app` bundle for notifications and screen-recording permission

### Windows

- install MinGW-w64

## Screenshot Capture

Capture depends on the display server:

- Xorg: direct display capture
- Wayland: XDG desktop portal via `org.freedesktop.portal.Screenshot`

Wayland requirements:

- a working `xdg-desktop-portal`
- a supported desktop backend such as GNOME, KDE, or wlroots

Capture failure is treated as a normal unavailable state. It is logged, but it
does not crash the app.

## Build And Release

Releases use [release-please](https://github.com/googleapis/release-please).

Use [Conventional Commits](https://www.conventionalcommits.org/):

- `feat:`
- `fix:`
- `feat!:` or `BREAKING CHANGE:` for breaking changes

Workflow:

1. Merge conventional commits into `main`.
2. release-please updates the release PR and `CHANGELOG.md`.
3. Merge the release PR.
4. GitHub tags the release and publishes binaries for Linux, macOS, and Windows.

`flake.nix` version bumps are also managed by release-please.

If `go.sum` changes, refresh the Nix `vendorHash` with:

```bash
nix build .#default 2>&1 | grep got:
```

## Development

### Build everything

Run inside the Nix shell when touching GUI code:

```bash
go build -buildvcs=false ./...
```

### Run core tests

```bash
go test ./internal/vault/... ./internal/state/... ./internal/queue/... ./internal/session/... ./internal/settings/...
```

### Format

```bash
gofmt -w ./cmd ./internal
```

### Vet

```bash
go vet ./...
```

## Test Coverage

Core tests cover:

- vault seal/open behavior
- tamper detection
- non-plaintext persistence checks
- sequence monotonicity across reloads
- offline queue drain ordering
- screenshot queue cleanup
- escalation through auto-end
- break behavior
- runtime webhook persistence

These tests do not require a display server or CGO.
