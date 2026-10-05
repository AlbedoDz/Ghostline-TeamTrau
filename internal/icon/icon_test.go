package icon_test

import (
	"bytes"
	"image/color"
	"image/png"
	"testing"

	"github.com/hashcott/ghostline/internal/icon"
	"github.com/stretchr/testify/require"
)

func TestRing(t *testing.T) {
	c := color.RGBA{0x00, 0xff, 0xa3, 0xff}
	img, err := png.Decode(bytes.NewReader(icon.Ring(c, 32)))
	require.NoError(t, err)
	require.Equal(t, 32, img.Bounds().Dx())
	require.Equal(t, 32, img.Bounds().Dy())
	r, g, b, a := img.At(16, 1).RGBA()
	require.Equal(t, [4]uint32{0x0000, 0xffff, 0xa3a3, 0xffff}, [4]uint32{r, g, b, a})
	_, _, _, a = img.At(0, 0).RGBA()
	require.Zero(t, a)
	r, g, b, a = img.At(16, 16).RGBA()
	require.Equal(t, [4]uint32{0x0000, 0xffff, 0xa3a3, 0xffff}, [4]uint32{r, g, b, a}, "center dot")
}

// The ring edge is anti-aliased: pixels on the boundary are partly
// transparent instead of all-or-nothing (jagged when Windows shows it).
func TestRing_AntiAliased(t *testing.T) {
	img, err := png.Decode(bytes.NewReader(icon.Ring(color.RGBA{0x00, 0xff, 0xa3, 0xff}, 20)))
	require.NoError(t, err)
	partial := 0
	for y := 0; y < 20; y++ {
		for x := 0; x < 20; x++ {
			if _, _, _, a := img.At(x, y).RGBA(); a > 0 && a < 0xffff {
				partial++
			}
		}
	}
	require.Greater(t, partial, 20)
}
