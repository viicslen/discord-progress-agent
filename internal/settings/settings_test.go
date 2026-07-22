package settings

import (
	"testing"
	"time"
)

func TestCountFallback(t *testing.T) {
	if got := count("", 42); got != 42 {
		t.Fatalf("empty should fall back to default: got %d", got)
	}
	if got := count("not-a-number", 7); got != 7 {
		t.Fatalf("bad int should fall back (not crash): got %d", got)
	}
	if got := count("15", 0); got != 15 {
		t.Fatalf("valid int should parse: got %d", got)
	}
}

func TestDefaultsParsed(t *testing.T) {
	if CheckInBase != 60*time.Minute {
		t.Fatalf("CheckInBase default = %v", CheckInBase)
	}
	if InactiveThresholdN != 2 || AutoEndThresholdN != 3 {
		t.Fatalf("threshold defaults = %d, %d", InactiveThresholdN, AutoEndThresholdN)
	}
}
