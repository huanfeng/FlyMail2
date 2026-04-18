package main

import (
	"bytes"
	"encoding/binary"
	"image"
	"image/color"
	"image/png"
	"math"
)

// generateIcon creates a 64x64 mail envelope icon in ICO format.
// Uses transparent background for proper display on Windows taskbar.
func generateIcon() []byte {
	const size = 64
	img := image.NewRGBA(image.Rect(0, 0, size, size))

	// Colors
	transparent := color.RGBA{A: 0}
	envelopeBg := color.RGBA{R: 255, G: 255, B: 255, A: 255}
	border := color.RGBA{R: 59, G: 130, B: 246, A: 255}   // blue-500
	flapTop := color.RGBA{R: 37, G: 99, B: 235, A: 255}   // blue-600
	shadow := color.RGBA{R: 30, G: 64, B: 175, A: 80}      // subtle shadow
	dot := color.RGBA{R: 239, G: 68, B: 68, A: 255}        // red notification dot

	// Fill transparent
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			img.Set(x, y, transparent)
		}
	}

	// Envelope body: rounded rectangle from (8,18) to (56,50)
	bodyLeft, bodyRight := 8, 56
	bodyTop, bodyBottom := 18, 50
	radius := 3

	for y := bodyTop; y < bodyBottom; y++ {
		for x := bodyLeft; x < bodyRight; x++ {
			if inRoundedRect(x, y, bodyLeft, bodyTop, bodyRight, bodyBottom, radius) {
				img.Set(x, y, envelopeBg)
			}
		}
	}

	// Shadow below envelope
	for y := bodyBottom; y < bodyBottom+2; y++ {
		for x := bodyLeft + 2; x < bodyRight-2; x++ {
			img.Set(x, y, shadow)
		}
	}

	// Border of envelope body
	drawRoundedRectBorder(img, bodyLeft, bodyTop, bodyRight, bodyBottom, radius, border, 2)

	// Flap (V shape from top corners to center)
	midX := (bodyLeft + bodyRight) / 2
	flapDepth := 18 // how far down the V goes

	for t := 0.0; t <= 1.0; t += 0.002 {
		// Left line: (bodyLeft, bodyTop) -> (midX, bodyTop+flapDepth)
		lx := float64(bodyLeft) + t*float64(midX-bodyLeft)
		ly := float64(bodyTop) + t*float64(flapDepth)
		for w := 0; w < 2; w++ {
			img.Set(int(lx)+w, int(ly), flapTop)
			img.Set(int(lx), int(ly)+w, flapTop)
		}

		// Right line: (bodyRight-1, bodyTop) -> (midX, bodyTop+flapDepth)
		rx := float64(bodyRight-1) - t*float64(bodyRight-1-midX)
		ry := float64(bodyTop) + t*float64(flapDepth)
		for w := 0; w < 2; w++ {
			img.Set(int(rx)-w, int(ry), flapTop)
			img.Set(int(rx), int(ry)+w, flapTop)
		}
	}

	// Fill the flap triangle
	for y := bodyTop + 1; y < bodyTop+flapDepth; y++ {
		progress := float64(y-bodyTop) / float64(flapDepth)
		leftEdge := float64(bodyLeft) + progress*float64(midX-bodyLeft)
		rightEdge := float64(bodyRight-1) - progress*float64(bodyRight-1-midX)
		// Gradient from flapTop to slightly lighter
		r := uint8(float64(flapTop.R) + progress*30)
		g := uint8(float64(flapTop.G) + progress*40)
		b := uint8(math.Min(255, float64(flapTop.B)+progress*20))
		fillColor := color.RGBA{R: r, G: g, B: b, A: 255}
		for x := int(leftEdge) + 1; x < int(rightEdge); x++ {
			img.Set(x, y, fillColor)
		}
	}

	// Top border line
	for x := bodyLeft; x < bodyRight; x++ {
		img.Set(x, bodyTop, border)
		img.Set(x, bodyTop+1, border)
	}

	// Notification dot (top-right, for future new-mail indicator)
	dotCx, dotCy, dotR := 50, 14, 6
	for y := dotCy - dotR; y <= dotCy+dotR; y++ {
		for x := dotCx - dotR; x <= dotCx+dotR; x++ {
			dx := float64(x - dotCx)
			dy := float64(y - dotCy)
			if dx*dx+dy*dy <= float64(dotR*dotR) {
				img.Set(x, y, dot)
			}
		}
	}
	// White inner highlight on dot
	for y := dotCy - 2; y <= dotCy; y++ {
		for x := dotCx - 2; x <= dotCx; x++ {
			dx := float64(x - (dotCx - 1))
			dy := float64(y - (dotCy - 1))
			if dx*dx+dy*dy <= 2 {
				img.Set(x, y, color.RGBA{R: 255, G: 150, B: 150, A: 255})
			}
		}
	}

	// Encode to ICO format (Windows prefers ICO over PNG for tray)
	return pngToICO(img, size)
}

// pngToICO wraps a single RGBA image into a minimal ICO file.
func pngToICO(img *image.RGBA, size int) []byte {
	// Encode the image as PNG first
	var pngBuf bytes.Buffer
	png.Encode(&pngBuf, img)
	pngData := pngBuf.Bytes()

	// ICO header: 6 bytes
	// ICO entry: 16 bytes
	// Then PNG data
	var buf bytes.Buffer

	// ICONDIR header
	binary.Write(&buf, binary.LittleEndian, uint16(0))     // reserved
	binary.Write(&buf, binary.LittleEndian, uint16(1))     // type: 1=ICO
	binary.Write(&buf, binary.LittleEndian, uint16(1))     // count: 1 image

	// ICONDIRENTRY
	sz := byte(size)
	if size >= 256 {
		sz = 0 // 0 means 256
	}
	buf.WriteByte(sz)                                              // width
	buf.WriteByte(sz)                                              // height
	buf.WriteByte(0)                                               // color palette
	buf.WriteByte(0)                                               // reserved
	binary.Write(&buf, binary.LittleEndian, uint16(1))            // color planes
	binary.Write(&buf, binary.LittleEndian, uint16(32))           // bits per pixel
	binary.Write(&buf, binary.LittleEndian, uint32(len(pngData))) // data size
	binary.Write(&buf, binary.LittleEndian, uint32(22))           // data offset (6+16=22)

	// PNG data
	buf.Write(pngData)

	return buf.Bytes()
}

func inRoundedRect(x, y, left, top, right, bottom, r int) bool {
	if x < left || x >= right || y < top || y >= bottom {
		return false
	}
	// Check corners
	corners := [][2]int{
		{left + r, top + r},
		{right - r - 1, top + r},
		{left + r, bottom - r - 1},
		{right - r - 1, bottom - r - 1},
	}
	for _, c := range corners {
		dx := x - c[0]
		dy := y - c[1]
		isCornerZone := false
		if x < left+r && y < top+r {
			isCornerZone = true
		} else if x >= right-r && y < top+r {
			isCornerZone = true
		} else if x < left+r && y >= bottom-r {
			isCornerZone = true
		} else if x >= right-r && y >= bottom-r {
			isCornerZone = true
		}
		if isCornerZone && dx*dx+dy*dy > r*r {
			return false
		}
	}
	return true
}

func drawRoundedRectBorder(img *image.RGBA, left, top, right, bottom, r int, c color.RGBA, width int) {
	for w := 0; w < width; w++ {
		// Top & bottom edges
		for x := left + r; x < right-r; x++ {
			img.Set(x, top+w, c)
			img.Set(x, bottom-1-w, c)
		}
		// Left & right edges
		for y := top + r; y < bottom-r; y++ {
			img.Set(left+w, y, c)
			img.Set(right-1-w, y, c)
		}
	}
	// Corner arcs
	for angle := 0.0; angle < math.Pi/2; angle += 0.02 {
		cx := math.Cos(angle)
		cy := math.Sin(angle)
		for w := 0; w < width; w++ {
			rf := float64(r - w)
			// Top-left
			img.Set(left+r-int(cx*rf), top+r-int(cy*rf), c)
			// Top-right
			img.Set(right-r-1+int(cx*rf), top+r-int(cy*rf), c)
			// Bottom-left
			img.Set(left+r-int(cx*rf), bottom-r-1+int(cy*rf), c)
			// Bottom-right
			img.Set(right-r-1+int(cx*rf), bottom-r-1+int(cy*rf), c)
		}
	}
}
