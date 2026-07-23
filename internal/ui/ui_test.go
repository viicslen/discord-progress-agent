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

func TestShowFormReusesStableWindowTitle(t *testing.T) {
	a := test.NewApp()
	u := New(a, nil)

	u.ShowForm("Change webhook", "New Discord webhook URL:", "https://initial", nil)
	if got := u.form.Title(); got != "Session Agent Settings" {
		t.Fatalf("form window title = %q, want %q", got, "Session Agent Settings")
	}
	if got := u.formBody.Text; got != "Change webhook\n\nNew Discord webhook URL:" {
		t.Fatalf("form body = %q", got)
	}
	if got := u.formEntry.Text; got != "https://initial" {
		t.Fatalf("form entry = %q", got)
	}

	first := u.form
	u.ShowForm("Change name", "Worker name shown in Discord:", "alice", nil)
	if u.form != first {
		t.Fatal("ShowForm should reuse the same window")
	}
	if got := u.formBody.Text; got != "Change name\n\nWorker name shown in Discord:" {
		t.Fatalf("form body after reuse = %q", got)
	}
	if got := u.formEntry.Text; got != "alice" {
		t.Fatalf("form entry after reuse = %q", got)
	}
}
