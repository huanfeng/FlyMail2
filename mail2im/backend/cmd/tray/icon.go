package main

// iconData is a minimal 16x16 mail icon (PNG).
// Generated as a simple envelope shape. Replace with a proper icon file later.
//
// To use a custom icon, replace this with:
//
//	//go:embed icon.ico
//	var iconData []byte
//
// For now, we use a programmatically generated PNG.

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
)

func generateIcon() []byte {
	const size = 32
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	bg := color.RGBA{R: 59, G: 130, B: 246, A: 255}   // blue
	fg := color.RGBA{R: 255, G: 255, B: 255, A: 255}   // white
	edge := color.RGBA{R: 37, G: 99, B: 235, A: 255}   // darker blue

	// Fill background (rounded feel via full fill)
	for y := 2; y < size-2; y++ {
		for x := 2; x < size-2; x++ {
			img.Set(x, y, bg)
		}
	}

	// Envelope body (white rectangle)
	for y := 10; y < 24; y++ {
		for x := 5; x < 27; x++ {
			img.Set(x, y, fg)
		}
	}

	// Envelope flap (V shape)
	for i := 0; i < 11; i++ {
		// Left diagonal
		img.Set(5+i, 10+i, edge)
		img.Set(6+i, 10+i, edge)
		// Right diagonal
		img.Set(26-i, 10+i, edge)
		img.Set(25-i, 10+i, edge)
	}

	// Top edge
	for x := 5; x < 27; x++ {
		img.Set(x, 10, edge)
	}
	// Bottom edge
	for x := 5; x < 27; x++ {
		img.Set(x, 23, edge)
	}
	// Left edge
	for y := 10; y < 24; y++ {
		img.Set(5, y, edge)
	}
	// Right edge
	for y := 10; y < 24; y++ {
		img.Set(26, y, edge)
	}

	var buf bytes.Buffer
	png.Encode(&buf, img)
	return buf.Bytes()
}
