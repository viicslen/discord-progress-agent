// Package session is the engine: it replicates the bot's check-in / warning /
// late / inactive / auto-end escalation, breaks, and the end-of-day two-step
// flow, mapped onto a desktop app. It does not import Fyne or the capture/github
// packages directly — the UI and those side effects are injected as hooks, so
// the engine is unit-testable with tiny durations and no display.
package session

import (
	"context"
	"fmt"
	"log"
	"math/rand"
	"strings"
	"sync"
	"time"

	"discord-tracker-agent/internal/discord"
	"discord-tracker-agent/internal/queue"
	"discord-tracker-agent/internal/state"
)

// Message strings copied verbatim from the bot's messages.json.
const (
	msgCheckIn        = "Time for a check-in! What are you working on?"
	msgWarning        = "⚠️ Warning: You have %d minutes left to respond to the check-in!"
	msgMissedLate     = "You missed the last check-in and are marked as late."
	msgMissedInactive = "You missed the last check-in and are marked as inactive."
	msgAutoEnded      = "Your session has been automatically ended due to inactivity (too many missed check-ins)."
	msgBreakAlert     = "⏰ Break reminder: You've been on break for %d minutes. Don't forget to return to work when you're ready!"
	msgAskEOD         = "Please provide your end-of-day report. What did you accomplish today?"
	msgAskNextPlan    = "Do you have a plan for your next session? If yes, please share it. If not, just type 'none' or 'no'."
	msgEODTimeout     = "You didn't provide an end-of-day report in time. Your session has been automatically ended."
	msgSessionEnded   = "Session ended. Generating report..."
	updMissedLate     = "Missed check-in (Late)"
	updMissedInactive = "Missed check-in (Inactive)"
	updAutoEnded      = "Session auto-ended due to inactivity"
	updEODTimeout     = "Session ended (no end-of-day report provided)"
)

const colorGreen = 0x00ff00

// UI is the injected front-end. Implementations must be safe to call from any
// goroutine (the Fyne impl marshals onto the UI thread internally).
type UI interface {
	Notify(title, body string)
	Prompt(title, body string) // raise the input window with a prompt
}

type pending int

const (
	pendingNone pending = iota
	pendingEOD
	pendingPlan
)

type Config struct {
	WorkerName     string
	DefaultWebhook string // compile-time webhook; overridden by state.WebhookURL if set

	CheckInBase   time.Duration
	CheckInJitter time.Duration
	ShotBase      time.Duration
	ShotJitter    time.Duration
	WarningBefore time.Duration
	LateTimeout   time.Duration
	InactiveTO    time.Duration
	BreakAlert    time.Duration
	EODTimeout    time.Duration

	InactiveThreshold int
	AutoEndThreshold  int

	// Hooks (nil-safe). CaptureFn returns screenshots to enqueue; CommitsFn
	// returns a markdown commit block for the report ("" if disabled/failed).
	CaptureFn func() ([]Shot, error)
	CommitsFn func() string
}

// Shot is a captured screenshot on disk. Defined here so session does not import
// the capture package (keeps the engine free of the kbinani/cgo dependency).
type Shot struct {
	Path string
	SHA  string
	Name string
}

type Engine struct {
	cfg   Config
	ui    UI
	st    *state.State
	q     *queue.Queue
	rng   *rand.Rand
	onEnd func() // called after the report is sent (e.g. quit)

	mu       sync.Mutex
	cycle    int64 // increments each check-in; stale timers no-op
	answered bool
	missed   int
	pend     pending

	checkInT *time.Timer
	shotT    *time.Timer
	breakT   *time.Timer
	eodT     *time.Timer
	breakN   int
	closed   bool
}

func New(cfg Config, ui UI, st *state.State, q *queue.Queue, onEnd func()) *Engine {
	return &Engine{
		cfg: cfg, ui: ui, st: st, q: q, onEnd: onEnd,
		rng: rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Run starts the timers and blocks until ctx is cancelled.
func (e *Engine) Run(ctx context.Context) {
	e.mu.Lock()
	if e.st.OnBreak {
		// Resume a break that was active at shutdown.
		e.startBreakTimerLocked()
	} else {
		e.scheduleCheckInLocked()
		e.scheduleShotLocked()
	}
	e.mu.Unlock()

	<-ctx.Done()

	e.mu.Lock()
	e.closed = true
	e.stopAllLocked()
	e.mu.Unlock()
}

// ---- scheduling helpers (call with mu held) ----

func (e *Engine) jitter(base, j time.Duration) time.Duration {
	if j <= 0 {
		return base
	}
	d := base + time.Duration(e.rng.Int63n(int64(2*j))) - j
	if d < time.Second {
		d = time.Second
	}
	return d
}

func (e *Engine) scheduleCheckInLocked() {
	e.checkInT = time.AfterFunc(e.jitter(e.cfg.CheckInBase, e.cfg.CheckInJitter), e.fireCheckIn)
}

func (e *Engine) scheduleShotLocked() {
	e.shotT = time.AfterFunc(e.jitter(e.cfg.ShotBase, e.cfg.ShotJitter), e.fireShot)
}

func (e *Engine) startBreakTimerLocked() {
	e.breakN = 0
	e.breakT = time.AfterFunc(e.cfg.BreakAlert, e.fireBreakAlert)
}

func (e *Engine) stopAllLocked() {
	for _, t := range []*time.Timer{e.checkInT, e.shotT, e.breakT, e.eodT} {
		if t != nil {
			t.Stop()
		}
	}
}

// ---- check-in cycle ----

func (e *Engine) fireCheckIn() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed || e.st.OnBreak || e.pend != pendingNone {
		return
	}
	e.cycle++
	c := e.cycle
	e.answered = false

	e.ui.Notify("Check-in", msgCheckIn)
	e.ui.Prompt("Check-in", msgCheckIn)
	e.post(queue.Item{Kind: "checkin", Title: "Check-in: " + e.nameLocked(), Content: msgCheckIn, Color: colorGreen})

	// Warning before the late deadline.
	if e.cfg.WarningBefore > 0 && e.cfg.WarningBefore < e.cfg.LateTimeout {
		time.AfterFunc(e.cfg.LateTimeout-e.cfg.WarningBefore, func() { e.fireWarning(c) })
	}
	// Late deadline.
	time.AfterFunc(e.cfg.LateTimeout, func() { e.fireLate(c) })

	// Next check-in (periodic, jittered).
	e.scheduleCheckInLocked()
}

func (e *Engine) fireWarning(c int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed || e.answered || e.st.OnBreak || c != e.cycle {
		return
	}
	body := fmt.Sprintf(msgWarning, int(e.cfg.WarningBefore.Minutes()))
	e.ui.Notify("Check-in warning", body)
	e.post(queue.Item{Kind: "warning", Title: "Warning: " + e.nameLocked(), Content: body, Color: 0xffcc00})
}

func (e *Engine) fireLate(c int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed || e.answered || e.st.OnBreak || c != e.cycle {
		return
	}
	u := e.st.Append("missed_late", state.StatusLate, updMissedLate, time.Now().Unix())
	e.ui.Notify("Missed check-in", msgMissedLate)
	e.postUpdate(u)
	e.missed++
	_ = e.st.Save()

	if e.missed >= e.cfg.InactiveThreshold {
		time.AfterFunc(e.cfg.InactiveTO, func() { e.fireInactive(c) })
	}
}

func (e *Engine) fireInactive(c int64) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed || e.answered || e.st.OnBreak || c != e.cycle {
		return
	}
	u := e.st.Append("missed_inactive", state.StatusMissed, updMissedInactive, time.Now().Unix())
	e.ui.Notify("Marked inactive", msgMissedInactive)
	e.postUpdate(u)
	_ = e.st.Save()

	if e.cfg.AutoEndThreshold > 0 && e.missed >= e.cfg.AutoEndThreshold {
		au := e.st.Append("auto_end", state.StatusMissed, updAutoEnded, time.Now().Unix())
		e.postUpdate(au)
		e.ui.Notify("Session ended", msgAutoEnded)
		e.finalizeLocked()
	}
}

// ---- user input ----

// Submit is called by the UI when the worker submits text. It routes based on
// the current pending state (normal update, EOD report, or next-session plan).
func (e *Engine) Submit(content string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed {
		return
	}
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	now := time.Now().Unix()

	switch e.pend {
	case pendingEOD:
		u := e.st.Append("eod_report", state.StatusOnTime, "End-of-day report: "+content, now)
		e.postUpdate(u)
		e.pend = pendingPlan
		if e.eodT != nil {
			e.eodT.Stop()
		}
		_ = e.st.Save()
		e.ui.Prompt("Next session plan", msgAskNextPlan)

	case pendingPlan:
		low := strings.ToLower(content)
		if low != "none" && low != "no" {
			u := e.st.Append("next_plan", state.StatusOnTime, "Next session plan: "+content, now)
			e.postUpdate(u)
		}
		e.finalizeLocked()

	default: // normal update / daily plan
		status := state.StatusOnTime
		kind := "update"
		if len(e.st.Updates) == 0 {
			kind = "plan"
		}
		u := e.st.Append(kind, status, content, now)
		e.postUpdate(u)
		e.answered = true
		e.missed = 0
		_ = e.st.Save()
	}
}

// ---- webhook (runtime, ungated) ----

// nameLocked returns the effective worker name (runtime override, else the
// compile-time default). Caller must hold mu.
func (e *Engine) nameLocked() string {
	if e.st.WorkerName != "" {
		return e.st.WorkerName
	}
	return e.cfg.WorkerName
}

// Name returns the effective worker name.
func (e *Engine) Name() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.nameLocked()
}

// SetName changes the worker name at runtime and persists it (encrypted).
func (e *Engine) SetName(name string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.st.WorkerName = name
	_ = e.st.Save()
}

// Webhook returns the effective webhook: the runtime override if set, else the
// compile-time default.
func (e *Engine) Webhook() string {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.st.WebhookURL != "" {
		return e.st.WebhookURL
	}
	return e.cfg.DefaultWebhook
}

// SetWebhook changes the webhook at runtime and persists it (encrypted) so it
// survives restarts. Queued items drain to the new URL from the next send on.
func (e *Engine) SetWebhook(url string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.st.WebhookURL = url
	_ = e.st.Save()
}

// ---- breaks ----

func (e *Engine) StartBreak() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed || e.st.OnBreak || e.pend != pendingNone {
		return
	}
	e.st.OnBreak = true
	e.st.BreakStart = time.Now().Unix()
	e.answered = true // suppress the current cycle's warning/late while away
	if e.checkInT != nil {
		e.checkInT.Stop()
	}
	if e.shotT != nil {
		e.shotT.Stop()
	}
	u := e.st.Append("break_start", state.StatusOnTime, "Break started", time.Now().Unix())
	e.postUpdate(u)
	_ = e.st.Save()
	e.startBreakTimerLocked()
}

func (e *Engine) EndBreak() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed || !e.st.OnBreak {
		return
	}
	e.st.OnBreak = false
	if e.breakT != nil {
		e.breakT.Stop()
	}
	u := e.st.Append("break_end", state.StatusOnTime, "Break ended", time.Now().Unix())
	e.postUpdate(u)
	_ = e.st.Save()
	e.scheduleCheckInLocked()
	e.scheduleShotLocked()
}

func (e *Engine) fireBreakAlert() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed || !e.st.OnBreak {
		return
	}
	e.breakN++
	mins := int(e.cfg.BreakAlert.Minutes()) * e.breakN
	body := fmt.Sprintf(msgBreakAlert, mins)
	e.ui.Notify("Break reminder", body)
	e.post(queue.Item{Kind: "break_alert", Title: "Break: " + e.nameLocked(), Content: body, Color: 0x3399ff})
	e.breakT = time.AfterFunc(e.cfg.BreakAlert, e.fireBreakAlert)
}

// ---- end of day ----

// EndSession begins the two-step end-of-day flow (tray "End session").
func (e *Engine) EndSession() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed || e.pend != pendingNone {
		return
	}
	if e.checkInT != nil {
		e.checkInT.Stop()
	}
	if e.shotT != nil {
		e.shotT.Stop()
	}
	if e.breakT != nil {
		e.breakT.Stop()
	}
	e.pend = pendingEOD
	e.ui.Prompt("End of day", msgAskEOD)
	e.eodT = time.AfterFunc(e.cfg.EODTimeout, e.fireEODTimeout)
}

func (e *Engine) fireEODTimeout() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.closed || e.pend != pendingEOD {
		return
	}
	u := e.st.Append("eod_timeout", state.StatusMissed, updEODTimeout, time.Now().Unix())
	e.postUpdate(u)
	e.finalizeLocked()
}

// finalizeLocked builds and enqueues the report, resets for the next session,
// and calls onEnd. Caller holds mu.
func (e *Engine) finalizeLocked() {
	report := e.buildReport()
	seq := e.st.Take()
	_ = e.q.Add(queue.Item{Seq: seq, Timestamp: time.Now().Unix(), Kind: "report", Embed: &report})
	e.ui.Notify("Session ended", msgSessionEnded)

	// Reset for a fresh session; NextSeq is never rewound.
	e.st.Updates = nil
	e.st.SessionID++
	e.pend = pendingNone
	e.closed = true
	e.stopAllLocked()
	_ = e.st.Save()

	if e.onEnd != nil {
		go e.onEnd()
	}
}

func (e *Engine) buildReport() discord.Embed {
	fields := make([]discord.Field, 0, len(e.st.Updates))
	for _, u := range e.st.Updates {
		fields = append(fields, discord.Field{
			Name:   fmt.Sprintf("<t:%d:t>", u.Timestamp),
			Value:  emoji(u.Status) + " " + u.Content,
			Inline: false,
		})
	}
	e2 := discord.Embed{
		Title:       "Work Report: " + e.nameLocked(),
		Description: fmt.Sprintf("Session ID: %d • seq %d", e.st.SessionID, e.st.NextSeq),
		Color:       colorGreen,
		Fields:      fields,
		Footer:      &discord.Footer{Text: fmt.Sprintf("Total Updates: %d", len(e.st.Updates))},
		Timestamp:   discord.RFC3339Now(),
	}
	if e.cfg.CommitsFn != nil {
		if commits := e.cfg.CommitsFn(); commits != "" {
			e2.Fields = append(e2.Fields, discord.Field{Name: "GitHub commits", Value: truncate(commits, 1024)})
		}
	}
	return e2
}

// ---- screenshots ----

func (e *Engine) fireShot() {
	e.mu.Lock()
	onBreak := e.st.OnBreak
	closed := e.closed
	fn := e.cfg.CaptureFn
	name := e.nameLocked()
	e.mu.Unlock()

	if !closed && !onBreak && fn != nil {
		shots, err := fn()
		if err != nil {
			log.Printf("capture: %v (treated as unavailable)", err)
		}
		for _, s := range shots {
			e.mu.Lock()
			seq := e.st.Take()
			_ = e.st.Save()
			e.mu.Unlock()
			_ = e.q.Add(queue.Item{
				Seq: seq, Timestamp: time.Now().Unix(), Kind: "screenshot",
				Title:     "Screenshot: " + name,
				Content:   fmt.Sprintf("seq %d", seq),
				ImagePath: s.Path, ImageSHA: s.SHA, Filename: s.Name,
			})
		}
	}

	e.mu.Lock()
	defer e.mu.Unlock()
	if !e.closed && !e.st.OnBreak {
		e.scheduleShotLocked()
	}
}

// ---- posting helpers (call with mu held) ----

func (e *Engine) postUpdate(u state.Update) {
	title := "Update: " + e.nameLocked()
	if u.Kind == "plan" {
		title = "Daily Plan: " + e.nameLocked()
	}
	e.q.Add(queue.Item{
		Seq: u.Seq, Timestamp: u.Timestamp, Kind: u.Kind,
		Title: title, Content: u.Content, Color: colorGreen,
	})
}

func (e *Engine) post(it queue.Item) {
	if it.Seq == 0 {
		it.Seq = e.st.Take()
		_ = e.st.Save()
	}
	if it.Timestamp == 0 {
		it.Timestamp = time.Now().Unix()
	}
	_ = e.q.Add(it)
}

func emoji(status string) string {
	switch status {
	case state.StatusOnTime:
		return "✅"
	case state.StatusLate:
		return "⚠️"
	case state.StatusMissed:
		return "❌"
	default:
		return "❓"
	}
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n-1] + "…"
}
