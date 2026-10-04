// Package icon draws Ghostline's ring icon (tray states and app icon).
package icon

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"math"
)

// Ring returns a size×size PNG: a transparent background, a ring of colour
// c (thickness size/10) and a centre dot.
func Ring(c color.RGBA, size int) []byte {
	img := image.NewRGBA(image.Rect(0, 0, size, size))
	cx, cy := float64(size)/2, float64(size)/2
	outer := float64(size)/2 - 0.5
	thick := math.Max(1, float64(size)/10)
	dot := float64(size) / 7
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			d := math.Hypot(float64(x)+0.5-cx, float64(y)+0.5-cy)
			if (d <= outer && d >= outer-thick) || d <= dot {
				img.SetRGBA(x, y, c)
			}
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}
