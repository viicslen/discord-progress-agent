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
	App        fyne.App
	input      fyne.Window
	entry      *widget.Entry
	prompt     *widget.Label
	form       fyne.Window
	formBody   *widget.Label
	formEntry  *widget.Entry
	formSave   *widget.Button
	formSubmit func(string)
	onSubmit   func(string)
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

	u.formBody = widget.NewLabel("")
	u.formBody.Wrapping = fyne.TextWrapWord
	u.formEntry = widget.NewEntry()
	u.formSave = widget.NewButton("Save", func() {
		text := u.formEntry.Text
		u.formEntry.SetText("")
		u.form.Hide()
		if u.formSubmit != nil {
			u.formSubmit(text)
		}
	})

	u.form = app.NewWindow("Session Agent Settings")
	u.form.SetContent(container.NewBorder(u.formBody, u.formSave, nil, nil, u.formEntry))
	u.form.Resize(fyne.NewSize(480, 180))
	u.form.SetCloseIntercept(func() { u.form.Hide() })
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

// ShowForm opens a one-off input window (its own callback), separate from the
// status-update window so its submission is not logged as an update. Used for
// the runtime webhook editor.
func (u *UI) ShowForm(title, prompt, initial string, onSubmit func(string)) {
	fyne.Do(func() {
		u.form.SetTitle("Session Agent Settings")
		u.formBody.SetText(title + "\n\n" + prompt)
		u.formEntry.SetText(initial)
		u.formSubmit = onSubmit
		u.form.Show()
		u.form.RequestFocus()
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
