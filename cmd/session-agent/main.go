// Command agent is the single-user work-session tracker. Tunable per-worker
// settings are compiled in via -ldflags (see internal/settings + build.sh); the
// worker name is a compile-time default that can be changed at runtime, and the
// webhook is configured at runtime. On first run it shows a hard consent gate;
// on agreement it runs in the system tray, prompting for updates and taking
// screenshots, and drains an encrypted offline queue to a Discord webhook.
package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/app"
	"fyne.io/fyne/v2/driver/desktop"

	"discord-tracker-agent/internal/capture"
	"discord-tracker-agent/internal/discord"
	"discord-tracker-agent/internal/github"
	"discord-tracker-agent/internal/queue"
	"discord-tracker-agent/internal/session"
	"discord-tracker-agent/internal/settings"
	"discord-tracker-agent/internal/state"
	"discord-tracker-agent/internal/ui"
	"discord-tracker-agent/internal/vault"
)

func main() {
	log.SetFlags(log.LstdFlags)

	if len(os.Args) > 1 && (os.Args[1] == "--version" || os.Args[1] == "-v") {
		fmt.Println(settings.Version)
		return
	}

	dir := configDir()

	if err := vault.Init(resolveKey(dir)); err != nil {
		log.Fatalf("vault init: %v", err)
	}

	st, err := state.Load(filepath.Join(dir, "state.dat"))
	if err != nil {
		log.Fatalf("state load (tampered/corrupt?): %v", err)
	}
	q, err := queue.Load(filepath.Join(dir, "queue.dat"))
	if err != nil {
		log.Fatalf("queue load (tampered/corrupt?): %v", err)
	}
	shotsDir := filepath.Join(dir, "shots")

	if settings.WebhookURL == "" && st.WebhookURL == "" {
		log.Print("no webhook configured — items will queue until one is set via the tray")
	}

	// NewWithID (not New): a unique app ID is required for notifications and the
	// preferences API, and it namespaces the OS notification identity.
	a := app.NewWithID("com.viicslen.discord-progress-agent")
	a.SetIcon(appIconResource())
	ctx, cancel := context.WithCancel(context.Background())

	var eng *session.Engine
	u := ui.New(a, func(text string) {
		if eng != nil {
			eng.Submit(text)
		}
	})

	start := func() {
		eng = session.New(engineConfig(shotsDir), u, st, q, func() {})
		setupTray(a, eng, u, cancel)
		go eng.Run(ctx)
		go drainLoop(ctx, q, eng)
		// No webhook yet (first run, or never configured) → ask for one. Items
		// keep queuing until it's set, so this is non-blocking.
		if eng.Webhook() == "" {
			u.ShowSettings(eng.Name(), "", func(name, webhook string) {
				if name = strings.TrimSpace(name); name != "" {
					eng.SetName(name)
				}
				if webhook = strings.TrimSpace(webhook); webhook != "" {
					eng.SetWebhook(webhook)
				}
			})
		}
	}

	if !st.Consent {
		u.ShowConsent(defaultName(), func() {
			st.Consent = true
			st.ConsentAt = time.Now().Unix()
			if err := st.Save(); err != nil {
				log.Printf("save consent: %v", err)
			}
			start()
		})
	} else {
		start()
	}

	a.Run()
	cancel()
}

func engineConfig(shotsDir string) session.Config {
	return session.Config{
		WorkerName:        defaultName(),
		DefaultWebhook:    settings.WebhookURL,
		CheckInBase:       settings.CheckInBase,
		CheckInJitter:     settings.CheckInJitter,
		ShotBase:          settings.ShotBase,
		ShotJitter:        settings.ShotJitter,
		WarningBefore:     settings.WarningBefore,
		LateTimeout:       settings.LateTimeout,
		InactiveTO:        settings.InactiveTO,
		BreakAlert:        settings.BreakAlert,
		EODTimeout:        settings.EODTimeout,
		InactiveThreshold: settings.InactiveThresholdN,
		AutoEndThreshold:  settings.AutoEndThresholdN,
		CaptureFn:         func() ([]session.Shot, error) { return capture.All(shotsDir) },
		CommitsFn:         github.TodayCommits,
	}
}

func setupTray(a fyne.App, eng *session.Engine, u *ui.UI, cancel context.CancelFunc) {
	desk, ok := a.(desktop.App)
	if !ok {
		return
	}
	desk.SetSystemTrayIcon(appIconResource())
	var addUpdate, startBreak, endBreak, startSession, endSession *fyne.MenuItem
	addUpdate = fyne.NewMenuItem("Add update…", func() { u.Prompt("Update", "What are you working on?") })
	settingsItem := fyne.NewMenuItem("Settings…", func() {
		u.ShowSettings(eng.Name(), eng.Webhook(), func(name, webhook string) {
			if name = strings.TrimSpace(name); name != "" {
				eng.SetName(name)
			}
			if webhook = strings.TrimSpace(webhook); webhook != "" {
				eng.SetWebhook(webhook)
			}
		})
	})
	startBreak = fyne.NewMenuItem("Start break", func() {
		eng.StartBreak()
		refreshTrayState(addUpdate, startBreak, endBreak, startSession, endSession, eng)
	})
	endBreak = fyne.NewMenuItem("End break", func() {
		eng.EndBreak()
		refreshTrayState(addUpdate, startBreak, endBreak, startSession, endSession, eng)
	})
	startSession = fyne.NewMenuItem("Start session", func() {
		eng.StartSession()
		refreshTrayState(addUpdate, startBreak, endBreak, startSession, endSession, eng)
	})
	endSession = fyne.NewMenuItem("End session", func() {
		eng.EndSession()
		refreshTrayState(addUpdate, startBreak, endBreak, startSession, endSession, eng)
	})
	quitItem := fyne.NewMenuItem("Quit", func() {
		cancel()
		a.Quit()
	})
	m := fyne.NewMenu("Session Agent",
		addUpdate,
		fyne.NewMenuItemSeparator(),
		startSession,
		endSession,
		fyne.NewMenuItemSeparator(),
		startBreak,
		endBreak,
		fyne.NewMenuItemSeparator(),
		settingsItem,
		fyne.NewMenuItemSeparator(),
		quitItem,
	)
	refreshTrayState(addUpdate, startBreak, endBreak, startSession, endSession, eng)
	desk.SetSystemTrayMenu(m)
}

func refreshTrayState(addUpdate, startBreak, endBreak, startSession, endSession *fyne.MenuItem, eng *session.Engine) {
	s := eng.Snapshot()
	active := s.Active
	onBreak := s.OnBreak
	pending := s.Pending

	addUpdate.Disabled = !active || pending
	startBreak.Disabled = !active || onBreak || pending
	endBreak.Disabled = !active || !onBreak
	startSession.Disabled = active || pending
	endSession.Disabled = !active || pending
}

// drainLoop sends queued items to the current webhook every 30s (and once at
// startup). The URL is read live from the engine each cycle so a runtime change
// takes effect immediately.
func drainLoop(ctx context.Context, q *queue.Queue, eng *session.Engine) {
	drain := func() {
		url := eng.Webhook()
		if url == "" {
			return // nothing configured yet; keep items queued
		}
		q.Drain(func(it queue.Item) error { return sendItem(url, it) })
	}
	drain()
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			drain()
		}
	}
}

func sendItem(url string, it queue.Item) error {
	switch {
	case it.ImagePath != "":
		if !verifySHA(it.ImagePath, it.ImageSHA) {
			log.Printf("screenshot %s failed integrity check — dropping", it.Filename)
			return nil // drop tampered evidence rather than retry forever
		}
		e := discord.Embed{
			Title:       it.Title,
			Description: fmt.Sprintf("seq %d", it.Seq),
			Color:       it.Color,
			Timestamp:   discord.RFC3339Now(),
		}
		return discord.SendImage(url, it.Filename, it.ImagePath, e)
	case it.Embed != nil:
		return discord.SendEmbed(url, *it.Embed)
	default:
		e := discord.Embed{
			Title:       it.Title,
			Description: fmt.Sprintf("%s\n\n*seq %d*", it.Content, it.Seq),
			Color:       it.Color,
			Timestamp:   discord.RFC3339Now(),
		}
		return discord.SendEmbed(url, e)
	}
}

func verifySHA(path, want string) bool {
	if want == "" {
		return true
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]) == want
}

func configDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base = "."
	}
	d := filepath.Join(base, "session-agent")
	_ = os.MkdirAll(d, 0o700)
	return d
}

// defaultName is the compile-time worker name, or the OS user as a fallback for
// generic (unbaked) builds. It is only the DEFAULT — a runtime override in the
// sealed state takes precedence (Engine.Name/SetName).
func defaultName() string {
	if settings.WorkerName != "" {
		return settings.WorkerName
	}
	if u, err := user.Current(); err == nil && u.Username != "" {
		return u.Username
	}
	return "worker"
}

// resolveKey returns the AES key. A per-worker build bakes settings.AESKeyHex.
// A generic build has none, so we provision a random per-machine key once and
// store it (0600) next to the sealed files. ponytail: this is the accepted
// weakening for generic release binaries — the key sits on disk, so it stops
// casual editing, not a determined machine owner. Per-worker builds via build.sh
// keep the stronger baked-key model.
func resolveKey(dir string) []byte {
	if settings.AESKeyHex != "" {
		k, err := hex.DecodeString(settings.AESKeyHex)
		if err != nil || len(k) != 32 {
			log.Fatalf("bad baked AES key: %v", err)
		}
		return k
	}
	path := filepath.Join(dir, "key.bin")
	if b, err := os.ReadFile(path); err == nil && len(b) == 32 {
		return b
	}
	k := make([]byte, 32)
	if _, err := rand.Read(k); err != nil {
		log.Fatalf("generate key: %v", err)
	}
	if err := os.WriteFile(path, k, 0o600); err != nil {
		log.Fatalf("store key: %v", err)
	}
	return k
}
