#!/usr/bin/env bash
# Build a per-worker binary with a STABLE baked AES key (stronger tamper model
# than the generic CI release binaries). Run once per worker; reuse the same
# AES_KEY across versions so a worker's existing sealed files stay readable.
#
#   WORKER_NAME="Alice" AES_KEY="<hex from last time>" ./build.sh
#
# WORKER_NAME is optional — it's only the default name (changeable at runtime).
# The webhook URL is NOT baked — it's configured at runtime (first launch / tray).
# Optional overrides (else the bot defaults apply):
#   CHECKIN_BASE_MIN, CHECKIN_JITTER_MIN, SHOT_BASE_MIN, SHOT_JITTER_MIN,
#   WARNING_BEFORE_MIN, LATE_TIMEOUT_MIN, INACTIVE_TIMEOUT_MIN,
#   INACTIVE_THRESHOLD, AUTO_END_THRESHOLD, BREAK_ALERT_MIN, EOD_TIMEOUT_MIN,
#   GITHUB_TOKEN, GITHUB_USERNAME, GITHUB_ORGS
#
# A stricter tracker for a less-trusted worker = smaller CHECKIN_BASE_MIN etc.
set -euo pipefail

# A unique 32-byte key per worker: one worker's extracted key can't open another's files.
AES_KEY="${AES_KEY:-$(openssl rand -hex 32)}"

P="discord-tracker-agent/internal/settings"
LD="-s -w"
add() { [ -n "${2:-}" ] && LD="$LD -X '$P.$1=$2'"; }

add WorkerName "${WORKER_NAME:-}"
add WebhookURL "${WEBHOOK_URL:-}"
add AESKeyHex  "$AES_KEY"
add CheckInBaseMin    "${CHECKIN_BASE_MIN:-}"
add CheckInJitterMin  "${CHECKIN_JITTER_MIN:-}"
add ShotBaseMin       "${SHOT_BASE_MIN:-}"
add ShotJitterMin     "${SHOT_JITTER_MIN:-}"
add WarningBeforeMin  "${WARNING_BEFORE_MIN:-}"
add LateTimeoutMin    "${LATE_TIMEOUT_MIN:-}"
add InactiveTimeoutMin "${INACTIVE_TIMEOUT_MIN:-}"
add InactiveThreshold "${INACTIVE_THRESHOLD:-}"
add AutoEndThreshold  "${AUTO_END_THRESHOLD:-}"
add BreakAlertMin     "${BREAK_ALERT_MIN:-}"
add EODTimeoutMin     "${EOD_TIMEOUT_MIN:-}"
add GitHubToken       "${GITHUB_TOKEN:-}"
add GitHubUsername    "${GITHUB_USERNAME:-}"
add GitHubOrgs        "${GITHUB_ORGS:-}"

OUT="${OUT:-session-agent}"
echo "Building $OUT for worker '${WORKER_NAME:-<runtime default>}' (AES key ${AES_KEY:0:8}…)"
eval CGO_ENABLED=1 go build -ldflags \"$LD\" -o "$OUT" ./cmd/session-agent
echo "Done: $OUT"
