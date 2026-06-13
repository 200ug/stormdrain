package manager

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"codeberg.org/2ug/stormdrain/internal/util"
)

type Config struct {
	SourcePattern   string   `json:"src"`
	DestinationPath string   `json:"dst"`
	Exclude         []string `json:"exclude"`
}

type PortMap struct {
	Host      int `json:"host"`
	Container int `json:"container"`
}

type VirtualVolume struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

type Profile struct {
	Name           string          `json:"name"` // ideally matches the profile's filename for clarity
	Description    string          `json:"description"`
	Shell          string          `json:"shell"`
	Packages       []string        `json:"packages"`
	Installers     []string        `json:"installers"`
	Configs        []Config        `json:"configs"`       // globbing supported
	ProjectMount   *bool           `json:"project_mount"` // defaults to true
	Ports          []PortMap       `json:"ports"`
	VirtualVolumes []VirtualVolume `json:"virtual_volumes"`
	EnvFiles       []string        `json:"env_files"` // injected at runtime
}

func LoadProfile(configsDir, profileName string) (*Profile, error) {
	if !strings.HasSuffix(profileName, ".json") {
		profileName += ".json"
	}
	fp := filepath.Join(configsDir, "profiles", profileName)
	return decodeProfile(fp)
}

func LoadProfileFromPath(path string) (*Profile, error) {
	return decodeProfile(path)
}

// Substitutes the profile's configuration into the base Dockerfile and writes the
// resulting Dockerfile to project's .stormdrain/ directory as Dockerfile.sd.
func (p *Profile) SubstituteDockerfileTemplate(configsDir, projectPath, containerName string) error {
	templatePath := filepath.Join(configsDir, "Dockerfile.base")
	dfTemplate, err := os.ReadFile(templatePath)
	if err != nil {
		return err
	}
	projectName := filepath.Base(projectPath)
	dfSubst := strings.Replace(string(dfTemplate), "# {{PROFILE_PKGS}}\n", p.buildPackagesBlock(), 1)
	dfSubst = strings.Replace(dfSubst, "# {{PROFILE_INSTALLERS}}\n", p.buildInstallersBlock(), 1)
	dfSubst = strings.Replace(dfSubst, "# {{PROFILE_CONFIGS}}\n", p.buildConfigsBlock(), 1)
	dfSubst = strings.Replace(dfSubst, "# {{PROFILE_DIRS}}\n", p.buildDirsBlock(projectName), 1)

	sdDir := filepath.Join(projectPath, ".stormdrain", containerName)
	if err := os.MkdirAll(sdDir, 0755); err != nil {
		return err
	}
	outPath := filepath.Join(sdDir, "Dockerfile.sd")
	return os.WriteFile(outPath, []byte(dfSubst), 0644)
}

func (p *Profile) buildPackagesBlock() string {
	if len(p.Packages) == 0 {
		return ""
	}
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

func (p *Profile) buildDirsBlock(projectName string) string {
	var dirs []string
	if p.ProjectMount == nil || *p.ProjectMount {
		dirs = append(dirs, fmt.Sprintf("/home/dev/%s", projectName))
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
		// NOTE: we should blindly trust that these files will be in .stormdrain/configs/
		//		 at container image build stage
		src := strings.Replace(df.SourcePattern, "~", "configs", 1)
		dst := strings.Replace(df.DestinationPath, "~", "/home/dev", 1)
		fmt.Fprintf(&b, "COPY --chown=$UID:$GID %s %s\n", src, dst)
	}
	return b.String()
}

// Copies configs from their original locations (specified in profile config) to
// project's .stormdrain/configs temp. directory so that they're in the correct
// build context (thus accessible by Podman). The order between this method and
// SubstituteDockerfileTemplate doesn't as the substitution stage doesn't check
// the existence of the files it writes to COPY commands.
//
// E.g. ~/.config/nvim becomes $projectPath/.stormdrain/configs/.config/nvim
func (p *Profile) StageConfigs(userHome, projectPath, containerName string) error {
	if len(p.Configs) == 0 {
		return nil
	}

	tmpConfigsDir := filepath.Join(projectPath, ".stormdrain", containerName, "configs")
	if err := os.MkdirAll(tmpConfigsDir, 0755); err != nil {
		return err
	}

	for _, cf := range p.Configs {
		src := cf.SourcePattern
		src = strings.Replace(src, "~", userHome, 1)
		matches, err := filepath.Glob(src)
		if err != nil {
			return err
		} else if len(matches) == 0 {
			return fmt.Errorf("config glob pattern %q returned no matches", cf.SourcePattern)
		}

		for _, match := range matches {
			if err = p.handleGlobMatchCopy(userHome, tmpConfigsDir, match, cf.Exclude); err != nil {
				return err
			}
		}
	}
	return nil
}

func (p *Profile) handleGlobMatchCopy(userHome, tmpConfigsDir, match string, excludes []string) error {
	info, err := os.Stat(match)
	if err != nil {
		return err
	}
	relToHome, err := filepath.Rel(userHome, match)
	if err != nil {
		return err
	}
	dstPath := filepath.Join(tmpConfigsDir, relToHome)
	if info.IsDir() {
		if err := util.CopyDir(match, dstPath, excludes); err != nil {
			return err
		}
	} else {
		if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
			return err
		}
		if err := util.CopyFile(match, dstPath); err != nil {
			return err
		}
	}
	return nil
}

func CleanupStagedConfigs(projectPath, containerName string) error {
	return os.RemoveAll(filepath.Join(projectPath, ".stormdrain", containerName, "configs"))
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
