package cmd

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"codeberg.org/2ug/stormdrain/internal"
)

func deleteContainer(containerName string) {
	var spec *internal.PodmanSpec
	projectPath, err := internal.ContainerProjectPath(containerName)
	if err != nil {
		fmt.Printf("[!] failed to resolve project path for '%s': %v\n", containerName, err)
	} else {
		spec, _ = internal.LoadPodmanSpec(projectPath)
		sdDir := filepath.Join(projectPath, ".stormdrain")
		if err = os.RemoveAll(sdDir); err != nil {
			fmt.Printf("[!] failed to remove data directory '%s': %v\n", sdDir, err)
		}
	}

	fmt.Printf("[~] killing and removing container '%s'... ", containerName)
	_ = internal.PodmanStop(containerName, true, true)
	if err := internal.PodmanRemove(containerName); err != nil {
		fmt.Println("failed")
		fmt.Printf("[!] error: %v\n", containerName, err)
		return
	}
	fmt.Println("done")

	if spec != nil && spec.ImageTag != "" {
		fmt.Printf("[~] removing image '%s'... ", spec.ImageTag)
		if err := internal.PodmanImageRemove(spec.ImageTag); err != nil {
			fmt.Println("failed")
			fmt.Printf("[!] error: %v\n", err)
		} else {
			fmt.Println("done")
		}
	}
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
	// (as the decrease in performance is quite substantial with lower amounts of containers to remove)
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

	volumeNames := make(map[string]struct{})
	for _, id := range ids {
		projectPath, err := internal.ContainerProjectPath(id)
		if err != nil {
			continue
		}
		spec, err := internal.LoadPodmanSpec(projectPath)
		if err != nil {
			continue
		}
		for _, vv := range spec.VirtualVolumes {
			volumeNames[vv.Name] = struct{}{}
		}
	}

	for _, id := range ids {
		deleteContainer(id)
	}

	for name := range volumeNames {
		fmt.Printf("[~] removing volume '%s'... ", name)
		if err := internal.PodmanVolumeRemove(name); err != nil {
			fmt.Printf("failed\n[!] %v\n", err)
		} else {
			fmt.Println("done")
		}
	}
}
