package session

import (
	"path/filepath"
	"testing"
	"time"

	"discord-tracker-agent/internal/queue"
	"discord-tracker-agent/internal/state"
	"discord-tracker-agent/internal/vault"
)

func init() {
	k := make([]byte, 32)
	for i := range k {
		k[i] = byte(i + 3)
	}
	_ = vault.Init(k)
}

type fakeUI struct{}

func (fakeUI) Notify(_, _ string) {}
func (fakeUI) Prompt(_, _ string) {}

func newEngine(t *testing.T, cfg Config, onEnd func()) (*Engine, *queue.Queue, *state.State) {
	t.Helper()
	dir := t.TempDir()
	st, err := state.Load(filepath.Join(dir, "state.dat"))
	if err != nil {
		t.Fatal(err)
	}
	q, err := queue.Load(filepath.Join(dir, "queue.dat"))
	if err != nil {
		t.Fatal(err)
	}
	cfg.WorkerName = "tester"
	return New(cfg, fakeUI{}, st, q, onEnd), q, st
}

func kinds(q *queue.Queue) []string {
	var out []string
	for _, it := range q.Items {
		out = append(out, it.Kind)
	}
	return out
}

func TestEscalationToAutoEnd(t *testing.T) {
	done := make(chan struct{}, 1)
	cfg := Config{
		CheckInBase:       10 * time.Second, // large: no second check-in during the test
		LateTimeout:       40 * time.Millisecond,
		WarningBefore:     15 * time.Millisecond,
		InactiveTO:        20 * time.Millisecond,
		InactiveThreshold: 1,
		AutoEndThreshold:  1,
	}
	e, q, _ := newEngine(t, cfg, func() { done <- struct{}{} })

	e.fireCheckIn() // start one cycle manually; never answer it

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("auto-end never fired")
	}
	time.Sleep(20 * time.Millisecond)

	got := kinds(q)
	want := []string{"warning", "missed_late", "missed_inactive", "auto_end", "report"}
	if len(got) != len(want) {
		t.Fatalf("kinds = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("kinds = %v, want %v", got, want)
		}
	}
}

// Auto-end takes a final screenshot too.
func TestAutoEndCaptures(t *testing.T) {
	shot := make(chan struct{}, 1)
	cfg := Config{
		CheckInBase:       10 * time.Second,
		LateTimeout:       40 * time.Millisecond,
		WarningBefore:     15 * time.Millisecond,
		InactiveTO:        20 * time.Millisecond,
		InactiveThreshold: 1,
		AutoEndThreshold:  1,
		CaptureFn: func() ([]Shot, error) {
			shot <- struct{}{}
			return nil, nil
		},
	}
	e, _, _ := newEngine(t, cfg, func() {})

	e.fireCheckIn() // never answered → escalates to auto-end
	select {
	case <-shot:
	case <-time.After(2 * time.Second):
		t.Fatal("auto-end should take a final screenshot")
	}
}

func TestAnswerResetsMissed(t *testing.T) {
	cfg := Config{
		CheckInBase:       10 * time.Second,
		LateTimeout:       60 * time.Millisecond,
		WarningBefore:     20 * time.Millisecond,
		InactiveThreshold: 1,
		AutoEndThreshold:  1,
	}
	e, q, _ := newEngine(t, cfg, func() {})

	e.fireCheckIn()
	e.Submit("working on the parser") // answer well before the late deadline
	time.Sleep(120 * time.Millisecond)

	for _, k := range kinds(q) {
		if k == "checkin" || k == "missed_late" || k == "missed_inactive" || k == "auto_end" {
			t.Fatalf("answered cycle should not escalate, got kinds %v", kinds(q))
		}
	}
	got := kinds(q)
	if len(got) != 1 || got[0] != "plan" {
		t.Fatalf("expected first answer logged as plan, got %v", got)
	}
}

func TestWebhookRuntimeConfigPersists(t *testing.T) {
	dir := t.TempDir()
	sp := filepath.Join(dir, "state.dat")
	st, err := state.Load(sp)
	if err != nil {
		t.Fatal(err)
	}
	q, err := queue.Load(filepath.Join(dir, "queue.dat"))
	if err != nil {
		t.Fatal(err)
	}
	e := New(Config{WorkerName: "t"}, fakeUI{}, st, q, func() {})

	if got := e.Webhook(); got != "" {
		t.Fatalf("expected no webhook before configuration, got %q", got)
	}
	e.SetWebhook("https://configured")
	if got := e.Webhook(); got != "https://configured" {
		t.Fatalf("runtime webhook not applied, got %q", got)
	}

	reloaded, err := state.Load(sp)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.WebhookURL != "https://configured" {
		t.Fatalf("runtime webhook did not persist, got %q", reloaded.WebhookURL)
	}
}

func TestBreakPausesCapture(t *testing.T) {
	var captures int
	cfg := Config{
		CheckInBase: 10 * time.Second,
		ShotBase:    10 * time.Second,
		BreakAlert:  10 * time.Second,
		CaptureFn: func() ([]Shot, error) {
			captures++
			return nil, nil
		},
	}
	e, _, st := newEngine(t, cfg, func() {})

	e.StartBreak()
	if !st.OnBreak {
		t.Fatal("StartBreak should set OnBreak")
	}
	e.fireShot() // should skip capture while on break
	if captures != 0 {
		t.Fatalf("capture ran during break: %d", captures)
	}

	e.EndBreak()
	e.fireShot() // now capture should run
	if captures != 1 {
		t.Fatalf("capture should run after break ended: %d", captures)
	}
}

// EndSession takes a final screenshot of the working state.
func TestEndSessionCaptures(t *testing.T) {
	done := make(chan struct{}, 1)
	cfg := Config{
		CheckInBase: 10 * time.Second,
		ShotBase:    10 * time.Second,
		EODTimeout:  10 * time.Second,
		CaptureFn: func() ([]Shot, error) {
			done <- struct{}{}
			return nil, nil
		},
	}
	e, _, _ := newEngine(t, cfg, func() {})

	e.EndSession() // async final capture
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("EndSession should take a final screenshot")
	}
}

func TestStartSessionReopensFinalizedEngine(t *testing.T) {
	done := make(chan struct{}, 1)
	// StartSession fires an immediate check-in; large timeouts keep its
	// escalation (late/inactive) from posting before Submit answers it.
	cfg := Config{CheckInBase: 10 * time.Second, ShotBase: 10 * time.Second, LateTimeout: 10 * time.Second}
	e, q, st := newEngine(t, cfg, func() { done <- struct{}{} })

	e.missed = 3
	e.mu.Lock()
	e.finalizeLocked()
	e.mu.Unlock()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("finalize should invoke onEnd")
	}

	if !e.closed {
		t.Fatal("engine should be closed after finalize")
	}
	if st.SessionID != 2 {
		t.Fatalf("session id = %d, want 2", st.SessionID)
	}
	if len(st.Updates) != 0 {
		t.Fatalf("updates should be cleared after finalize, got %d", len(st.Updates))
	}

	if !e.StartSession() {
		t.Fatal("StartSession should reopen a finalized engine")
	}
	if e.closed {
		t.Fatal("engine should be open after StartSession")
	}
	if e.missed != 0 {
		t.Fatalf("missed should reset for the new session, got %d", e.missed)
	}
	if e.StartSession() {
		t.Fatal("StartSession should fail while a session is already active")
	}

	e.Submit("back to work")
	got := kinds(q)
	if len(got) != 2 || got[1] != "plan" {
		t.Fatalf("expected report then new session plan, got %v", got)
	}
}

// Ending a session while on a break must not leave OnBreak persisted, or the
// next launch resumes a phantom break with no check-ins/screenshots.
func TestFinalizeClearsBreak(t *testing.T) {
	cfg := Config{CheckInBase: 10 * time.Second, ShotBase: 10 * time.Second, BreakAlert: 10 * time.Second}
	e, _, st := newEngine(t, cfg, func() {})

	e.StartBreak()
	if !st.OnBreak {
		t.Fatal("break should be active")
	}

	e.mu.Lock()
	e.finalizeLocked()
	e.mu.Unlock()

	if st.OnBreak || st.BreakStart != 0 {
		t.Fatalf("break state should be cleared after finalize: OnBreak=%v BreakStart=%d", st.OnBreak, st.BreakStart)
	}
}
