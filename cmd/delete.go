package cmd

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"codeberg.org/2ug/stormdrain/internal"
)

/*
	TODO:

	- handle removal of virtual volumes ("workspace" -> "virtual_volumes" in profiles) if necessary
	- handle volume naming conflicts (e.g. add randomized suffix to name & persist it to spec?)
*/

func deleteContainer(containerName string) {
	// TODO: kill container first before deleting to speed the process up drastically

	projectPath, err := internal.ContainerProjectPath(containerName)
	if err != nil {
		fmt.Printf("[!] failed to resolve project path for '%s': %v\n", containerName, err)
	} else {
		sdDir := filepath.Join(projectPath, ".stormdrain")
		if err = os.RemoveAll(sdDir); err != nil {
			fmt.Printf("[!] failed to remove data directory '%s': %v\n", sdDir, err)
		}
	}

	fmt.Printf("[~] removing container '%s'... ", containerName)
	if err := internal.PodmanRemove(containerName); err != nil {
		fmt.Println("failed")
		fmt.Printf("[!] failed to remove container '%s': %v\n", containerName, err)
	}
	fmt.Println("done")
}

func CmdDelete(args []string) {
	fs := flag.NewFlagSet("rm", flag.ExitOnError)
	fs.Usage = func() {
		fmt.Println("[?] usage: stormdrain rm [name]")
	}
	fs.Parse(args)

	var containerName string
	if fs.NArg() > 0 {
		containerName = fs.Arg(0)
	} else {
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

	deleteContainer(containerName)
}

func CmdDeleteAll() {
	// NOTE: instead of utilizing podman's built-in batch removal system, we opt for cleaner design here
	//		 (as the decrease in performance is quite substantial with lower amounts of containers to remove)
	ids, err := internal.ListContainerIDs()
	if err != nil {
		fmt.Printf("[!] failed to list stormdrain containers: %v\n", err)
		os.Exit(1)
	}
	if len(ids) == 0 {
		fmt.Println("[+] no stormdrain containers found")
		return
	}
	fmt.Printf("[~] removing a total of %d containers\n", len(ids))

	for _, id := range ids {
		deleteContainer(id)
	}
}
