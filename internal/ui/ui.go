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
	App      fyne.App
	input    fyne.Window
	entry    *widget.Entry
	prompt   *widget.Label
	onSubmit func(string)
}

// New builds the UI and its (hidden) input window. onSubmit is invoked with the
// text whenever the worker submits an update.
func New(app fyne.App, onSubmit func(string)) *UI {
	u := &UI{App: app, onSubmit: onSubmit}

	u.entry = widget.NewMultiLineEntry()
	u.entry.SetPlaceHolder("What are you working on?")
	u.prompt = widget.NewLabel("")
	u.prompt.Wrapping = fyne.TextWrapWord

	w := app.NewWindow("Status update")
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
	return u
}

// Notify sends a native OS notification (safe from any goroutine).
func (u *UI) Notify(title, body string) {
	u.App.SendNotification(fyne.NewNotification(title, body))
}

// Prompt raises the input window with the given prompt text.
func (u *UI) Prompt(title, body string) {
	fyne.Do(func() {
		u.input.SetTitle(title)
		u.prompt.SetText(body)
		u.input.Show()
		u.input.RequestFocus()
	})
}

// ShowConsent displays the first-run consent gate. onAccept runs on "I Agree";
// "Decline" (or closing) quits the app so nothing is ever captured.
func (u *UI) ShowConsent(workerName string, onAccept func()) {
	w := u.App.NewWindow("Consent required")
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
