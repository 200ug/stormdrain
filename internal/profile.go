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

type Config struct {
	SourcePattern   string   `json:"src"`
	DestinationPath string   `json:"dst"`
	Exclude         []string `json:"exclude"`
}

type VirtualVolume struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type PortMap struct {
	Host      int `json:"host"`
	Container int `json:"container"`
}

type Profile struct {
	Name           string          `json:"name"` // ideally matches the profile's filename for clarity
	Description    string          `json:"description"`
	Shell          string          `json:"shell"`
	Packages       []string        `json:"packages"`
	Installers     []string        `json:"installers"`
	Configs        []Config        `json:"configs"`       // copied during container image building (-> globbing supported)
	ProjectMount   *bool           `json:"project_mount"` // mount project directory into container (default true)
	Ports          []PortMap       `json:"ports"`         // host <-> container port mappings
	VirtualVolumes []VirtualVolume `json:"virtual_volumes"`
}

func (p *Profile) IsProjectMounted() bool {
	return p.ProjectMount == nil || *p.ProjectMount
}

func LoadProfile(profileName string) (*Profile, error) {
	if !strings.HasSuffix(profileName, ".json") {
		profileName += ".json"
	}
	fp := filepath.Join(globalConfigDir, "profiles", profileName)
	return decodeProfile(fp)
}

func LoadProfileFromPath(path string) (*Profile, error) {
	return decodeProfile(path)
}

func decodeProfile(path string) (*Profile, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var profile Profile
	if err = json.NewDecoder(file).Decode(&profile); err != nil {
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

	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	projName := filepath.Base(cwd)

	res := strings.Replace(string(og), "# {{PROFILE_PKGS}}\n", p.buildPackagesBlock(), 1)
	res = strings.Replace(res, "# {{PROFILE_INSTALLERS}}\n", p.buildInstallersBlock(), 1)
	res = strings.Replace(res, "# {{PROFILE_CONFIGS}}\n", p.buildConfigsBlock(), 1)
	res = strings.Replace(res, "# {{PROFILE_DIRS}}\n", p.buildDirsBlock(projName), 1)

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

func (p *Profile) buildDirsBlock(projName string) string {
	var dirs []string
	if p.IsProjectMounted() {
		dirs = append(dirs, fmt.Sprintf("/home/dev/%s", projName))
	}
	for _, v := range p.VirtualVolumes {
		dirs = append(dirs, v.Path)
	}
	if len(dirs) == 0 {
		return ""
	}

	b := strings.Builder{}
	b.WriteString("RUN mkdir -p")
	for _, d := range dirs {
		fmt.Fprintf(&b, " %s", d)
	}
	b.WriteString(" && chown -R $UID:$GID /home/$USERNAME")
	for _, d := range dirs {
		if !strings.HasPrefix(d, "/home/") {
			fmt.Fprintf(&b, " && chown $UID:$GID %s", d)
		}
	}
	b.WriteString("\n")

	return b.String()
}

func (p *Profile) buildConfigsBlock() string {
	b := strings.Builder{}
	for _, df := range p.Configs {
		// build ctx is $cwd/.stormdrain, configs are temporarily copied to configs/ to allow access
		// NOTE: this assumes all config paths contain '~' (!!!)
		src := strings.Replace(df.SourcePattern, "~", "configs", 1)
		dst := strings.Replace(df.DestinationPath, "~", "/home/dev", 1)
		fmt.Fprintf(&b, "COPY --chown=$UID:$GID %s %s\n", src, dst)
	}
	return b.String()
}

// Copies config sources (specified in profile config) to $cwd/.stormdrain/configs
// so they're accessible from Dockerfile's build context. '~' and env variables
// are expanded as per usual.
//
// E.g. '~/.config/nvim' becomes '$cwd/.stormdrain/configs/.config/nvim'
func (p *Profile) StageConfigs(cwd string) error {
	if len(p.Configs) == 0 {
		return nil
	}

	configsDir := filepath.Join(cwd, ".stormdrain", "configs")
	if err := os.MkdirAll(configsDir, 0755); err != nil {
		return err
	}

	for _, df := range p.Configs {
		src := df.SourcePattern
		src = strings.Replace(src, "~", userHome, 1)
		src = os.ExpandEnv(src) // expand $HOME, $XDG_CONFIG_HOME etc. just in case

		matches, err := filepath.Glob(src)
		if err != nil {
			return fmt.Errorf("config glob failed for '%s': %w", df.SourcePattern, err)
		} else if len(matches) == 0 {
			return fmt.Errorf("config glob pattern '%s' matched no files", df.SourcePattern)
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
			dstPath := filepath.Join(configsDir, relToHome)

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

func CleanupStagedConfigs(cwd string) error {
	return os.RemoveAll(filepath.Join(cwd, ".stormdrain", "configs"))
}

// Build configuration for a new container image. Only used during build stage ('new' cmd).
type PodmanSpec struct {
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
		ConfigsDir:    filepath.Join(cwd, ".stormdrain", "configs"),
		BuildArgs: map[string]string{
			"UID": strconv.Itoa(uid),
			"GID": strconv.Itoa(gid),
		},
	}

	spec.ProjectMount = p.IsProjectMounted()
	spec.VirtualVolumes = p.VirtualVolumes
	spec.Ports = p.Ports

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
