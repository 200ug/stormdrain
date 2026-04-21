package cmd

import (
	"flag"
	"fmt"
	"os"

	"codeberg.org/2ug/stormdrain/internal"
)

func newUsage() {
	fmt.Printf("usage: %s new <profile>\n", os.Args[0])
}

func CmdNew(args []string) {
	fs := flag.NewFlagSet("new", flag.ExitOnError)
	fs.Usage = newUsage
	fs.Parse(args)
	if fs.NArg() != 1 {
		fs.Usage()
		os.Exit(1)
	}
	profileName := args[0]

	// load profile from ~/.config/stormdrain/profiles/
	profile, err := internal.LoadProfile(profileName)
	if err != nil {
		fmt.Printf("[!] failed to load profile: %v\n", err)
		os.Exit(1)
	}

	// substitute profile values to Dockerfile.base template
	if err := profile.DockerfileSubstitution(); err != nil {
		fmt.Printf("[!] dockerfile substitution failed: %v\n", err)
		os.Exit(1)
	}

	// create spec and create (build image and create) a new container
	cwd, err := os.Getwd()
	if err != nil {
		fmt.Printf("[!] failed resolving cwd: %v\n", err)
		os.Exit(1)
	}
	defer internal.CleanupStagedDotfiles(cwd)
	if err = profile.StageDotfiles(cwd); err != nil {
		fmt.Printf("[!] dotfiles staging failed: %v\n", err)
		internal.CleanupStagedDotfiles(cwd)
		os.Exit(1)
	}
	podSpec, err := profile.NewPodmanSpec(cwd)
	if err != nil {
		fmt.Printf("[!] container spec creation failed: %v\n", err)
		internal.CleanupStagedDotfiles(cwd)
		os.Exit(1)
	}

	if err = internal.PodmanCreate(podSpec); err != nil {
		fmt.Printf("[!] container creation failed: %v\n", err)
		internal.CleanupStagedDotfiles(cwd)
		os.Exit(1)
	}

	// persistence to $cwd/.stormdrain/
	if err = podSpec.WriteToDisk(cwd); err != nil {
		fmt.Printf("[!] writing container spec to disk failed: %v\n", err)
		internal.CleanupStagedDotfiles(cwd)
		os.Exit(1)
	}
}
