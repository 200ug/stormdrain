package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func EnsurePodmanRunning() error {
	if _, err := exec.LookPath("podman"); err != nil {
		return fmt.Errorf("podman not found in PATH: %w", err)
	}

	out, err := exec.Command("podman", "machine", "list", "--format", "json").Output()
	if err != nil {
		return fmt.Errorf("failed to list podman machines: %w", err)
	}

	var machines []struct {
		Name    string `json:"Name"`
		Running bool   `json:"Running"`
		Default bool   `json:"Default"`
	}
	if err := json.Unmarshal(out, &machines); err != nil {
		return fmt.Errorf("failed to parse podman machine list: %w", err)
	}

	if len(machines) == 0 {
		return fmt.Errorf("no podman machine initialized; run 'podman machine init' first")
	}

	machine := machines[0]
	for _, m := range machines {
		if m.Default {
			machine = m
			break
		}
	}

	if machine.Running {
		return nil
	}

	fmt.Printf("[~] no running podman machine detected, starting '%s'\n", machine.Name)
	cmd := exec.Command("podman", "machine", "start", machine.Name)
	cmd.Stdout = nil
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("failed to start podman machine %q: %w", machine.Name, err)
	}

	return nil
}

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
	if spec.ProjectMount {
		args = append(args, "-w", fmt.Sprintf("/home/dev/%s", spec.ContainerName))
		args = append(args, "-v", fmt.Sprintf("%s:/home/dev/%s", spec.ProjectPath, spec.ContainerName))
	}
	for _, v := range spec.VirtualVolumes {
		args = append(args, "-v", fmt.Sprintf("%s:%s:U", v.Name, v.Path))
	}
	for _, pm := range spec.Ports {
		args = append(args, "-p", fmt.Sprintf("%d:%d", pm.Host, pm.Container))
	}
	for _, ef := range spec.EnvFiles {
		// if ef doesn't exist on host, this will error (captured through the cmd.Run() call)
		args = append(args, "--env-file", ef)
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

func PodmanList(filter string) error {
	args := []string{"ps", "-a", "--filter", "label=stormdrain"}
	if filter != "" {
		args = append(args, "--filter", filter)
	}
	cmd := exec.Command("podman", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}

func PodmanStop(containerName string, kill bool, ignoreErr bool) error {
	action := "stop"
	if kill {
		action = "kill"
	}
	cmd := exec.Command("podman", action, containerName)
	cmd.Stdout = nil
	if ignoreErr {
		cmd.Stderr = nil
	} else {
		cmd.Stderr = os.Stderr
	}

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

func PodmanImageRemove(imageTag string) error {
	cmd := exec.Command("podman", "rmi", imageTag)
	cmd.Stdout = nil
	cmd.Stderr = nil

	return cmd.Run()
}

func PodmanVolumeRemove(name string) error {
	cmd := exec.Command("podman", "volume", "rm", name)
	cmd.Stdout = nil
	cmd.Stderr = nil

	return cmd.Run()
}

func podmanStart(name string) error {
	cmd := exec.Command("podman", "start", name)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	return cmd.Run()
}
