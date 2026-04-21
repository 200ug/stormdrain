package cmd

import (
	"fmt"
	"os"

	"codeberg.org/2ug/stormdrain/internal"
)

func CmdEnter() {
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Printf("[!] failed resolving cwd: %v\n", err)
		os.Exit(1)
	}
	podSpec, err := internal.LoadPodmanSpec(cwd)
	if err != nil {
		fmt.Printf("[!] failed to load container spec: %v\n", err)
		os.Exit(1)
	}

	if err := internal.PodmanAttach(podSpec.ContainerName, podSpec.Shell); err != nil {
		fmt.Printf("[!] failed to attach to container: %v\n", err)
		os.Exit(1)
	}
}
