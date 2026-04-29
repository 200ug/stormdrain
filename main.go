package main

import (
	"fmt"
	"os"

	"codeberg.org/2ug/stormdrain/cmd"
	"codeberg.org/2ug/stormdrain/internal"
)

const versionCode = "v1.0 (2026-04-28)"

func main() {
	if err := internal.EnsurePodmanRunning(); err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}

	cmd.RunTUI(versionCode)
}
