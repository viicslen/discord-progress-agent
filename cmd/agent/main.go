// Command agent is the single-user work-session tracker. All per-worker config
// is compiled in via -ldflags (see internal/settings + build.sh); there is no
// runtime config file. On first run it shows a hard consent gate; on agreement
// it runs in the system tray, prompting for updates and taking screenshots, and
// drains an encrypted offline queue to a Discord webhook.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log"
	"os"
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

	if settings.WorkerName == "" {
		log.Fatal("built without WorkerName — see build.sh")
	}
	key, err := hex.DecodeString(settings.AESKeyHex)
	if err != nil {
		log.Fatalf("bad AES key: %v", err)
	}
	if err := vault.Init(key); err != nil {
		log.Fatalf("vault init: %v", err)
	}

	dir := configDir()
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

	a := app.New()
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
			u.ShowForm("Configure webhook", "Enter the Discord webhook URL to send this worker's activity to:", "", func(url string) {
				if url = strings.TrimSpace(url); url != "" {
					eng.SetWebhook(url)
				}
			})
		}
	}

	if !st.Consent {
		u.ShowConsent(settings.WorkerName, func() {
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
		WorkerName:        settings.WorkerName,
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
	m := fyne.NewMenu("Session Agent",
		fyne.NewMenuItem("Add update…", func() { u.Prompt("Update", "What are you working on?") }),
		fyne.NewMenuItem("Change webhook…", func() {
			u.ShowForm("Change webhook", "New Discord webhook URL:", eng.Webhook(), func(url string) {
				if url = strings.TrimSpace(url); url != "" {
					eng.SetWebhook(url)
				}
			})
		}),
		fyne.NewMenuItem("Start break", func() { eng.StartBreak() }),
		fyne.NewMenuItem("End break", func() { eng.EndBreak() }),
		fyne.NewMenuItem("End session", func() { eng.EndSession() }),
		fyne.NewMenuItem("Quit", func() {
			cancel()
			a.Quit()
		}),
	)
	desk.SetSystemTrayMenu(m)
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
