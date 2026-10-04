package qr_test

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/hashcott/ghostline/internal/qr"
	"github.com/makiuchi-d/gozxing"
	gzqr "github.com/makiuchi-d/gozxing/qrcode"
	"github.com/stretchr/testify/require"
)

func render(m [][]bool) image.Image {
	const scale, quiet = 4, 4
	n := (len(m) + 2*quiet) * scale
	img := image.NewGray(image.Rect(0, 0, n, n))
	for y := 0; y < n; y++ {
		for x := 0; x < n; x++ {
			img.SetGray(x, y, color.Gray{Y: 255})
		}
	}
	for r, row := range m {
		for c, dark := range row {
			if !dark {
				continue
			}
			for dy := 0; dy < scale; dy++ {
				for dx := 0; dx < scale; dx++ {
					img.SetGray((c+quiet)*scale+dx, (r+quiet)*scale+dy, color.Gray{Y: 0})
				}
			}
		}
	}
	return img
}

func decode(t *testing.T, m [][]bool) string {
	t.Helper()
	bmp, err := gozxing.NewBinaryBitmapFromImage(render(m))
	require.NoError(t, err)
	res, err := gzqr.NewQRCodeReader().Decode(bmp, nil)
	require.NoError(t, err)
	return res.GetText()
}

func TestEncode_DecodesBack(t *testing.T) {
	for _, s := range []string{
		"192.168.1.5:8080",
		"[fd00::1234]:8080",
		"socks5://192.168.100.200:65535",
		"http://192.168.1.5:8080 socks5://192.168.1.5:8080",
		strings.Repeat("x", 120), // needs version 7+ (version info blocks)
		strings.Repeat("y", 210), // version 10 (16-bit length)
	} {
		m, err := qr.Encode(s)
		require.NoError(t, err, s)
		require.Equal(t, len(m), len(m[0]))
		require.Zero(t, (len(m)-17)%4)
		require.Equal(t, s, decode(t, m), s)
	}
}

func TestEncode_SmallestVersion(t *testing.T) {
	m, err := qr.Encode("192.168.1.5:8080")
	require.NoError(t, err)
	require.Len(t, m, 25) // version 1 holds 14 bytes at level M; 16 bytes need version 2
}

func TestEncode_TooLong(t *testing.T) {
	_, err := qr.Encode(strings.Repeat("z", 300))
	require.Error(t, err)
}
