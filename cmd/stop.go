package cmd

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"codeberg.org/2ug/stormdrain/internal"
)

func CmdClose(args []string) {
	fs := flag.NewFlagSet("close", flag.ExitOnError)
	force := fs.Bool("f", false, "send SIGKILL instead of SIGTERM")
	fs.Usage = func() {
		fmt.Printf("usage: %s close [name] [-f]\n", os.Args[0])
	}
	fs.Parse(args)

	containerName := ""
	if fs.NArg() > 0 {
		containerName = fs.Arg(0)
	}

	if containerName == "" {
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Printf("[!] failed resolving cwd: %v\n", err)
			os.Exit(1)
		}
		containerName = filepath.Base(cwd)
	}

	if err := internal.PodmanStop(containerName, *force); err != nil {
		fmt.Printf("[!] failed to close container %s: %v\n", containerName, err)
		os.Exit(1)
	}
}
