package cmd

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"codeberg.org/2ug/stormdrain/internal"
)

func CmdDelete(args []string) {
	fs := flag.NewFlagSet("rm", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Printf("usage: %s rm [name]\n", os.Args[0])
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

	if err := internal.PodmanRemove(containerName); err != nil {
		fmt.Printf("[!] failed to remove container %s: %v\n", containerName, err)
		os.Exit(1)
	}
}

func CmdDeleteAll() {
	if err := internal.PodmanPurge(); err != nil {
		fmt.Printf("[!] failed to purge containers: %v\n", err)
		os.Exit(1)
	}
}
