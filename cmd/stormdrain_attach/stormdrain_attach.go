package main

import (
	"log"
	"os"

	"codeberg.org/2ug/stormdrain/internal/manager"
)

func main() {
	if len(os.Args) != 2 {
		log.Fatalf("Usage: %s <container_name>", os.Args[0])
	}

	_, err := manager.NewManager(false) // some startup checks
	if err != nil {
		log.Fatalf("%v", err.Error())
	}

	containerName := os.Args[1]
	projectPath, err := manager.ContainerProjectPath(containerName)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	spec, err := manager.LoadSpec(projectPath, containerName)
	if err != nil {
		log.Fatalf("Error: %v", err)
	}
	if err := spec.AttachIntoContainer(); err != nil {
		log.Fatalf("Error: %v", err)
	}
}
