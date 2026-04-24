package internal

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// TODO: add function that starts the podman machine if it's not running (podman machine start)

func PodmanCreate(spec *PodmanSpec) error {
	fmt.Println("[~] building container image")
	if err := podmanBuild(spec); err != nil {
		return fmt.Errorf("podman build failed: %w", err)
	}
	fmt.Println("[~] running the newly built container")
	if err := podmanRun(spec); err != nil {
		return fmt.Errorf("podman run failed: %w", err)
	}

	return nil
}

func podmanBuild(spec *PodmanSpec) error {
	args := []string{
		"build",
		"-t", spec.ImageTag,
		"-f", filepath.Join(spec.BuildCtx, "Dockerfile.sd"),
	}
	for k, v := range spec.BuildArgs {
		args = append(args, "--build-arg", fmt.Sprintf("%s=%s", k, v))
	}
	args = append(args, spec.BuildCtx)

	cmd := exec.Command("podman", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// Creates and runs a new container with the specified mounts and volumes.
// If a container with the same name already exists (i.e. is stopped),
// it is started instead.
func podmanRun(spec *PodmanSpec) error {
	if ContainerExists(spec.ContainerName) {
		return podmanStart(spec.ContainerName)
	}

	args := []string{
		"run",
		"-d",
		"--name", spec.ContainerName,
		"--hostname", spec.Hostname,
		"--label", "stormdrain",
		"--label", fmt.Sprintf("stormdrain.project-path=%s", spec.ProjectPath),
	}
	if len(spec.DirectMounts) > 0 {
		args = append(args, "-w", fmt.Sprintf("/home/dev/%s", spec.ContainerName))
	}
	for _, m := range spec.DirectMounts {
		args = append(args, "-v", fmt.Sprintf("%s:%s", m.HostPath, m.ContainerPath))
	}
	for _, v := range spec.VirtualVolumes {
		args = append(args, "-v", fmt.Sprintf("%s:%s:U", v.Name, v.Path))
	}
	args = append(args, spec.ImageTag)

	cmd := exec.Command("podman", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

// Ensures the container is running (starting it if stopped), then attaches
// an interactive shell session.
func PodmanAttach(containerName, shell string) error {
	if ContainerExists(containerName) {
		if err := podmanStart(containerName); err != nil {
			return fmt.Errorf("podman start failed: %w", err)
		}
	} else {
		return fmt.Errorf("container '%q' does not exist", containerName)
	}

	cmd := exec.Command("podman", "exec", "-it", containerName, shell)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if _, ok := err.(*exec.ExitError); ok {
		return nil
	}

	return err
}

func PodmanList(filter string, stats bool) error {
	// TODO: implement -s stats output (image size, uptime, resource usage, etc.)
	args := []string{"ps", "-a", "--filter", "label=stormdrain"}
	if filter != "" {
		args = append(args, "--filter", filter)
	}
	cmd := exec.Command("podman", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func PodmanStop(containerName string, kill bool) error {
	action := "stop"
	if kill {
		action = "kill"
	}
	cmd := exec.Command("podman", action, containerName)
	cmd.Stdout = nil
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func PodmanRemove(containerName string) error {
	cmd := exec.Command("podman", "rm", "-f", containerName)
	cmd.Stdout = nil
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func ListContainerIDs() ([]string, error) {
	cmd := exec.Command("podman", "ps", "-a", "-q", "--filter", "label=stormdrain")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list stormdrain containers: %w", err)
	}
	ids := strings.Fields(strings.TrimSpace(string(output)))

	return ids, nil
}

func ContainerExists(name string) bool {
	cmd := exec.Command("podman", "inspect", name, "--format", "{{.Name}}")
	cmd.Stdout = nil
	cmd.Stderr = nil

	return cmd.Run() == nil
}

func ContainerProjectPath(name string) (string, error) {
	cmd := exec.Command("podman", "inspect", name, "--format", "{{index .Config.Labels \"stormdrain.project-path\"}}")
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("failed to inspect container '%q': %w", name, err)
	}

	path := strings.TrimSpace(string(out))
	if path == "" {
		return "", fmt.Errorf("container '%q' has no stormdrain.project-path label", name)
	}

	return path, nil
}

func podmanStart(name string) error {
	cmd := exec.Command("podman", "start", name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
