// Package ui is the Fyne front-end: the first-run consent window, a reusable
// input window for updates, and native notifications. It implements the
// session.UI interface. All cross-goroutine UI work goes through fyne.Do.
package ui

import (
	"fmt"

	"fyne.io/fyne/v2"
	"fyne.io/fyne/v2/container"
	"fyne.io/fyne/v2/widget"
)

const consentNotice = `This computer runs a work-session tracker for %s.

While a session is active it will:
  • ask you for periodic status updates,
  • take occasional screenshots of your screen,
  • send these to your team's Discord.

Screenshots pause while you are on a break. Nothing is captured or sent unless
you agree below. If you do not consent, the app will close and record nothing.`

type UI struct {
	App             fyne.App
	input           fyne.Window
	entry           *widget.Entry
	prompt          *widget.Label
	settings        fyne.Window
	settingsName    *widget.Entry
	settingsWebhook *widget.Entry
	onSubmit        func(string)
}

// New builds the UI and its (hidden) input window. onSubmit is invoked with the
// text whenever the worker submits an update.
func New(app fyne.App, onSubmit func(string)) *UI {
	u := &UI{App: app, onSubmit: onSubmit}

	u.entry = widget.NewMultiLineEntry()
	u.entry.SetPlaceHolder("What are you working on?")
	u.prompt = widget.NewLabel("")
	u.prompt.Wrapping = fyne.TextWrapWord

	w := app.NewWindow("Session Agent")
	send := widget.NewButton("Send", func() {
		text := u.entry.Text
		u.entry.SetText("")
		w.Hide()
		if u.onSubmit != nil {
			u.onSubmit(text)
		}
	})
	w.SetContent(container.NewBorder(u.prompt, send, nil, nil, u.entry))
	w.Resize(fyne.NewSize(420, 220))
	w.SetCloseIntercept(func() { w.Hide() }) // closing hides, never quits
	u.input = w

	settingsTitle := widget.NewLabel("Settings")
	settingsTitle.Wrapping = fyne.TextWrapWord
	nameLabel := widget.NewLabel("Worker name")
	webhookLabel := widget.NewLabel("Discord webhook URL")
	u.settingsName = widget.NewEntry()
	u.settingsWebhook = widget.NewEntry()
	saveSettings := widget.NewButton("Save", func() {})

	u.settings = app.NewWindow("Session Agent Settings")
	u.settings.SetContent(container.NewVBox(
		settingsTitle,
		nameLabel,
		u.settingsName,
		webhookLabel,
		u.settingsWebhook,
		saveSettings,
	))
	u.settings.Resize(fyne.NewSize(520, 240))
	u.settings.SetCloseIntercept(func() { u.settings.Hide() })
	return u
}

// Notify sends a native OS notification (safe from any goroutine).
func (u *UI) Notify(title, body string) {
	u.App.SendNotification(fyne.NewNotification(title, body))
}

// Prompt raises the input window with the given prompt text.
func (u *UI) Prompt(title, body string) {
	fyne.Do(func() {
		u.input.SetTitle("Session Agent")
		u.prompt.SetText(title + "\n\n" + body)
		u.input.Show()
		u.input.RequestFocus()
	})
}

// ShowSettings opens a reusable settings window for the runtime worker name and
// webhook URL.
func (u *UI) ShowSettings(name, webhook string, onSubmit func(name, webhook string)) {
	fyne.Do(func() {
		u.settingsName.SetText(name)
		u.settingsWebhook.SetText(webhook)
		for _, obj := range u.settings.Content().(*fyne.Container).Objects {
			if btn, ok := obj.(*widget.Button); ok {
				btn.OnTapped = func() {
					u.settings.Hide()
					if onSubmit != nil {
						onSubmit(u.settingsName.Text, u.settingsWebhook.Text)
					}
				}
			}
		}
		u.settings.Show()
		u.settings.RequestFocus()
	})
}

// ShowConsent displays the first-run consent gate. onAccept runs on "I Agree";
// "Decline" (or closing) quits the app so nothing is ever captured.
func (u *UI) ShowConsent(workerName string, onAccept func()) {
	w := u.App.NewWindow("Session Agent Consent")
	notice := widget.NewLabel("")
	notice.Wrapping = fyne.TextWrapWord
	notice.SetText(fmt.Sprintf(consentNotice, workerName))

	agree := widget.NewButton("I Agree", func() {
		w.Close()
		onAccept()
	})
	decline := widget.NewButton("Decline", func() { u.App.Quit() })
	buttons := container.NewGridWithColumns(2, decline, agree)

	w.SetContent(container.NewBorder(nil, buttons, nil, nil, notice))
	w.Resize(fyne.NewSize(460, 320))
	w.SetCloseIntercept(func() { u.App.Quit() }) // closing = declining
	w.Show()
}
