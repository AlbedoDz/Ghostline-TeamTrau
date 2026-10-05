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
// c (thickness size/10) and a centre dot, anti-aliased by 4×4 supersampling
// so the edges stay smooth at tray sizes.
func Ring(c color.RGBA, size int) []byte {
	img := image.NewNRGBA(image.Rect(0, 0, size, size))
	cx, cy := float64(size)/2, float64(size)/2
	outer := float64(size)/2 - 0.5
	thick := math.Max(1, float64(size)/10)
	dot := float64(size) / 7
	const ss = 4
	for y := 0; y < size; y++ {
		for x := 0; x < size; x++ {
			hit := 0
			for sy := 0; sy < ss; sy++ {
				for sx := 0; sx < ss; sx++ {
					d := math.Hypot(float64(x)+(float64(sx)+0.5)/ss-cx, float64(y)+(float64(sy)+0.5)/ss-cy)
					if (d <= outer && d >= outer-thick) || d <= dot {
						hit++
					}
				}
			}
			if hit > 0 {
				img.SetNRGBA(x, y, color.NRGBA{c.R, c.G, c.B, uint8(int(c.A) * hit / (ss * ss))})
			}
		}
	}
	var buf bytes.Buffer
	_ = png.Encode(&buf, img)
	return buf.Bytes()
}
