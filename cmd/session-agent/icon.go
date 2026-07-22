package main

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"sync"

	"fyne.io/fyne/v2"
)

var (
	iconOnce     sync.Once
	iconResource fyne.Resource
)

func appIconResource() fyne.Resource {
	iconOnce.Do(func() {
		const size = 64

		img := image.NewRGBA(image.Rect(0, 0, size, size))
		bg := color.RGBA{R: 88, G: 101, B: 242, A: 255}
		fg := color.RGBA{R: 255, G: 255, B: 255, A: 255}

		fillRoundedRect(img, bg, 12)
		drawRing(img, fg, 32, 32, 17, 21)
		drawLine(img, fg, 32, 32, 32, 21, 2)
		drawLine(img, fg, 32, 32, 42, 37, 2)
		drawDisc(img, fg, 32, 32, 4)

		var buf bytes.Buffer
		if err := png.Encode(&buf, img); err != nil {
			panic(err)
		}

		iconResource = fyne.NewStaticResource("session-agent.png", buf.Bytes())
	})

	return iconResource
}

func fillRoundedRect(img *image.RGBA, c color.Color, radius int) {
	b := img.Bounds()
	maxX := b.Max.X - 1
	maxY := b.Max.Y - 1

	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if insideRoundedRect(x, y, maxX, maxY, radius) {
				img.Set(x, y, c)
			}
		}
	}
}

func insideRoundedRect(x, y, maxX, maxY, radius int) bool {
	if x >= radius && x <= maxX-radius {
		return true
	}
	if y >= radius && y <= maxY-radius {
		return true
	}

	switch {
	case x < radius && y < radius:
		return withinCircle(x, y, radius, radius, radius)
	case x > maxX-radius && y < radius:
		return withinCircle(x, y, maxX-radius, radius, radius)
	case x < radius && y > maxY-radius:
		return withinCircle(x, y, radius, maxY-radius, radius)
	case x > maxX-radius && y > maxY-radius:
		return withinCircle(x, y, maxX-radius, maxY-radius, radius)
	default:
		return false
	}
}

func drawRing(img *image.RGBA, c color.Color, cx, cy, inner, outer int) {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			dx := x - cx
			dy := y - cy
			d2 := dx*dx + dy*dy
			if d2 >= inner*inner && d2 <= outer*outer {
				img.Set(x, y, c)
			}
		}
	}
}

func drawDisc(img *image.RGBA, c color.Color, cx, cy, radius int) {
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if withinCircle(x, y, cx, cy, radius) {
				img.Set(x, y, c)
			}
		}
	}
}

func drawLine(img *image.RGBA, c color.Color, x0, y0, x1, y1, thickness int) {
	dx := x1 - x0
	dy := y1 - y0
	steps := max(abs(dx), abs(dy))
	if steps == 0 {
		drawDisc(img, c, x0, y0, thickness)
		return
	}

	for i := 0; i <= steps; i++ {
		x := x0 + dx*i/steps
		y := y0 + dy*i/steps
		drawDisc(img, c, x, y, thickness)
	}
}

func withinCircle(x, y, cx, cy, radius int) bool {
	dx := x - cx
	dy := y - cy
	return dx*dx+dy*dy <= radius*radius
}

func abs(n int) int {
	if n < 0 {
		return -n
	}
	return n
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}
