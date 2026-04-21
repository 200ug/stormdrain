package main

import (
	"fmt"
	"os"

	"codeberg.org/2ug/stormdrain/cmd"
)

const versionCode = "v0.1 (2026-04-22)"

const usage = `usage: stormdrain <command> [flags]

commands:
  new <profile>             create a new container from a profile
  enter                     attach a shell to a container matching cwd
  close [name] [-f]         close the container matching cwd or given container name (optionally SIGKILL)
  rm [name]                 remove the container matching cwd or given container name
  list [-f <filter>] [-s]   list all stormdrain containers (optionl filtering and stats)
  purge                     shut down and delete *all* stormdrain containers
  help                      print this usage message
  version                   print current build version
`

func main() {
	if len(os.Args) < 2 {
		fmt.Print(usage)
		os.Exit(1)
	}

	switch os.Args[1] {
	case "new":
		cmd.CmdNew(os.Args[2:])
	case "enter":
		cmd.CmdEnter()
	case "close":
		cmd.CmdClose(os.Args[2:])
	case "rm":
		cmd.CmdDelete(os.Args[2:])
	case "list":
		cmd.CmdList(os.Args[2:])
	case "purge":
		cmd.CmdDeleteAll()
	case "version":
		fmt.Printf("stormdrain %s\n", versionCode)
	case "help":
		fmt.Print(usage)
	default:
		fmt.Print(usage)
		os.Exit(1)
	}
}
