package ui

import (
	"testing"

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
