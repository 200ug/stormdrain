package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
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

	// NOTE: hardcoded detach keys for now, probably should be configurable
	cmd := exec.Command("podman", "exec", "-it", "--detach-keys", "ctrl-x,ctrl-q", containerName, shell)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	err := cmd.Run()
	if _, ok := err.(*exec.ExitError); ok {
		return nil
	}

	return err
}

type GeneralStats struct {
	// static
	MachineName            string
	AvailableTotalCPUs     int
	AvailableTotalMemoryGB int
	AvailableDiskSizeGB    int

	// dynamic, refreshed via Update()
	TotalContainers int
	TotalRunning    int
	Containers      []ContainerStats
}

func NewGeneralStats() (*GeneralStats, error) {
	machineStats, err := queryActiveMachineStats()
	if err != nil {
		return nil, err
	}

	containers, err := ListContainers()
	if err != nil {
		return nil, err
	}
	runningCount := 0
	for _, c := range containers {
		if c.Uptime != -1 {
			runningCount++
		}
	}
	memory, err := strconv.Atoi(machineStats.Memory)
	if err != nil {
		memory = -1
	} else {
		memory = memory / 1_000_000_000
	}
	diskSize, err := strconv.Atoi(machineStats.DiskSize)
	if err != nil {
		diskSize = -1
	} else {
		diskSize = diskSize / 1_000_000_000
	}
	gs := &GeneralStats{
		MachineName:            machineStats.Name,
		AvailableTotalCPUs:     machineStats.CPUs,
		AvailableTotalMemoryGB: memory,
		AvailableDiskSizeGB:    diskSize,
		TotalContainers:        len(containers),
		TotalRunning:           runningCount,
		Containers:             containers,
	}

	return gs, nil
}

func (gs *GeneralStats) Update() error {
	all, err := ListContainers()
	if err != nil {
		return err
	}
	gs.TotalContainers = len(all)
	gs.TotalRunning = 0
	for _, c := range all {
		if c.Uptime != -1 {
			gs.TotalRunning++
		}
	}
	gs.Containers = all

	return nil
}

type ContainerStats struct {
	// list view
	Name   string
	Uptime int    // -1 if down (overall status derived from this)
	CPU    string // "<dir_perc>% / <avg_perc>%"
	Memory string // "<perc>%"
	NetIO  string // "<total_sent> / <total_received>"

	// expanded/inspection view
	ImageTag    string
	ProjectPath string // from label
	Mounts      []string
	Ports       []portStat
}

type machineStats struct {
	Name     string `json:"Name"`
	Running  bool   `json:"Running"`
	Default  bool   `json:"Default"`
	CPUs     int    `json:"CPUs"`
	Memory   string `json:"Memory"`
	DiskSize string    `json:"DiskSize"`
}

func queryActiveMachineStats() (*machineStats, error) {
	raw, err := exec.Command("podman", "machine", "list", "--format", "json").Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list podman machines: %w", err)
	}

	var machines []machineStats
	if err := json.Unmarshal(raw, &machines); err != nil {
		return nil, fmt.Errorf("failed to parse machine list: %w", err)
	}

	for _, m := range machines {
		if m.Running && m.Default {
			return &m, nil
		}
	}
	for _, m := range machines {
		if m.Running {
			return &m, nil
		}
	}

	return nil, fmt.Errorf("no running podman machine found")
}

type rawPSOutput struct {
	ID        string `json:"Id"` // first 12 bytes used to match stats output
	State     string `json:"State"`
	StartedAt int    `json:"StartedAt"`
	CreatedAt int    `json:"Created"`
	ImageTag  string `json:"Image"`
	Labels    struct {
		ProjectPath string `json:"stormdrain.project-path"`
	} `json:"Labels"`
	Mounts []string   `json:"Mounts"`
	Ports  []portStat `json:"Ports"`
}

type rawStatOutput struct {
	ID                   string `json:"id"`
	Name                 string `json:"name"`
	CPUDirectPercentage  string `json:"cpu_percent"`
	CPUAveragePercentage string `json:"avg_cpu"`
	MemoryPercentage     string `json:"mem_percent"`
	NetworkIO            string `json:"net_io"`
}

type portStat struct {
	ContainerPort int    `json:"container_port"`
	HostPort      int    `json:"host_port"`
	Protocol      string `json:"protocol"`
}

func ListContainers() ([]ContainerStats, error) {
	psArgs := []string{"ps", "-a", "--format", "json", "--filter", "label=stormdrain"}
	psOut, err := exec.Command("podman", psArgs...).Output()
	if err != nil {
		return nil, fmt.Errorf("failed to list containers: %w", err)
	}
	var psRaw []rawPSOutput
	if err := json.Unmarshal(psOut, &psRaw); err != nil {
		return nil, fmt.Errorf("failed to parse list output: %w", err)
	}

	statsArgs := []string{"stats", "-a", "--format", "json", "--no-stream"}
	statsOut, err := exec.Command("podman", statsArgs...).Output()
	if err != nil {
		return nil, fmt.Errorf("failed to get podman stats: %w", err)
	}
	var statsRaw []rawStatOutput
	if err := json.Unmarshal(statsOut, &statsRaw); err != nil {
		return nil, fmt.Errorf("failed to parse podman stats: %w", err)
	}
	// build lookup map from stats output (should be 1-to-1 with ps output now 
	// that there's no *additional* filtering on either of them)
	statsByID := make(map[string]*rawStatOutput, len(statsRaw))
	for i := range statsRaw {
		statsByID[statsRaw[i].ID] = &statsRaw[i]
	}

	containers := make([]ContainerStats, 0, len(psRaw))
	for _, psr := range psRaw {
		cs := ContainerStats{
			ImageTag:    psr.ImageTag,
			ProjectPath: psr.Labels.ProjectPath,
			Mounts:      psr.Mounts,
			Ports:       psr.Ports,
		}
		if psr.State != "running" {
			cs.Uptime = -1
		} else {
			cs.Uptime = computeUptime(psr.StartedAt)
		}
		shortID := psr.ID[:12]
		if stat, ok := statsByID[shortID]; ok {
			cs.Name = stat.Name
			cs.CPU = fmt.Sprintf("%s / %s", stat.CPUDirectPercentage, stat.CPUAveragePercentage)
			cs.Memory = stat.MemoryPercentage
			cs.NetIO = stat.NetworkIO
		}
		containers = append(containers, cs)
	}

	return containers, nil
}

func computeUptime(startedAt int) int {
	if startedAt == 0 {
		return -1
	}
	return int(time.Now().Unix()) - startedAt
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
