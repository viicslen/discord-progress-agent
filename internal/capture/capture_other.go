//go:build !linux

package capture

import "discord-tracker-agent/internal/session"

// platformCapture on macOS/Windows: kbinani grabs the native display(s).
func platformCapture(dir string) ([]session.Shot, error) {
	return captureKbinani(dir)
}
