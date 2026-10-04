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
