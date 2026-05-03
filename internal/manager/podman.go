package manager

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"codeberg.org/2ug/stormdrain/internal/util"
)

const (
	DefaultShell = "/bin/zsh"
	detachKeys   = "ctrl-x,ctrl-q"
)

// Static statistics that can be queried on tool startup. They won't change
// unless the active Podman machine is stopped and modified.
type PodmanStats struct {
	MachineName            string
	AvailableTotalCPUs     int
	AvailableTotalMemoryGB int
	AvailableDiskSizeGB    int
}

func NewPodmanStats(rawMachineStats *machineStats) PodmanStats {
	memory, err := strconv.Atoi(rawMachineStats.Memory)
	if err != nil {
		memory = -1
	} else {
		memory = memory / 1_000_000_000
	}
	diskSize, err := strconv.Atoi(rawMachineStats.DiskSize)
	if err != nil {
		diskSize = -1
	} else {
		diskSize = diskSize / 1_000_000_000
	}
	return PodmanStats{
		MachineName:            rawMachineStats.Name,
		AvailableTotalCPUs:     rawMachineStats.CPUs,
		AvailableTotalMemoryGB: memory,
		AvailableDiskSizeGB:    diskSize,
	}
}

func podmanInPath() bool {
	if _, err := exec.LookPath("podman"); err != nil {
		return false
	}
	return true
}

// Helper struct to parse JSON from machine list command into.
type machineStats struct {
	Name     string `json:"Name"`
	Running  bool   `json:"Running"`
	Default  bool   `json:"Default"`
	CPUs     int    `json:"CPUs"`
	Memory   string `json:"Memory"`
	DiskSize string `json:"DiskSize"`
}

// Attempts to start the default Podman machine if it's not running. Returns
// the commands parsed output for the started/running machine.
func ensurePodmanMachineIsRunning() (*machineStats, error) {
	rawList, err := exec.Command("podman", "machine", "list", "--format", "json").Output()
	if err != nil {
		return nil, err
	}

	var machines []machineStats
	if err := json.Unmarshal(rawList, &machines); err != nil {
		return nil, fmt.Errorf("ensurePodmanMachineIsRunning parsing: %w", err)
	}

	if len(machines) == 0 {
		return nil, fmt.Errorf("no podman machine initialized")
	}

	machine := machines[0]
	for _, m := range machines {
		if m.Default {
			machine = m
			break
		}
	}
	if machine.Running {
		return &machine, nil
	}

	// NOTE: logging is ok here, as this runs before TUI initialization
	log.Printf("No running machine found, starting %q...", machine.Name)
	cmd := exec.Command("podman", "machine", "start", machine.Name)
	cmd.Stdout = nil
	cmd.Stderr = os.Stderr
	return &machine, cmd.Run()
}

type containerPs struct {
	ID        string `json:"Id"` // first 12 bytes used to match stats output
	State     string `json:"State"`
	StartedAt int    `json:"StartedAt"`
	CreatedAt int    `json:"Created"`
	ImageTag  string `json:"Image"`
	Labels    struct {
		ProjectPath string `json:"stormdrain.project-path"`
	} `json:"Labels"`
	Mounts []string `json:"Mounts"`
	Ports  []portPs `json:"Ports"`
}

type portPs struct {
	ContainerPort int    `json:"container_port"`
	HostPort      int    `json:"host_port"`
	Protocol      string `json:"protocol"`
}

type containerStats struct {
	ID                   string `json:"id"`
	Name                 string `json:"name"`
	CPUDirectPercentage  string `json:"cpu_percent"`
	CPUAveragePercentage string `json:"avg_cpu"`
	MemoryPercentage     string `json:"mem_percent"`
	NetworkIO            string `json:"net_io"`
}

// Executes "podman ps -a" and "podman stats -a" commands to query states and stats
// of all existing (running or not) stormdrain related containers. This function
// should be used for periodic container listing updates.
func getStormdrainContainers() ([]Container, error) {
	var containers []Container
	rawList, err := exec.Command("podman", "ps", "-a", "--format", "json", "--filter", "label=stormdrain").Output()
	if err != nil {
		return containers, err
	}
	var parsedList []containerPs
	if err := json.Unmarshal(rawList, &parsedList); err != nil {
		return containers, err
	}

	rawStats, err := exec.Command("podman", "stats", "-a", "--format", "json", "--no-stream").Output()
	if err != nil {
		return containers, err
	}
	var parsedStats []containerStats
	if err := json.Unmarshal(rawStats, &parsedStats); err != nil {
		return containers, err
	}

	// combine ps and stats outputs with an ID based lookup table
	statsByID := make(map[string]*containerStats, len(parsedStats))
	for i := range parsedStats {
		statsByID[parsedStats[i].ID] = &parsedStats[i]
	}
	for _, ps := range parsedList {
		shortID := ps.ID
		if len(shortID) > 12 {
			shortID = shortID[:12]
		}
		stats, _ := statsByID[shortID]
		c := NewContainer(ps, stats)
		containers = append(containers, c)
	}
	return containers, nil
}

// Build configuration for an image. Created during new container construction
// (values derived from the given profile) and stored to be referenced by
// follow-up commands.
type Spec struct {
	ContainerName  string            `json:"container_name"`
	Hostname       string            `json:"hostname"`
	ImageTag       string            `json:"image_tag"`
	Shell          string            `json:"shell"`
	ProjectPath    string            `json:"project_path"`
	ProjectMount   bool              `json:"project_mount"`
	BuildCtx       string            `json:"-"`
	ConfigsDir     string            `json:"-"`
	BuildArgs      map[string]string `json:"build_args"`
	VirtualVolumes []VirtualVolume   `json:"virtual_volumes"`
	Ports          []PortMap         `json:"ports"`
	EnvFiles       []string          `json:"env_files"`
}

func NewSpec(profile *Profile, projectPath string) (*Spec, error) {
	userHome, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	uid := os.Getuid()
	gid := os.Getgid()
	projectName := filepath.Base(projectPath)
	containerName, hostname := uniqueContainerName(projectName)
	shell := profile.Shell
	if shell == "" {
		shell = DefaultShell
	}

	spec := &Spec{
		ContainerName: containerName,
		Hostname:      hostname,
		ImageTag:      fmt.Sprintf("stormdrain-%s-%s", profile.Name, projectName),
		Shell:         shell,
		ProjectPath:   projectPath,
		ProjectMount:  profile.ProjectMount == nil || *profile.ProjectMount,
		BuildCtx:      filepath.Join(projectPath, ".stormdrain"),
		ConfigsDir:    filepath.Join(projectPath, ".stormdrain", "configs"),
		BuildArgs: map[string]string{
			"UID": strconv.Itoa(uid),
			"GID": strconv.Itoa(gid),
		},
		VirtualVolumes: profile.VirtualVolumes,
		Ports:          profile.Ports,
	}
	for _, envFile := range profile.EnvFiles {
		expPath := os.ExpandEnv(strings.Replace(envFile, "~", userHome, 1))
		if _, err := os.Stat(expPath); err != nil {
			return nil, err
		}
		spec.EnvFiles = append(spec.EnvFiles, expPath)
	}
	return spec, nil
}

func LoadSpec(projectPath string) (*Spec, error) {
	specPath := filepath.Join(projectPath, ".stormdrain", "pod_spec.json")
	rawContents, err := os.ReadFile(specPath)
	if err != nil {
		return nil, err
	}
	var s Spec
	if err = json.Unmarshal(rawContents, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

// Build and run a brand new container. If those steps are successful, the spec
// will be written to the project's .stormdrain/ directory as pod_spec.json.
func (s *Spec) CreateContainer() error {
	// 1. build image from generated template
	buildArgs := []string{
		"build",
		"-t", s.ImageTag,
		"-f", filepath.Join(s.BuildCtx, "Dockerfile.sd"),
	}
	for k, v := range s.BuildArgs {
		buildArgs = append(buildArgs, "--build-arg", fmt.Sprintf("%s=%s", k, v))
	}
	buildArgs = append(buildArgs, s.BuildCtx)
	buildCmd := exec.Command("podman", buildArgs...)
	buildCmd.Stdout = nil
	buildCmd.Stderr = nil
	err := buildCmd.Run()
	if err != nil {
		return err
	}

	// 2. run using built image
	runArgs := []string{
		"run",
		"-d",
		"--name", s.ContainerName,
		"--hostname", s.Hostname,
		"--label", "stormdrain",
		"--label", fmt.Sprintf("stormdrain.project-path=%s", s.ProjectPath),
	}
	if s.ProjectMount {
		runArgs = append(runArgs, "-w", fmt.Sprintf("/home/dev/%s", s.ContainerName))
		runArgs = append(runArgs, "-v", fmt.Sprintf("%s:/home/dev/%s", s.ProjectPath, s.ContainerName))
	}
	for _, v := range s.VirtualVolumes {
		runArgs = append(runArgs, "-v", fmt.Sprintf("%s:%s:U", v.Name, v.Path))
	}
	for _, pm := range s.Ports {
		runArgs = append(runArgs, "-p", fmt.Sprintf("%d:%d", pm.Host, pm.Container))
	}
	for _, ef := range s.EnvFiles {
		// will error if ef doesn't exist on host fs
		runArgs = append(runArgs, "--env-file", ef)
	}
	runArgs = append(runArgs, s.ImageTag)
	runCmd := exec.Command("podman", runArgs...)
	runCmd.Stdout = nil
	runCmd.Stderr = nil
	if err := runCmd.Run(); err != nil {
		return err
	}

	// 3. persist the image to host disk
	return s.WriteToDisk()
}

// Writes the file as formatted JSON to .stormdrain/pod_spec.json of the project.
func (s *Spec) WriteToDisk() error {
	formattedJson, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	specPath := filepath.Join(s.ProjectPath, ".stormdrain", "pod_spec.json")
	return os.WriteFile(specPath, formattedJson, 0644)
}

// Kills and deletes the container and its build image. Also removes the related
// .stormdrain/ directory from the given (project root) path.
func (s *Spec) RemoveContainer() error {
	exists, isRunning := containerExists(s.ContainerName)
	if exists {
		if isRunning {
			if err := stopContainer(s.ContainerName, true); err != nil {
				return err
			}
		}
		if err := removeContainer(s.ContainerName); err != nil {
			return err
		}
	}
	if err := removeImage(s.ImageTag); err != nil {
		return err
	}
	return os.RemoveAll(filepath.Join(s.ProjectPath, ".stormdrain"))
}

func (s *Spec) AttachIntoContainer() error {
	exists, isRunning := containerExists(s.ContainerName)
	if !exists {
		return fmt.Errorf("container %s does not exist", s.ContainerName)
	} else if !isRunning {
		if err := startContainer(s.ContainerName); err != nil {
			return err
		}
	}

	cmd := exec.Command("podman", "exec", "-it", "--detach-keys", detachKeys, s.ContainerName, s.Shell)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	err := cmd.Run()
	if _, ok := err.(*exec.ExitError); ok {
		return nil
	}
	return err
}

func containerExists(containerName string) (bool, bool) {
	cmdOut, err := exec.Command("podman", "inspect", containerName, "--format", "{{.State.Running}}").Output()
	if err != nil {
		return false, false
	}
	isRunning, err := strconv.ParseBool(strings.TrimSpace(string(cmdOut)))
	if err != nil {
		return true, false
	}
	return true, isRunning
}

func ContainerProjectPath(containerName string) (string, error) {
	cmdOut, err := exec.Command("podman", "inspect", containerName, "--format", "{{index .Config.Labels \"stormdrain.project-path\"}}").Output()
	if err != nil {
		return "", err
	}
	path := strings.TrimSpace(string(cmdOut))
	if path == "" {
		return "", fmt.Errorf("container %q missing project path label", containerName)
	}
	return path, nil
}

func startContainer(containerName string) error {
	cmd := exec.Command("podman", "start", containerName)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

func removeContainer(containerName string) error {
	cmd := exec.Command("podman", "rm", "-f", containerName)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

func stopContainer(containerName string, force bool) error {
	action := "stop"
	if force {
		action = "kill"
	}
	cmd := exec.Command("podman", action, containerName)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

func removeVolume(volumeName string) error {
	cmd := exec.Command("podman", "volume", "rm", volumeName)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

func removeImage(imageTag string) error {
	cmd := exec.Command("podman", "rmi", imageTag)
	cmd.Stdout = nil
	cmd.Stderr = nil
	return cmd.Run()
}

// Randomize container name's suffix (hostname) to find a unique container name.
func uniqueContainerName(projectName string) (string, string) {
	var containerName, hostname string
	for {
		hostname = util.RandomHostname()
		containerName = fmt.Sprintf("%s-%s", projectName, hostname)
		if exists, _ := containerExists(containerName); !exists {
			break
		}
	}
	return containerName, hostname
}
