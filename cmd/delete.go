package cmd

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"codeberg.org/2ug/stormdrain/internal"
)

func deleteContainer(containerName string) {
	projectPath, err := internal.ContainerProjectPath(containerName)
	if err != nil {
		fmt.Printf("[!] failed to resolve project path for %s: %v\n", containerName, err)
	} else {
		sdDir := filepath.Join(projectPath, ".stormdrain")
		if err = os.RemoveAll(sdDir); err != nil {
			fmt.Printf("[!] failed to remove data directory %s: %v\n", sdDir, err)
		}
	}

	if err := internal.PodmanRemove(containerName); err != nil {
		fmt.Printf("[!] failed to remove container %s: %v\n", containerName, err)
	}
}

func CmdDelete(args []string) {
	fs := flag.NewFlagSet("rm", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Printf("usage: %s rm [name]\n", os.Args[0])
	}
	fs.Parse(args)

	var containerName string
	if fs.NArg() > 0 {
		containerName = fs.Arg(0)
	} else {
		cwd, err := os.Getwd()
		if err != nil {
			fmt.Printf("[!] failed resolving cwd: %v\n", err)
			os.Exit(1)
		}
		containerName = filepath.Base(cwd)
	}

	deleteContainer(containerName)
}

func CmdDeleteAll() {
	// NOTE: instead of utilizing podman's built-in batch removal system, we opt for cleaner design here
	//		 (as the decrease in performance is quite substantial with lower amounts of containers to remove)
	ids, err := internal.StormdrainContainerIDs()
	if err != nil {
		fmt.Printf("[!] failed to list stormdrain containers: %v\n", err)
		os.Exit(1)
	}
	if len(ids) == 0 {
		fmt.Println("no stormdrain containers found")
		return
	}

	for _, id := range ids {
		deleteContainer(id)
	}
}
