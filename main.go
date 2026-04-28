package main

import (
	"fmt"
	"os"

	"codeberg.org/2ug/stormdrain/cmd"
	"codeberg.org/2ug/stormdrain/internal"
)

const versionCode = "v1.0 (2026-04-28)"

const usage = `[?] usage: stormdrain <command> [flags]

commands:
  new [-f <path>] <profile>  create a new container with profile name (from permanent configs) or path
  enter [name]               attach a shell to a container (defaults to cwd if not given a name)
  close [name] [-f]          close or kill the container (defaults to cwd if not given a name)
  rm [name]                  kill and remove the container and its image (defaults to cwd if not given a name)
  ls [-f <filter>]           list all stormdrain containers (optional filtering)
  purge                      kill and delete all containers, their images, and all (stormdrain related) volumes
  help                       print this usage message
  version                    print current build version

`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(1)
	}

	if err := internal.EnsurePodmanRunning(); err != nil {
		fmt.Printf("[!] tool prep failed: %v\n", err)
		os.Exit(1)
	}

	switch os.Args[1] {
	case "new":
		cmd.CmdNew(os.Args[2:])
	case "enter":
		cmd.CmdEnter(os.Args[2:])
	case "close":
		cmd.CmdClose(os.Args[2:])
	case "rm":
		cmd.CmdDelete(os.Args[2:])
	case "ls":
		cmd.CmdList(os.Args[2:])
	case "purge":
		cmd.CmdDeleteAll()
	case "version", "v":
		fmt.Printf("stormdrain %s\n", versionCode)
	case "help", "h":
		fmt.Print(usage)
	default:
		fmt.Print(usage)
		os.Exit(1)
	}
}
