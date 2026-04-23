package internal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

var (
	globalConfigDir, userHome string
)

func init() {
	var err error
	userHome, err = os.UserHomeDir()
	if err != nil {
		fmt.Printf("[!] failed to resolve user home directory: %s\n", err)
		os.Exit(1)
	}
	globalConfigDir = filepath.Join(userHome, ".config", "stormdrain")
}

type Dotfile struct {
	SourcePattern   string   `json:"src"`
	DestinationPath string   `json:"dst"`
	Exclude         []string `json:"exclude"`
}

type Workspace struct {
	DirectMounts   []string        `json:"direct_mounts"`
	VirtualVolumes []VirtualVolume `json:"virtual_volumes"`
}

type VirtualVolume struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type Profile struct {
	Name        string    `json:"name"` // ideally matches the profile's filename for clarity
	Description string    `json:"description"`
	Shell       string    `json:"shell"`
	Packages    []string  `json:"packages"`
	Installers  []string  `json:"installers"`
	Dotfiles    []Dotfile `json:"dotfiles"`  // copied during container image building (i.e. globbing supported)
	Workspace   Workspace `json:"workspace"` // handled with live volume mounts
}

func LoadProfile(profileName string) (*Profile, error) {
	if !strings.HasSuffix(profileName, ".json") {
		profileName += ".json"
	}
	fp := filepath.Join(globalConfigDir, "profiles", profileName)
	file, err := os.Open(fp)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var profile Profile
	parser := json.NewDecoder(file)
	if err = parser.Decode(&profile); err != nil {
		return nil, err
	}

	return &profile, nil
}

// Substitutes the profiles configuration into the base Dockerfile and writes the
// resulting Dockerfile to cwd's .stormdrain/ directory (as 'Dockerfile.sd').
func (p *Profile) DockerfileSubstitution() error {
	baseFp := filepath.Join(globalConfigDir, "Dockerfile.base")
	og, err := os.ReadFile(baseFp)
	if err != nil {
		return err
	}

	res := strings.Replace(string(og), "# {{PROFILE_PKGS}}\n", p.buildPackagesBlock(), 1)
	res = strings.Replace(res, "# {{PROFILE_INSTALLERS}}\n", p.buildInstallersBlock(), 1)
	res = strings.Replace(res, "# {{PROFILE_DOTFILES}}\n", p.buildDotfilesBlock(), 1)

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	sdDir := filepath.Join(cwd, ".stormdrain")
	if err := os.MkdirAll(sdDir, 0755); err != nil {
		return err
	}
	outPath := filepath.Join(sdDir, "Dockerfile.sd")
	if err := os.WriteFile(outPath, []byte(res), 0644); err != nil {
		return err
	}

	return nil
}

func (p *Profile) buildPackagesBlock() string {
	if len(p.Packages) == 0 {
		return ""
	}

	// alternatively we could output multiline command, but that seems unnecessary at least for now
	b := strings.Builder{}
	b.WriteString("RUN sudo apt update && sudo apt install -y --no-install-recommends ")
	for _, pkg := range p.Packages {
		fmt.Fprintf(&b, "%s ", pkg)
	}
	b.WriteString("&& sudo rm -rf /var/lib/apt/lists/*\n")

	return b.String()
}

func (p *Profile) buildInstallersBlock() string {
	b := strings.Builder{}
	for _, inst := range p.Installers {
		fmt.Fprintf(&b, "RUN %s\n", inst)
	}
	return b.String()
}

func (p *Profile) buildDotfilesBlock() string {
	b := strings.Builder{}
	for _, df := range p.Dotfiles {
		// build ctx is $cwd/.stormdrain, dotfiles are temporarily copied to dots/ to allow access
		// NOTE: this assumes all dotfile paths contain '~' (!!!)
		src := strings.Replace(df.SourcePattern, "~", "dots", 1)
		dst := strings.Replace(df.DestinationPath, "~", "/home/dev", 1)
		fmt.Fprintf(&b, "COPY --chown=$UID:$GID %s %s\n", src, dst)
	}
	return b.String()
}

// Copies dotfile sources (specified in profile config) to $cwd/.stormdrain/dots
// so they're accessible from Dockerfile's build context. '~' and env variables
// are expanded as per usual.
//
// E.g. '~/.config/nvim' becomes '$cwd/.stormdrain/dots/.config/nvim'
func (p *Profile) StageDotfiles(cwd string) error {
	if len(p.Dotfiles) == 0 {
		return nil
	}

	dotsDir := filepath.Join(cwd, ".stormdrain", "dots")
	if err := os.MkdirAll(dotsDir, 0755); err != nil {
		return err
	}

	for _, df := range p.Dotfiles {
		src := df.SourcePattern
		src = strings.Replace(src, "~", userHome, 1)
		src = os.ExpandEnv(src) // expand $HOME, $XDG_CONFIG_HOME etc. just in case

		matches, err := filepath.Glob(src)
		if err != nil {
			return fmt.Errorf("dotfile glob failed for '%s': %w", df.SourcePattern, err)
		} else if len(matches) == 0 {
			return fmt.Errorf("dotfile glob pattern '%s' matched no files", df.SourcePattern)
		}

		for _, m := range matches {
			info, err := os.Stat(m)
			if err != nil {
				return err
			}

			relToHome, err := filepath.Rel(userHome, m)
			if err != nil {
				return err
			}
			dstPath := filepath.Join(dotsDir, relToHome)

			if info.IsDir() {
				if err := CopyDir(m, dstPath, df.Exclude); err != nil {
					return err
				}
			} else {
				if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
					return err
				}
				if err := CopyFile(m, dstPath); err != nil {
					return err
				}
			}
		}
	}

	return nil
}

func CleanupStagedDotfiles(cwd string) error {
	return os.RemoveAll(filepath.Join(cwd, ".stormdrain", "dots"))
}

type MountSpec struct {
	HostPath      string `json:"host_path"`
	ContainerPath string `json:"container_path"`
}

// Build configuration for a new container image. Only used during build stage ('new' cmd).
type PodmanSpec struct {
	ContainerName  string            `json:"container_name"`
	Hostname       string            `json:"hostname"`
	ImageTag       string            `json:"image_tag"`
	Shell          string            `json:"shell"`
	ProjectPath    string            `json:"project_path"`
	BuildCtx       string            `json:"-"`
	DotfileDir     string            `json:"-"`
	BuildArgs      map[string]string `json:"build_args"`
	DirectMounts   []MountSpec       `json:"direct_mounts"`
	VirtualVolumes []VirtualVolume   `json:"virtual_volumes"`
}

func (p *Profile) NewPodmanSpec(cwd string) (*PodmanSpec, error) {
	uid := os.Getuid()
	gid := os.Getgid()
	projName := filepath.Base(cwd)
	shell := p.Shell
	if shell == "" {
		shell = "/bin/zsh"
	}

	spec := &PodmanSpec{
		ContainerName: projName,
		Hostname:      RandomHostname(),
		ImageTag:      fmt.Sprintf("stormdrain-%s-%s", p.Name, projName),
		Shell:         shell,
		ProjectPath:   cwd,
		BuildCtx:      filepath.Join(cwd, ".stormdrain"),
		DotfileDir:    filepath.Join(cwd, ".stormdrain", "dots"),
		BuildArgs: map[string]string{
			"UID": strconv.Itoa(uid),
			"GID": strconv.Itoa(gid),
		},
	}

	containerBase := fmt.Sprintf("/home/dev/%s", projName)
	for _, mount := range p.Workspace.DirectMounts {
		hostPath := filepath.Join(cwd, mount)
		if _, err := os.Stat(hostPath); err != nil {
			return nil, err
		}
		spec.DirectMounts = append(spec.DirectMounts, MountSpec{
			HostPath:      hostPath,
			ContainerPath: filepath.Join(containerBase, mount),
		})
	}
	spec.VirtualVolumes = p.Workspace.VirtualVolumes

	return spec, nil
}

func LoadPodmanSpec(cwd string) (*PodmanSpec, error) {
	specPath := filepath.Join(cwd, ".stormdrain", "pod_spec.json")
	raw, err := os.ReadFile(specPath)
	if err != nil {
		return nil, err
	}
	var ps PodmanSpec
	if err = json.Unmarshal(raw, &ps); err != nil {
		return nil, err
	}

	return &ps, nil
}

func (ps *PodmanSpec) WriteToDisk(cwd string) error {
	js, err := json.MarshalIndent(ps, "", "  ")
	if err != nil {
		return err
	}
	specPath := filepath.Join(cwd, ".stormdrain", "pod_spec.json")
	if err = os.WriteFile(specPath, js, 0644); err != nil {
		return err
	}

	return nil
}
