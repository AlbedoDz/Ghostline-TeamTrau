package main

import (
	"embed"
	"fmt"
	"os"

	goodbyedpi "github.com/hashcott/ghostline/assets/goodbyedpi"
	zapret2 "github.com/hashcott/ghostline/assets/zapret2"
	"github.com/hashcott/ghostline/internal/cli"
	"github.com/hashcott/ghostline/internal/shell"
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
	case cli.KindWatchdog, cli.KindRestore, cli.KindRemoveCerts:
		os.Exit(runHeadless(mode))
	}
	exe, err := os.Executable()
	if err != nil {
		os.Exit(1)
	}
	if err := shell.Run(shell.Options{Mode: mode, Assets: assets, GoodbyeDPIAssets: goodbyedpi.FS, Zapret2Assets: zapret2.FS, Executable: exe}); err != nil {
		os.Exit(1)
	}
}
