//go:build !linux

package ui

import "fyne.io/fyne/v2"

// ponytail: macOS/Windows expose no dismiss API through Fyne, so notifications
// there just expire on their own.
var sendNotification = func(app fyne.App, title, body string) uint32 {
	app.SendNotification(fyne.NewNotification(title, body))
	return 0
}

var closeNotification = func(fyne.App, uint32) {}
