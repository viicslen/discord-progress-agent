package capture

import (
	"image"
	"image/color"
	"os"
	"testing"
)

func TestCaptureKbinaniUsesDisplayBounds(t *testing.T) {
	origNum := numActiveDisplays
	origBounds := getDisplayBounds
	origCapture := captureRect
	defer func() {
		numActiveDisplays = origNum
		getDisplayBounds = origBounds
		captureRect = origCapture
	}()

	bounds := []image.Rectangle{
		image.Rect(100, 200, 300, 500),
		image.Rect(-400, 50, -100, 250),
	}
	var seen []image.Rectangle

	numActiveDisplays = func() int { return len(bounds) }
	getDisplayBounds = func(i int) image.Rectangle { return bounds[i] }
	captureRect = func(r image.Rectangle) (*image.RGBA, error) {
		seen = append(seen, r)
		img := image.NewRGBA(image.Rect(0, 0, r.Dx(), r.Dy()))
		img.Set(0, 0, color.RGBA{R: uint8(len(seen)), A: 255})
		return img, nil
	}

	dir := t.TempDir()
	shots, err := captureKbinani(dir)
	if err != nil {
		t.Fatalf("captureKbinani: %v", err)
	}
	if len(shots) != 2 {
		t.Fatalf("expected 2 shots, got %d", len(shots))
	}
	if len(seen) != len(bounds) {
		t.Fatalf("expected %d capture calls, got %d", len(bounds), len(seen))
	}
	for i := range bounds {
		if seen[i] != bounds[i] {
			t.Fatalf("captureRect call %d used bounds %v, want %v", i, seen[i], bounds[i])
		}
		if _, err := os.Stat(shots[i].Path); err != nil {
			t.Fatalf("shot %d not written: %v", i, err)
		}
		if shots[i].SHA == "" {
			t.Fatalf("shot %d missing sha", i)
		}
	}
}

func TestCaptureKbinaniSkipsEmptyBounds(t *testing.T) {
	origNum := numActiveDisplays
	origBounds := getDisplayBounds
	origCapture := captureRect
	defer func() {
		numActiveDisplays = origNum
		getDisplayBounds = origBounds
		captureRect = origCapture
	}()

	numActiveDisplays = func() int { return 1 }
	getDisplayBounds = func(int) image.Rectangle { return image.Rectangle{} }
	captureRect = func(r image.Rectangle) (*image.RGBA, error) {
		t.Fatalf("captureRect should not be called for empty bounds: %v", r)
		return nil, nil
	}

	if _, err := captureKbinani(t.TempDir()); err == nil {
		t.Fatal("expected error for empty display bounds")
	}
}
