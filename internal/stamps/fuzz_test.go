package stamps_test

import (
	"testing"

	"github.com/hashcott/ghostline/internal/stamps"
)

func FuzzDecode(f *testing.F) {
	f.Add("sdns://AgcAAAAAAAAADTIxNy4xNjkuMjAuMjIADWRucy5hYS5uZXQudWsKL2Rucy1xdWVyeQ")
	f.Add("sdns://gQsxLjIuMy40OjQ0Mw")
	f.Add("sdns://BQEAAAAAAAAADG9kb2guZXhhbXBsZQovZG5zLXF1ZXJ5")
	f.Fuzz(func(t *testing.T, s string) { _, _ = stamps.Decode(s) })
}
