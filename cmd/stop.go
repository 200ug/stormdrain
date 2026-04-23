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
		fmt.Println("[?] usage: stormdrain close [name] [-f]")
	}
	fs.Parse(args)

	containerName := ""
	if fs.NArg() > 0 {
		containerName = fs.Arg(0)
	}

	if containerName == "" {
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Printf("[!] failed to resolve cwd: %v\n", err)
			os.Exit(1)
		}
		containerName = filepath.Base(cwd)
	}

	if !internal.ContainerExists(containerName) {
		fmt.Printf("[!] container '%s' does not exist\n", containerName)
		os.Exit(1)
	}

	fmt.Printf("[~] stopping container '%s'... ", containerName)
	if err := internal.PodmanStop(containerName, *force); err != nil {
		fmt.Println("failed")
		fmt.Printf("[!] failed to close container '%s': %v\n", containerName, err)
		os.Exit(1)
	}
	fmt.Println("done")
}
