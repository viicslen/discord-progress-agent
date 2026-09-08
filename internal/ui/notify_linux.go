//go:build linux

package ui

import (
	"github.com/godbus/dbus/v5"

	"fyne.io/fyne/v2"
)

const (
	notifyDest = "org.freedesktop.Notifications"
	notifyPath = "/org/freedesktop/Notifications"
)

// sendNotification posts through the freedesktop notification service directly
// rather than fyne.App.SendNotification, which throws away the server-assigned
// id — we need that id to dismiss the notification once the worker answers.
// ponytail: falls back to Fyne (id 0, not dismissable) if the bus is missing.
var sendNotification = func(app fyne.App, title, body string) uint32 {
	conn, err := dbus.SessionBus() // shared connection, don't close
	if err == nil {
		var id uint32
		call := conn.Object(notifyDest, notifyPath).Call(notifyDest+".Notify", 0,
			app.UniqueID(), uint32(0), "", title, body,
			[]string{}, map[string]dbus.Variant{}, int32(0))
		if err := call.Store(&id); err == nil {
			return id
		}
	}
	app.SendNotification(fyne.NewNotification(title, body))
	return 0
}

var closeNotification = func(_ fyne.App, id uint32) {
	conn, err := dbus.SessionBus()
	if err != nil {
		return
	}
	conn.Object(notifyDest, notifyPath).Call(notifyDest+".CloseNotification", 0, id)
}
