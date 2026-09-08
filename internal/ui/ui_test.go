package ui

import (
	"testing"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/driver/desktop"
	"fyne.io/fyne/v2/test"
)

func TestPromptUsesStableWindowTitle(t *testing.T) {
	a := test.NewApp()
	u := New(a, nil)

	u.Prompt("End of day", "Prompt body")
	if got := u.input.Title(); got != "Session Agent" {
		t.Fatalf("prompt window title = %q, want %q", got, "Session Agent")
	}
	if got := u.prompt.Text; got != "End of day\n\nPrompt body" {
		t.Fatalf("prompt text = %q", got)
	}
}

func TestShowSettingsUsesStableWindowTitle(t *testing.T) {
	a := test.NewApp()
	u := New(a, nil)

	u.ShowSettings("alice", "https://initial", nil)
	if got := u.settings.Title(); got != "Session Agent Settings" {
		t.Fatalf("settings window title = %q, want %q", got, "Session Agent Settings")
	}
	if got := u.settingsName.Text; got != "alice" {
		t.Fatalf("settings name = %q", got)
	}
	if got := u.settingsWebhook.Text; got != "https://initial" {
		t.Fatalf("settings webhook = %q", got)
	}

	first := u.settings
	u.ShowSettings("bob", "https://next", nil)
	if u.settings != first {
		t.Fatal("ShowSettings should reuse the same window")
	}
	if got := u.settingsName.Text; got != "bob" {
		t.Fatalf("settings name after reuse = %q", got)
	}
	if got := u.settingsWebhook.Text; got != "https://next" {
		t.Fatalf("settings webhook after reuse = %q", got)
	}
}

func TestCtrlEnterSubmitsUpdate(t *testing.T) {
	a := test.NewApp()
	got := make(chan string, 1)
	u := New(a, func(s string) { got <- s })

	u.entry.SetText("working on the thing")
	u.entry.TypedShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyReturn,
		Modifier: fyne.KeyModifierControl,
	})

	select {
	case s := <-got:
		if s != "working on the thing" {
			t.Fatalf("submitted %q", s)
		}
	default:
		t.Fatal("ctrl+enter did not submit")
	}
	if u.entry.Text != "" {
		t.Fatalf("entry not cleared: %q", u.entry.Text)
	}
}

func TestSubmitDismissesNotifications(t *testing.T) {
	sendFn, closeFn := sendNotification, closeNotification
	defer func() { sendNotification, closeNotification = sendFn, closeFn }()

	var next uint32
	sendNotification = func(fyne.App, string, string) uint32 { next++; return next }
	var closed []uint32
	closeNotification = func(_ fyne.App, id uint32) { closed = append(closed, id) }

	u := New(test.NewApp(), func(string) {})
	u.Notify("Check-in", "body")
	u.Notify("Check-in warning", "body")

	u.entry.TypedShortcut(&desktop.CustomShortcut{
		KeyName:  fyne.KeyReturn,
		Modifier: fyne.KeyModifierControl,
	})

	if len(closed) != 2 || closed[0] != 1 || closed[1] != 2 {
		t.Fatalf("closed = %v, want [1 2]", closed)
	}
	if len(u.open) != 0 {
		t.Fatalf("open notifications left: %v", u.open)
	}
}
