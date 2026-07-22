// Package settings holds per-worker configuration baked in at COMPILE TIME via
// -ldflags "-X". There is deliberately no runtime config file: a worker must not
// be able to change intervals, the webhook URL, their name, or the GitHub token
// without recompiling. Build one binary per worker (see build.sh).
//
// ponytail: -X only sets string vars, so numeric knobs are strings parsed once
// in init() with the bot's defaults as fallback when a var is left empty.
package settings

import (
	"log"
	"strconv"
	"time"
)

// Injected at build time. Empty => use the default parsed below.
var (
	Version = "dev" // release tag, baked in CI

	WorkerName = "" // compile-time DEFAULT name; changeable at runtime (tray)
	WebhookURL = "" // optional compile-time default; normally configured at runtime
	AESKeyHex  = "" // 64 hex chars; if empty (generic build) a per-machine key is provisioned at runtime

	CheckInBaseMin   = "60"
	CheckInJitterMin = "15"
	ShotBaseMin      = "50"
	ShotJitterMin    = "10"

	WarningBeforeMin   = "3"
	LateTimeoutMin     = "30"
	InactiveTimeoutMin = "10"
	InactiveThreshold  = "2"
	AutoEndThreshold   = "3"
	BreakAlertMin      = "30"
	EODTimeoutMin      = "5"

	GitHubToken    = "" // PAT; empty disables the GitHub feature
	GitHubUsername = ""
	GitHubOrgs     = "" // optional CSV org filter
)

// Parsed values, populated by init().
var (
	CheckInBase   time.Duration
	CheckInJitter time.Duration
	ShotBase      time.Duration
	ShotJitter    time.Duration
	WarningBefore time.Duration
	LateTimeout   time.Duration
	InactiveTO    time.Duration
	BreakAlert    time.Duration
	EODTimeout    time.Duration

	InactiveThresholdN int
	AutoEndThresholdN  int
)

func init() {
	CheckInBase = mins(CheckInBaseMin, 60)
	CheckInJitter = mins(CheckInJitterMin, 15)
	ShotBase = mins(ShotBaseMin, 50)
	ShotJitter = mins(ShotJitterMin, 10)
	WarningBefore = mins(WarningBeforeMin, 3)
	LateTimeout = mins(LateTimeoutMin, 30)
	InactiveTO = mins(InactiveTimeoutMin, 10)
	BreakAlert = mins(BreakAlertMin, 30)
	EODTimeout = mins(EODTimeoutMin, 5)
	InactiveThresholdN = count(InactiveThreshold, 2)
	AutoEndThresholdN = count(AutoEndThreshold, 3)
}

func mins(s string, def int) time.Duration {
	return time.Duration(count(s, def)) * time.Minute
}

func count(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		log.Printf("settings: bad int %q, using default %d", s, def)
		return def
	}
	return n
}
