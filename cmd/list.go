package cmd

import (
	"flag"
	"fmt"
	"os"

	"codeberg.org/2ug/stormdrain/internal"
)

func CmdList(args []string) {
	fs := flag.NewFlagSet("list", flag.ExitOnError)
	filter := fs.String("f", "", "filter containers by name")
	stats := fs.Bool("s", false, "show container statistics")
	fs.Usage = func() {
		fmt.Println("[?] usage: stormdrain list [-f <filter>] [-s]")
	}
	fs.Parse(args)

	if err := internal.PodmanList(*filter, *stats); err != nil {
		fmt.Printf("[!] failed to list containers: %v\n", err)
		os.Exit(1)
	}
}
