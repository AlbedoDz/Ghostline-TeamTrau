// Command genicons draws the Ghostline ring into build/appicon.png; run
// `wails3 task common:generate:icons` afterwards to refresh icon.ico.
//
//	go run ./tools/genicons
package main

import (
	"image/color"
	"log"
	"os"

	"github.com/hashcott/ghostline/internal/icon"
)

func main() {
	png := icon.Ring(color.RGBA{0x00, 0xff, 0xa3, 0xff}, 512)
	if err := os.WriteFile("build/appicon.png", png, 0o644); err != nil {
		log.Fatal(err)
	}
}
