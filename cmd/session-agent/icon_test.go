package main

import (
	"bytes"
	"image/png"
	"testing"
)

func TestAppIconResourceIsPNG(t *testing.T) {
	res := appIconResource()
	if res == nil {
		t.Fatal("appIconResource returned nil")
	}

	img, err := png.Decode(bytes.NewReader(res.Content()))
	if err != nil {
		t.Fatalf("decode png: %v", err)
	}

	b := img.Bounds()
	if b.Dx() != 64 || b.Dy() != 64 {
		t.Fatalf("unexpected icon size: %dx%d", b.Dx(), b.Dy())
	}
}
