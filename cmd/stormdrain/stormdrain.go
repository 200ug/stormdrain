package main

import (
	"log"

	"codeberg.org/2ug/stormdrain/internal/manager"
	"codeberg.org/2ug/stormdrain/internal/tui"
	"codeberg.org/2ug/stormdrain/internal/util"
)

func main() {
	m, err := manager.NewManager(true)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}

	t := tui.NewTUI(m, util.VersionCode)
	if err := t.Run(); err != nil {
		log.Fatalf("Error: %v", err)
	}
}
