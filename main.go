package main

import (
	"embed"
	"fmt"
	"log"
	"os"

	"github.com/hashcott/ghostline/internal/brand"
	"github.com/hashcott/ghostline/internal/cli"
	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Headless modes must branch before application.New: single-instance
	// handling lives inside New and would exit or steal the lock.
	mode, err := cli.Parse(os.Args[1:])
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	switch mode.Kind {
	case cli.KindWatchdog, cli.KindRestore:
		os.Exit(0) // wired in a later task
	}

	app := application.New(application.Options{
		Name:   brand.AppName,
		Assets: application.AssetOptions{Handler: application.AssetFileServerFS(assets)},
	})
	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            brand.AppName,
		Width:            380,
		Height:           580,
		BackgroundColour: application.NewRGB(5, 7, 10),
		URL:              "/",
	})
	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
