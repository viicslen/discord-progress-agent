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
	want := []string{"checkin", "warning", "missed_late", "missed_inactive", "auto_end", "report"}
	if len(got) != len(want) {
		t.Fatalf("kinds = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("kinds = %v, want %v", got, want)
		}
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
		if k == "missed_late" || k == "missed_inactive" || k == "auto_end" {
			t.Fatalf("answered cycle should not escalate, got kinds %v", kinds(q))
		}
	}
	got := kinds(q)
	if len(got) < 2 || got[1] != "plan" {
		t.Fatalf("expected first answer logged as plan, got %v", got)
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
