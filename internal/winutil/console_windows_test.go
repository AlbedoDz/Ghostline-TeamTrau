package winutil

import (
	"os"
	"testing"
)

func TestAttachParentConsole_KeepsWorkingStreams(t *testing.T) {
	out, errOut := os.Stdout, os.Stderr
	defer func() { os.Stdout, os.Stderr = out, errOut }()
	// go test runs with a console or with pipes; either way the call must
	// leave usable streams behind and never panic.
	AttachParentConsole()
	if os.Stdout == nil || os.Stderr == nil {
		t.Fatal("streams lost")
	}
}
