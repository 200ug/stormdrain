package cmd

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"codeberg.org/2ug/stormdrain/internal"
)

func CmdNew(args []string) {
	fs := flag.NewFlagSet("new", flag.ExitOnError)
	profilePath := fs.String("f", "", "path to profile JSON file")
	fs.Usage = func() {
		fmt.Println("[?] usage: stormdrain new [-f <path>] <profile>")
	}
	fs.Parse(args)

	var profile *internal.Profile
	var err error

	if *profilePath != "" {
		profile, err = internal.LoadProfileFromPath(*profilePath)
	} else {
		if fs.NArg() != 1 {
			fs.Usage()
			os.Exit(1)
		}
		profile, err = internal.LoadProfile(fs.Arg(0))
	}
	if err != nil {
		fmt.Printf("[!] failed to load profile: %v\n", err)
		os.Exit(1)
	}

	// substitute profile values to Dockerfile.base template
	if err := profile.DockerfileSubstitution(); err != nil {
		fmt.Printf("[!] failed to substitute dockerfile: %v\n", err)
		os.Exit(1)
	}

	// create spec and a new container (build + startup)
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Printf("[!] failed to resolve cwd: %v\n", err)
		os.Exit(1)
	}
	defer internal.CleanupStagedConfigs(cwd)
	if err = profile.StageConfigs(cwd); err != nil {
		fmt.Printf("[!] failed to stage configs: %v\n", err)
		internal.CleanupStagedConfigs(cwd)
		os.Exit(1)
	}
	podSpec, err := profile.NewPodmanSpec(cwd)
	if err != nil {
		fmt.Printf("[!] failed to create container spec: %v\n", err)
		internal.CleanupStagedConfigs(cwd)
		os.Exit(1)
	}

	if err = internal.PodmanCreate(podSpec); err != nil {
		fmt.Printf("[!] failed to create new container: %v\n", err)
		internal.CleanupStagedConfigs(cwd)
		os.Exit(1)
	}

	// persistence to $cwd/.stormdrain/
	if err = podSpec.WriteToDisk(cwd); err != nil {
		fmt.Printf("[!] failed to write container spec to disk: %v\n", err)
		internal.CleanupStagedConfigs(cwd)
		os.Exit(1)
	}
	fmt.Printf("[+] spec written to '%s'\n", filepath.Join(cwd, ".stormdrain"))
}
