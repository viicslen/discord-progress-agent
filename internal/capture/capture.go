// Package capture takes screenshots. Failure (no permission, headless, an
// unsupported Wayland compositor) is returned as an error for the caller to
// swallow — it is a normal state, never a crash.
//
// On Linux the display server matters: X11 sessions are grabbed directly
// (kbinani), while Wayland forbids that and must go through the XDG desktop
// portal — see capture_linux.go. All() dispatches via platformCapture.
package capture

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"time"

	"github.com/kbinani/screenshot"

	"discord-tracker-agent/internal/session"
)

// All captures the screen(s) to dir. It is wired into the engine as
// Config.CaptureFn. Filenames are unique on their own (the engine assigns the
// sequence number separately for the embed).
func All(dir string) ([]session.Shot, error) {
	return platformCapture(dir)
}

// captureKbinani grabs every active display directly (X11 on Linux, native on
// macOS/Windows), one Shot per display.
func captureKbinani(dir string) ([]session.Shot, error) {
	stamp := time.Now().UnixNano()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	n := screenshot.NumActiveDisplays()
	if n == 0 {
		return nil, fmt.Errorf("no active displays")
	}
	var shots []session.Shot
	var firstErr error
	for i := 0; i < n; i++ {
		img, err := screenshot.CaptureDisplay(i)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		name := fmt.Sprintf("shot-%d-%d.png", stamp, i)
		path := filepath.Join(dir, name)
		f, err := os.Create(path)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if err := png.Encode(f, img); err != nil {
			f.Close()
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		f.Close()

		sum, err := sha256File(path)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		shots = append(shots, session.Shot{Path: path, SHA: sum, Name: name})
	}
	if len(shots) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return shots, firstErr
}

func sha256File(path string) (string, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:]), nil
}
