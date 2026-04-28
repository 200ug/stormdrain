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
	fs.Usage = func() {
		fmt.Println("[?] usage: stormdrain list [-f <filter>]")
	}
	fs.Parse(args)

	if err := internal.PodmanList(*filter); err != nil {
		fmt.Printf("[!] failed to list containers: %v\n", err)
		os.Exit(1)
	}
}
