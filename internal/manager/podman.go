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
	"time"

	"codeberg.org/2ug/stormdrain/internal/util"
)

const (
	DefaultShell = "/bin/zsh"
	detachKeys   = "ctrl-x,ctrl-q"
)

// Static statistics that can be queried on tool startup. They won't change
// unless the active Podman machine is stopped and modified.
type PodmanStats struct {
	IsNative               bool
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
		IsNative:               false,
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

// Attempts to start the default Podman machine (VM) if it's not running.
// Returns the command's parsed output for the default/running machine, which
// can be fed to TUI as VM statistics. Notably skipped on all platforms except
// for Darwin.
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
		ProfileName string `json:"stormdrain.profile-name"`
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
	ProfileName    string            `json:"profile_name"`
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

func NewSpecWithContainerName(profile *Profile, projectPath, containerName, hostname string) (*Spec, error) {
	userHome, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	uid := os.Getuid()
	gid := os.Getgid()
	projectName := filepath.Base(projectPath)
	shell := profile.Shell
	if shell == "" {
		shell = DefaultShell
	}

	spec := &Spec{
		ContainerName: containerName,
		Hostname:      hostname,
		ProfileName:   profile.Name,
		ImageTag:      fmt.Sprintf("stormdrain-%s-%s", profile.Name, projectName),
		Shell:         shell,
		ProjectPath:   projectPath,
		ProjectMount:  profile.ProjectMount == nil || *profile.ProjectMount,
		// NOTE: scoping with containerName necessary to facilitate multiple containers
		//		 in the same project space (i.e. in the same .stormdrain directory)
		BuildCtx:   filepath.Join(projectPath, ".stormdrain", containerName),
		ConfigsDir: filepath.Join(projectPath, ".stormdrain", containerName, "configs"),
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

func LoadSpec(projectPath, containerName string) (*Spec, error) {
	specPath := filepath.Join(projectPath, ".stormdrain", containerName, "pod_spec.json")
	rawContents, err := os.ReadFile(specPath)
	if err != nil {
		return nil, err
	}
	var s Spec
	if err = json.Unmarshal(rawContents, &s); err != nil {
		return nil, err
	}
	// recompute values in order to support container recreation
	s.BuildCtx = filepath.Join(s.ProjectPath, ".stormdrain", s.ContainerName)
	s.ConfigsDir = filepath.Join(s.BuildCtx, "configs")
	return &s, nil
}

// Build and run a brand new container. If those steps are successful, the spec
// will be written to the project's .stormdrain/ directory as pod_spec.json.
// Regardless of the command's success, the podman commands' output will be logged
// into project's .stormdrain/build.log (build context) for debugging purposes.
func (s *Spec) CreateContainer() error {
	// open log file, write header
	logPath := filepath.Join(s.BuildCtx, "build.log")
	logFile, err := os.Create(logPath) // O_TRUNC -> always overrides
	if err != nil {
		return fmt.Errorf("could not create build log file: %w", err)
	}
	defer logFile.Close()
	// simple header for context
	fmt.Fprintf(logFile, "### CONTAINER BUILD LOG ###\n")
	fmt.Fprintf(logFile, "Timestamp: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(logFile, "Image: %s\n", s.ImageTag)
	fmt.Fprintf(logFile, "Project: %s\n", s.ProjectPath)

	if err := s.buildImage(logFile); err != nil {
		return err
	}
	if err := s.runContainer(logFile); err != nil {
		return err
	}

	// persist (full) config to local disk
	return s.WriteToDisk()
}

func (s *Spec) RecreateContainer() error {
	logPath := filepath.Join(s.BuildCtx, "recreate.log") // separate from build.log
	logFile, err := os.Create(logPath)
	if err != nil {
		return fmt.Errorf("could not create recreate log file: %w", err)
	}
	defer logFile.Close()

	fmt.Fprintf(logFile, "### CONTAINER RECREATE LOG ###\n")
	fmt.Fprintf(logFile, "Timestamp: %s\n", time.Now().Format("2006-01-02 15:04:05"))
	fmt.Fprintf(logFile, "Image: %s\n", s.ImageTag)
	fmt.Fprintf(logFile, "Project: %s\n", s.ProjectPath)

	// 1. build image only if it doesn't already exist
	if !imageExists(s.ImageTag) {
		if err := s.buildImage(logFile); err != nil {
			return err
		}
	} else {
		fmt.Fprintf(logFile, "Reusing existing image %s\n", s.ImageTag)
	}

	// 2. stop and remove the old container instance
	exists, isRunning := containerExists(s.ContainerName)
	if exists {
		if isRunning {
			if err := stopContainer(s.ContainerName, false); err != nil {
				return err
			}
		}
		if err := removeContainer(s.ContainerName); err != nil {
			return err
		}
	}

	// 3. run a new container with the updated spec
	if err := s.runContainer(logFile); err != nil {
		return err
	}

	// 4. persist the updated spec
	return s.WriteToDisk()
}

func (s *Spec) buildImage(logFile *os.File) error {
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
	buildCmd.Stdout = logFile
	buildCmd.Stderr = logFile
	return buildCmd.Run()
}

func (s *Spec) runContainer(logFile *os.File) error {
	// run using built image
	runArgs := []string{
		"run",
		"-d",
		"--name", s.ContainerName,
		"--hostname", s.Hostname,
		"--label", "stormdrain",
		"--label", fmt.Sprintf("stormdrain.project-path=%s", s.ProjectPath),
		"--label", fmt.Sprintf("stormdrain.profile-name=%s", s.ProfileName),
	}
	if !IsDarwin() {
		// map host UID to the same UID inside the container ("dev" instead of "root")
		// (on macOS Podman's VM handles UID/GID mapping transparenly with virtiofs filesystem sharing)
		runArgs = append(runArgs, "--userns=keep-id")
	}
	if s.ProjectMount {
		projectDirName := filepath.Base(s.ProjectPath)
		runArgs = append(runArgs, "-w", fmt.Sprintf("/home/dev/%s", projectDirName))
		runArgs = append(runArgs, "-v", fmt.Sprintf("%s:/home/dev/%s", s.ProjectPath, projectDirName))
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
	fmt.Fprintf(logFile, "\n### CONTAINER CREATE LOG ###\n")
	runCmd.Stdout = logFile
	runCmd.Stderr = logFile
	return runCmd.Run()
}

// Writes the file as formatted JSON to .stormdrain/pod_spec.json of the project.
func (s *Spec) WriteToDisk() error {
	formattedJson, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	specPath := filepath.Join(s.BuildCtx, "pod_spec.json")
	return os.WriteFile(specPath, formattedJson, 0644)
}

// Kills and deletes the container and its build image. Removes the container-specific
// subdirectory from within the project's .stormdrain directory, and also the parent
// directory if no other containers exists for the project (i.e. the .stormdrain/ dir.
// is empty after the latest deletion).
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
	return removeContainerDirs(s.ProjectPath, s.ContainerName)
}

func removeContainerDirs(projectPath, containerName string) error {
	sdDir := filepath.Join(projectPath, ".stormdrain")
	containerDir := filepath.Join(sdDir, containerName)

	if err := os.RemoveAll(containerDir); err != nil {
		return err
	}

	entries, err := os.ReadDir(sdDir)
	if err != nil {
		// nothing to clean up if parent doesn't exist or isn't readable
		return nil
	}
	if len(entries) == 0 {
		return os.Remove(sdDir)
	}
	return nil
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
func UniqueContainerName(projectName string) (string, string) {
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

func imageExists(imageTag string) bool {
	_, err := exec.Command("podman", "image", "inspect", imageTag, "--format", "{{.Id}}").Output()
	return err == nil
}
