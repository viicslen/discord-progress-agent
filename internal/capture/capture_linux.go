//go:build linux

package capture

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"

	"discord-tracker-agent/internal/session"
)

// platformCapture on Linux: Wayland forbids direct screen grabs, so use the XDG
// desktop portal there; on X11 fall through to the direct kbinani grab. Under
// Wayland we do NOT fall back to X11 — an X11 grab there just fails noisily and
// never sees Wayland surfaces.
func platformCapture(dir string) ([]session.Shot, error) {
	if isWayland() {
		return capturePortal(dir)
	}
	return captureKbinani(dir)
}

func isWayland() bool {
	return os.Getenv("WAYLAND_DISPLAY") != "" ||
		strings.EqualFold(os.Getenv("XDG_SESSION_TYPE"), "wayland")
}

// capturePortal asks the compositor for a screenshot via
// org.freedesktop.portal.Screenshot (interactive=false, so no per-shot prompt
// on GNOME/KDE/wlroots once permitted). It follows the standard portal
// Request/Response pattern: subscribe to the predicted request path, call, wait.
func capturePortal(dir string) ([]session.Shot, error) {
	conn, err := dbus.ConnectSessionBus()
	if err != nil {
		return nil, fmt.Errorf("session bus: %w", err)
	}
	defer conn.Close()

	names := conn.Names()
	if len(names) == 0 {
		return nil, errors.New("no dbus unique name")
	}
	sender := strings.ReplaceAll(strings.TrimPrefix(names[0], ":"), ".", "_")
	token := fmt.Sprintf("sessionagent%d", time.Now().UnixNano())
	reqPath := dbus.ObjectPath("/org/freedesktop/portal/desktop/request/" + sender + "/" + token)

	// Subscribe before calling to avoid missing a fast Response.
	if err := conn.AddMatchSignal(
		dbus.WithMatchObjectPath(reqPath),
		dbus.WithMatchInterface("org.freedesktop.portal.Request"),
		dbus.WithMatchMember("Response"),
	); err != nil {
		return nil, fmt.Errorf("add match: %w", err)
	}
	sigCh := make(chan *dbus.Signal, 1)
	conn.Signal(sigCh)

	obj := conn.Object("org.freedesktop.portal.Desktop", "/org/freedesktop/portal/desktop")
	options := map[string]dbus.Variant{
		"handle_token": dbus.MakeVariant(token),
		"interactive":  dbus.MakeVariant(false),
	}
	if call := obj.Call("org.freedesktop.portal.Screenshot.Screenshot", 0, "", options); call.Err != nil {
		return nil, fmt.Errorf("portal Screenshot: %w", call.Err)
	}

	select {
	case sig := <-sigCh:
		if len(sig.Body) < 2 {
			return nil, errors.New("malformed portal response")
		}
		code, _ := sig.Body[0].(uint32)
		if code != 0 {
			return nil, fmt.Errorf("portal screenshot not granted (response %d)", code)
		}
		results, _ := sig.Body[1].(map[string]dbus.Variant)
		uriVar, ok := results["uri"]
		if !ok {
			return nil, errors.New("portal response missing uri")
		}
		uri, _ := uriVar.Value().(string)
		return saveFromURI(uri, dir)
	case <-time.After(20 * time.Second):
		return nil, errors.New("portal screenshot timed out")
	}
}

// saveFromURI copies the portal's temp PNG into dir under our naming, hashes it,
// and removes the portal's copy. The portal returns one image for the whole
// screen, so this yields a single Shot.
func saveFromURI(uri, dir string) ([]session.Shot, error) {
	u, err := url.Parse(uri)
	if err != nil {
		return nil, fmt.Errorf("bad uri %q: %w", uri, err)
	}
	data, err := os.ReadFile(u.Path)
	if err != nil {
		return nil, fmt.Errorf("read portal screenshot: %w", err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	name := fmt.Sprintf("shot-%d-0.png", time.Now().UnixNano())
	dst := filepath.Join(dir, name)
	if err := os.WriteFile(dst, data, 0o600); err != nil {
		return nil, err
	}
	_ = os.Remove(u.Path) // best-effort cleanup of the portal's temp file

	sum := sha256.Sum256(data)
	return []session.Shot{{Path: dst, SHA: hex.EncodeToString(sum[:]), Name: name}}, nil
}
