package cmd

import (
	"flag"
	"fmt"
	"os"

	"codeberg.org/2ug/stormdrain/internal"
)

func CmdEnter(args []string) {
	fs := flag.NewFlagSet("enter", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Println("[?] usage: stormdrain enter [name]")
	}
	fs.Parse(args)

	var err error
	var specParentPath string
	if fs.NArg() > 0 {
		containerName := fs.Arg(0)
		if specParentPath, err = internal.ContainerProjectPath(containerName); err != nil {
			fmt.Printf("[!] failed to resolve project path for '%s': %v\n", containerName, err)
			os.Exit(1)
		}
	} else {
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Printf("[!] failed to resolve cwd: %v\n", err)
			os.Exit(1)
		}
		specParentPath = cwd
	}

	podSpec, err := internal.LoadPodmanSpec(specParentPath)
	if err != nil {
		fmt.Printf("[!] failed to load container spec: %v\n", err)
		os.Exit(1)
	}

	if !internal.ContainerExists(podSpec.ContainerName) {
		fmt.Printf("[!] container '%s' does not exist\n", podSpec.ContainerName)
		os.Exit(1)
	}

	if err := internal.PodmanAttach(podSpec.ContainerName, podSpec.Shell); err != nil {
		fmt.Printf("[!] failed to attach to container: %v\n", err)
		os.Exit(1)
	}
}
