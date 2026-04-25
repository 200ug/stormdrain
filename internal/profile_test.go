package internal

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// build functions

func TestBuildPackagesBlockWithPackages(t *testing.T) {
	p := &Profile{
		Packages: []string{"ripgrep", "fzf", "tmux"},
	}
	result := p.buildPackagesBlock()
	if !strings.HasPrefix(result, "RUN sudo apt update && sudo apt install -y --no-install-recommends ") {
		t.Fatalf("unexpected prefix: %q", result)
	}
	if !strings.HasSuffix(result, "&& sudo rm -rf /var/lib/apt/lists/*\n") {
		t.Fatalf("unexpected suffix: %q", result)
	}
	for _, pkg := range p.Packages {
		if !strings.Contains(result, pkg) {
			t.Errorf("package %q not found in output", pkg)
		}
	}
}

func TestBuildPackagesBlockEmpty(t *testing.T) {
	p := &Profile{}
	if result := p.buildPackagesBlock(); result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestBuildInstallersBlockWithInstallers(t *testing.T) {
	p := &Profile{
		Installers: []string{
			"curl -sL https://example.com/install.sh | bash",
			"sudo tar -C /usr/local -xzf /tmp/archive.tar.gz",
		},
	}
	result := p.buildInstallersBlock()
	for _, inst := range p.Installers {
		expected := "RUN " + inst + "\n"
		if !strings.Contains(result, expected) {
			t.Errorf("installer %q not found in output", inst)
		}
	}
}

func TestBuildInstallersBlockEmpty(t *testing.T) {
	p := &Profile{}
	if result := p.buildInstallersBlock(); result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

func TestBuildDotfilesBlockWithDotfiles(t *testing.T) {
	p := &Profile{
		Dotfiles: []Dotfile{
			{SourcePattern: "~/.config/nvim", DestinationPath: "~/.config/nvim"},
			{SourcePattern: "~/.zshrc", DestinationPath: "~/."},
		},
	}
	result := p.buildDotfilesBlock()
	expectedLines := []string{
		"COPY --chown=$UID:$GID dots/.config/nvim /home/dev/.config/nvim\n",
		"COPY --chown=$UID:$GID dots/.zshrc /home/dev/.\n",
	}
	for _, line := range expectedLines {
		if !strings.Contains(result, line) {
			t.Errorf("expected line %q not found in output: %q", line, result)
		}
	}
}

func TestBuildDotfilesBlockEmpty(t *testing.T) {
	p := &Profile{}
	if result := p.buildDotfilesBlock(); result != "" {
		t.Errorf("expected empty string, got %q", result)
	}
}

// build dirs block

func TestBuildDirsBlockEmpty(t *testing.T) {
	projectMount := false
	p := &Profile{ProjectMount: &projectMount}
	if result := p.buildDirsBlock("myproject"); result != "" {
		t.Errorf("expected empty string for disabled project mount and no volumes, got %q", result)
	}
}

func TestBuildDirsBlockWithWorkspace(t *testing.T) {
	projectMount := true
	p := &Profile{
		ProjectMount: &projectMount,
		VirtualVolumes: []VirtualVolume{
			{Name: "go-mod-cache", Path: "/go/pkg/mod"},
		},
	}
	result := p.buildDirsBlock("myproject")
	if !strings.Contains(result, "mkdir -p /home/dev/myproject /go/pkg/mod") {
		t.Errorf("expected mkdir with both dirs, got %q", result)
	}
	if !strings.Contains(result, "chown -R $UID:$GID /home/$USERNAME") {
		t.Errorf("expected recursive chown on /home/$USERNAME, got %q", result)
	}
	if !strings.Contains(result, "chown $UID:$GID /go/pkg/mod") {
		t.Errorf("expected individual chown for /go/pkg/mod, got %q", result)
	}
}

func TestBuildDirsBlockVirtualOnly(t *testing.T) {
	projectMount := false
	p := &Profile{
		ProjectMount: &projectMount,
		VirtualVolumes: []VirtualVolume{
			{Name: "go-mod-cache", Path: "/go/pkg/mod"},
			{Name: "go-build-cache", Path: "/home/dev/.cache/go-build"},
		},
	}
	result := p.buildDirsBlock("myproject")
	if strings.Contains(result, "/home/dev/myproject") {
		t.Errorf("workdir should not appear without project mount, got %q", result)
	}
	if !strings.Contains(result, "/go/pkg/mod") || !strings.Contains(result, "/home/dev/.cache/go-build") {
		t.Errorf("expected both virtual volume paths, got %q", result)
	}
	if !strings.Contains(result, "chown -R $UID:$GID /home/$USERNAME") {
		t.Errorf("expected recursive chown on /home/$USERNAME, got %q", result)
	}
	if strings.Contains(result, "chown $UID:$GID /home/dev") {
		t.Errorf("paths under /home/ should not get individual chown, got %q", result)
	}
	if !strings.Contains(result, "chown $UID:$GID /go/pkg/mod") {
		t.Errorf("paths outside /home/ should get individual chown, got %q", result)
	}
}

// substitution

func TestDockerfileSubstitution(t *testing.T) {
	origGlobalConfigDir := globalConfigDir
	origWd, _ := os.Getwd()
	t.Cleanup(func() {
		globalConfigDir = origGlobalConfigDir
		os.Chdir(origWd)
	})

	configDir := t.TempDir()
	profilesDir := filepath.Join(configDir, "profiles")
	os.MkdirAll(profilesDir, 0755)
	globalConfigDir = configDir

	dockerfileBase := `FROM test:latest
ARG UID=1000
ARG GID=1000
USER dev
# {{PROFILE_PKGS}}
# {{PROFILE_INSTALLERS}}
# {{PROFILE_DOTFILES}}
# {{PROFILE_DIRS}}
CMD ["sleep", "infinity"]
`
	if err := os.WriteFile(filepath.Join(configDir, "Dockerfile.base"), []byte(dockerfileBase), 0644); err != nil {
		t.Fatal(err)
	}

	workDir := t.TempDir()
	os.Chdir(workDir)

	p := &Profile{
		Packages:   []string{"ripgrep"},
		Installers: []string{"echo hello"},
		Dotfiles: []Dotfile{
			{SourcePattern: "~/.zshrc", DestinationPath: "~/."},
		},
	}

	if err := p.DockerfileSubstitution(); err != nil {
		t.Fatal(err)
	}

	result, err := os.ReadFile(filepath.Join(workDir, ".stormdrain", "Dockerfile.sd"))
	if err != nil {
		t.Fatal(err)
	}

	resultStr := string(result)
	if strings.Contains(resultStr, "# {{PROFILE_PKGS}}") {
		t.Error("PROFILE_PKGS marker not replaced")
	}
	if strings.Contains(resultStr, "# {{PROFILE_INSTALLERS}}") {
		t.Error("PROFILE_INSTALLERS marker not replaced")
	}
	if strings.Contains(resultStr, "# {{PROFILE_DOTFILES}}") {
		t.Error("PROFILE_DOTFILES marker not replaced")
	}
	if strings.Contains(resultStr, "# {{PROFILE_DIRS}}") {
		t.Error("PROFILE_DIRS marker not replaced")
	}
	if !strings.Contains(resultStr, "RUN sudo apt update && sudo apt install -y --no-install-recommends") {
		t.Error("packages block not present in output")
	}
	if !strings.Contains(resultStr, "RUN echo hello\n") {
		t.Error("installer block not present in output")
	}
	if !strings.Contains(resultStr, "COPY --chown=$UID:$GID dots/.zshrc /home/dev/.") {
		t.Error("dotfiles block not present in output")
	}
}

func TestDockerfileSubstitutionEmptyProfile(t *testing.T) {
	origGlobalConfigDir := globalConfigDir
	origWd, _ := os.Getwd()
	t.Cleanup(func() {
		globalConfigDir = origGlobalConfigDir
		os.Chdir(origWd)
	})

	configDir := t.TempDir()
	globalConfigDir = configDir

	dockerfileBase := `FROM test:latest
USER dev
# {{PROFILE_PKGS}}
# {{PROFILE_INSTALLERS}}
# {{PROFILE_DOTFILES}}
# {{PROFILE_DIRS}}
CMD ["sleep", "infinity"]
`
	if err := os.WriteFile(filepath.Join(configDir, "Dockerfile.base"), []byte(dockerfileBase), 0644); err != nil {
		t.Fatal(err)
	}

	workDir := t.TempDir()
	os.Chdir(workDir)

	p := &Profile{}
	if err := p.DockerfileSubstitution(); err != nil {
		t.Fatal(err)
	}

	result, err := os.ReadFile(filepath.Join(workDir, ".stormdrain", "Dockerfile.sd"))
	if err != nil {
		t.Fatal(err)
	}

	resultStr := string(result)
	if strings.Contains(resultStr, "{{PROFILE_") {
		t.Error("unreplaced markers remain in output")
	}
}

// dotfile staging

func TestStageDotfilesFileCopy(t *testing.T) {
	origUserHome := userHome
	t.Cleanup(func() { userHome = origUserHome })

	homeDir := t.TempDir()
	userHome = homeDir

	os.MkdirAll(filepath.Join(homeDir, ".config", "nvim"), 0755)
	os.WriteFile(filepath.Join(homeDir, ".config", "nvim", "init.lua"), []byte("nvim config"), 0644)
	os.WriteFile(filepath.Join(homeDir, ".zshrc"), []byte("zsh config"), 0644)

	workDir := t.TempDir()

	p := &Profile{
		Dotfiles: []Dotfile{
			{SourcePattern: "~/.zshrc", DestinationPath: "~/."},
			{SourcePattern: "~/.config/nvim", DestinationPath: "~/.config/nvim"},
		},
	}

	if err := p.StageDotfiles(workDir); err != nil {
		t.Fatal(err)
	}

	dotsDir := filepath.Join(workDir, ".stormdrain", "dots")

	zshrcDst := filepath.Join(dotsDir, ".zshrc")
	data, err := os.ReadFile(zshrcDst)
	if err != nil {
		t.Fatalf("zshrc not found at %s: %v", zshrcDst, err)
	}
	if string(data) != "zsh config" {
		t.Errorf("zshrc content mismatch: got %q", string(data))
	}

	initLuaDst := filepath.Join(dotsDir, ".config", "nvim", "init.lua")
	data, err = os.ReadFile(initLuaDst)
	if err != nil {
		t.Fatalf("init.lua not found at %s: %v", initLuaDst, err)
	}
	if string(data) != "nvim config" {
		t.Errorf("init.lua content mismatch: got %q", string(data))
	}
}

func TestStageDotfilesEmpty(t *testing.T) {
	p := &Profile{}
	workDir := t.TempDir()
	if err := p.StageDotfiles(workDir); err != nil {
		t.Fatalf("expected nil error for empty dotfiles, got %v", err)
	}
}

func TestStageDotfilesGlobPattern(t *testing.T) {
	origUserHome := userHome
	t.Cleanup(func() { userHome = origUserHome })

	homeDir := t.TempDir()
	userHome = homeDir

	zshDir := filepath.Join(homeDir, ".config", "zsh")
	os.MkdirAll(zshDir, 0755)
	os.WriteFile(filepath.Join(zshDir, "aliases.zsh"), []byte("alias ll='ls -la'"), 0644)
	os.WriteFile(filepath.Join(zshDir, "env.zsh"), []byte("export PATH=$HOME/bin:$PATH"), 0644)

	workDir := t.TempDir()

	p := &Profile{
		Dotfiles: []Dotfile{
			{SourcePattern: "~/.config/zsh/*", DestinationPath: "~/."},
		},
	}

	if err := p.StageDotfiles(workDir); err != nil {
		t.Fatal(err)
	}

	dotsDir := filepath.Join(workDir, ".stormdrain", "dots")
	aliases := filepath.Join(dotsDir, ".config", "zsh", "aliases.zsh")
	env := filepath.Join(dotsDir, ".config", "zsh", "env.zsh")

	if _, err := os.Stat(aliases); err != nil {
		t.Errorf("aliases.zsh not staged: %v", err)
	}
	if _, err := os.Stat(env); err != nil {
		t.Errorf("env.zsh not staged: %v", err)
	}
}

func TestStageDotfilesWithExclude(t *testing.T) {
	origUserHome := userHome
	t.Cleanup(func() { userHome = origUserHome })

	homeDir := t.TempDir()
	userHome = homeDir

	nvimDir := filepath.Join(homeDir, ".config", "nvim")
	pluginDir := filepath.Join(nvimDir, "plugin")
	luaDir := filepath.Join(nvimDir, "lua")
	os.MkdirAll(pluginDir, 0755)
	os.MkdirAll(luaDir, 0755)
	os.WriteFile(filepath.Join(nvimDir, "init.lua"), []byte("init"), 0644)
	os.WriteFile(filepath.Join(luaDir, "plugins.lua"), []byte("plugins"), 0644)
	os.WriteFile(filepath.Join(pluginDir, "packer_compiled.lua"), []byte("compiled"), 0644)

	workDir := t.TempDir()

	p := &Profile{
		Dotfiles: []Dotfile{
			{SourcePattern: "~/.config/nvim", DestinationPath: "~/.config/nvim", Exclude: []string{"plugin"}},
		},
	}

	if err := p.StageDotfiles(workDir); err != nil {
		t.Fatal(err)
	}

	dotsDir := filepath.Join(workDir, ".stormdrain", "dots")

	initData, err := os.ReadFile(filepath.Join(dotsDir, ".config", "nvim", "init.lua"))
	if err != nil {
		t.Fatal(err)
	}
	if string(initData) != "init" {
		t.Errorf("init.lua: got %q, want %q", string(initData), "init")
	}

	pluginsData, err := os.ReadFile(filepath.Join(dotsDir, ".config", "nvim", "lua", "plugins.lua"))
	if err != nil {
		t.Fatal(err)
	}
	if string(pluginsData) != "plugins" {
		t.Errorf("plugins.lua: got %q, want %q", string(pluginsData), "plugins")
	}

	if _, err := os.Stat(filepath.Join(dotsDir, ".config", "nvim", "plugin")); !os.IsNotExist(err) {
		t.Error("plugin directory should be excluded")
	}
}

func TestCleanupStagedDotfiles(t *testing.T) {
	workDir := t.TempDir()
	dotsDir := filepath.Join(workDir, ".stormdrain", "dots")
	os.MkdirAll(dotsDir, 0755)
	os.WriteFile(filepath.Join(dotsDir, "test"), []byte("data"), 0644)

	if err := CleanupStagedDotfiles(workDir); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(dotsDir); !os.IsNotExist(err) {
		t.Error("dots directory should be removed after cleanup")
	}
}

// podman spec

func TestNewPodmanSpecWithWorkspace(t *testing.T) {
	origWd, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(origWd) })

	workDir := t.TempDir()
	os.Chdir(workDir)

	projectMount := true
	p := &Profile{
		Name:         "golang",
		Shell:        "/bin/zsh",
		ProjectMount: &projectMount,
		VirtualVolumes: []VirtualVolume{
			{Name: "go-mod-cache", Path: "/go/pkg/mod"},
		},
	}

	spec, err := p.NewPodmanSpec(workDir)
	if err != nil {
		t.Fatal(err)
	}

	projName := filepath.Base(workDir)
	if spec.ContainerName != projName {
		t.Errorf("ContainerName: got %q, want %q", spec.ContainerName, projName)
	}
	if spec.ImageTag != "stormdrain-golang-"+projName {
		t.Errorf("ImageTag: got %q, want %q", spec.ImageTag, "stormdrain-golang-"+projName)
	}
	if spec.Hostname == "" {
		t.Error("Hostname should not be empty")
	}
	found := false
	for _, h := range Hostnames {
		if h == spec.Hostname {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Hostname %q not in Hostnames list", spec.Hostname)
	}
	if spec.Shell != "/bin/zsh" {
		t.Errorf("Shell: got %q, want %q", spec.Shell, "/bin/zsh")
	}
	if spec.BuildArgs["UID"] == "" || spec.BuildArgs["GID"] == "" {
		t.Error("BuildArgs UID or GID is empty")
	}
	if !spec.ProjectMount {
		t.Error("ProjectMount: got false, want true")
	}
	if spec.VirtualVolumes[0].Name != "go-mod-cache" {
		t.Errorf("VirtualVolumes[0].Name: got %q", spec.VirtualVolumes[0].Name)
	}
}

func TestNewPodmanSpecNoWorkspace(t *testing.T) {
	origWd, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(origWd) })

	workDir := t.TempDir()
	os.Chdir(workDir)

	p := &Profile{
		Name:  "default",
		Shell: "/bin/zsh",
	}

	spec, err := p.NewPodmanSpec(workDir)
	if err != nil {
		t.Fatal(err)
	}

	if !spec.ProjectMount {
		t.Error("ProjectMount: got false, want true (default)")
	}
	if len(spec.VirtualVolumes) != 0 {
		t.Errorf("VirtualVolumes: got %d, want 0", len(spec.VirtualVolumes))
	}
	if spec.BuildCtx != filepath.Join(workDir, ".stormdrain") {
		t.Errorf("BuildCtx: got %q, want %q", spec.BuildCtx, filepath.Join(workDir, ".stormdrain"))
	}
	if spec.DotfileDir != filepath.Join(workDir, ".stormdrain", "dots") {
		t.Errorf("DotfileDir: got %q, want %q", spec.DotfileDir, filepath.Join(workDir, ".stormdrain", "dots"))
	}
}

func TestNewPodmanSpecDefaultShell(t *testing.T) {
	origWd, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(origWd) })

	workDir := t.TempDir()
	os.Chdir(workDir)

	p := &Profile{
		Name:  "minimal",
		Shell: "",
	}

	spec, err := p.NewPodmanSpec(workDir)
	if err != nil {
		t.Fatal(err)
	}

	if spec.Shell != "/bin/zsh" {
		t.Errorf("Shell default: got %q, want %q", spec.Shell, "/bin/zsh")
	}
}

func TestNewPodmanSpecProjectMountDisabled(t *testing.T) {
	origWd, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(origWd) })

	workDir := t.TempDir()
	os.Chdir(workDir)

	projectMount := false
	p := &Profile{
		Name:         "test",
		ProjectMount: &projectMount,
	}

	spec, err := p.NewPodmanSpec(workDir)
	if err != nil {
		t.Fatal(err)
	}
	if spec.ProjectMount {
		t.Error("ProjectMount: got true, want false")
	}
}

func TestNewPodmanSpecWithPorts(t *testing.T) {
	origWd, _ := os.Getwd()
	t.Cleanup(func() { os.Chdir(origWd) })

	workDir := t.TempDir()
	os.Chdir(workDir)

	p := &Profile{
		Name:  "web",
		Shell: "/bin/zsh",
		Ports: []PortMap{
			{Host: 8080, Container: 3000},
			{Host: 5432, Container: 5432},
		},
	}

	spec, err := p.NewPodmanSpec(workDir)
	if err != nil {
		t.Fatal(err)
	}

	if len(spec.Ports) != 2 {
		t.Fatalf("Ports: got %d, want 2", len(spec.Ports))
	}
	if spec.Ports[0].Host != 8080 || spec.Ports[0].Container != 3000 {
		t.Errorf("Ports[0]: got host=%d container=%d, want host=8080 container=3000",
			spec.Ports[0].Host, spec.Ports[0].Container)
	}
	if spec.Ports[1].Host != 5432 || spec.Ports[1].Container != 5432 {
		t.Errorf("Ports[1]: got host=%d container=%d, want host=5432 container=5432",
			spec.Ports[1].Host, spec.Ports[1].Container)
	}
}

// spec round-trip

func TestPodmanSpecRoundTrip(t *testing.T) {
	workDir := t.TempDir()
	os.MkdirAll(filepath.Join(workDir, ".stormdrain"), 0755)

	original := &PodmanSpec{
		ContainerName: "myproject",
		Hostname:      "akarso",
		ImageTag:      "stormdrain-golang-myproject",
		Shell:         "/bin/zsh",
		ProjectPath:   "/home/user/project",
		ProjectMount:  true,
		BuildCtx:      "/tmp/.stormdrain",
		DotfileDir:    "/tmp/.stormdrain/dots",
		BuildArgs: map[string]string{
			"UID": "1000",
			"GID": "1000",
		},
		VirtualVolumes: []VirtualVolume{
			{Name: "go-mod-cache", Path: "/go/pkg/mod"},
		},
		Ports: []PortMap{
			{Host: 8080, Container: 3000},
		},
	}

	if err := original.WriteToDisk(workDir); err != nil {
		t.Fatal(err)
	}

	loaded, err := LoadPodmanSpec(workDir)
	if err != nil {
		t.Fatal(err)
	}

	if loaded.ContainerName != original.ContainerName {
		t.Errorf("ContainerName: got %q, want %q", loaded.ContainerName, original.ContainerName)
	}
	if loaded.Hostname != original.Hostname {
		t.Errorf("Hostname: got %q, want %q", loaded.Hostname, original.Hostname)
	}
	if loaded.ImageTag != original.ImageTag {
		t.Errorf("ImageTag: got %q, want %q", loaded.ImageTag, original.ImageTag)
	}
	if loaded.Shell != original.Shell {
		t.Errorf("Shell: got %q, want %q", loaded.Shell, original.Shell)
	}
	if loaded.ProjectPath != original.ProjectPath {
		t.Errorf("ProjectPath: got %q, want %q", loaded.ProjectPath, original.ProjectPath)
	}
	if loaded.BuildArgs["UID"] != original.BuildArgs["UID"] {
		t.Errorf("BuildArgs UID: got %q, want %q", loaded.BuildArgs["UID"], original.BuildArgs["UID"])
	}
	if loaded.ProjectMount != original.ProjectMount {
		t.Errorf("ProjectMount: got %v, want %v", loaded.ProjectMount, original.ProjectMount)
	}
	if len(loaded.VirtualVolumes) != len(original.VirtualVolumes) {
		t.Errorf("VirtualVolumes length: got %d, want %d", len(loaded.VirtualVolumes), len(original.VirtualVolumes))
	}
	if loaded.VirtualVolumes[0].Name != original.VirtualVolumes[0].Name {
		t.Errorf("VirtualVolumes[0].Name: got %q, want %q", loaded.VirtualVolumes[0].Name, original.VirtualVolumes[0].Name)
	}
	if loaded.BuildCtx != "" {
		t.Errorf("BuildCtx should be excluded from serialization, got %q", loaded.BuildCtx)
	}
	if loaded.DotfileDir != "" {
		t.Errorf("DotfileDir should be excluded from serialization, got %q", loaded.DotfileDir)
	}
	if len(loaded.Ports) != len(original.Ports) {
		t.Errorf("Ports length: got %d, want %d", len(loaded.Ports), len(original.Ports))
	}
	if loaded.Ports[0].Host != original.Ports[0].Host || loaded.Ports[0].Container != original.Ports[0].Container {
		t.Errorf("Ports[0]: got host=%d container=%d, want host=%d container=%d",
			loaded.Ports[0].Host, loaded.Ports[0].Container, original.Ports[0].Host, original.Ports[0].Container)
	}
}

// load profile

func TestLoadProfile(t *testing.T) {
	origGlobalConfigDir := globalConfigDir
	t.Cleanup(func() { globalConfigDir = origGlobalConfigDir })

	configDir := t.TempDir()
	globalConfigDir = configDir
	profilesDir := filepath.Join(configDir, "profiles")
	os.MkdirAll(profilesDir, 0755)

	profileData := Profile{
		Name:        "test",
		Description: "test profile",
		Shell:       "/bin/bash",
		Packages:    []string{"vim", "git"},
		Installers:  []string{"curl -sL https://example.com | bash"},
		Dotfiles: []Dotfile{
			{SourcePattern: "~/.bashrc", DestinationPath: "~/."},
		},
		VirtualVolumes: []VirtualVolume{{Name: "cache", Path: "/var/cache"}},
	}

	data, _ := json.Marshal(profileData)
	os.WriteFile(filepath.Join(profilesDir, "test.json"), data, 0644)

	profile, err := LoadProfile("test")
	if err != nil {
		t.Fatal(err)
	}

	if profile.Name != "test" {
		t.Errorf("Name: got %q, want %q", profile.Name, "test")
	}
	if profile.Shell != "/bin/bash" {
		t.Errorf("Shell: got %q, want %q", profile.Shell, "/bin/bash")
	}
	if len(profile.Packages) != 2 {
		t.Errorf("Packages length: got %d, want 2", len(profile.Packages))
	}
	if profile.VirtualVolumes[0].Name != "cache" {
		t.Errorf("VirtualVolumes[0].Name: got %q, want %q", profile.VirtualVolumes[0].Name, "cache")
	}
}

func TestLoadProfileWithExtension(t *testing.T) {
	origGlobalConfigDir := globalConfigDir
	t.Cleanup(func() { globalConfigDir = origGlobalConfigDir })

	configDir := t.TempDir()
	globalConfigDir = configDir
	profilesDir := filepath.Join(configDir, "profiles")
	os.MkdirAll(profilesDir, 0755)

	profileData := Profile{Name: "extended"}
	data, _ := json.Marshal(profileData)
	os.WriteFile(filepath.Join(profilesDir, "extended.json"), data, 0644)

	_, err := LoadProfile("extended.json")
	if err != nil {
		t.Errorf("should not fail when .json extension is provided: %v", err)
	}
}

func TestLoadProfileNotFound(t *testing.T) {
	origGlobalConfigDir := globalConfigDir
	t.Cleanup(func() { globalConfigDir = origGlobalConfigDir })

	configDir := t.TempDir()
	globalConfigDir = configDir

	_, err := LoadProfile("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent profile, got nil")
	}
}

func TestLoadProfileFromPath(t *testing.T) {
	tmpDir := t.TempDir()

	profileData := Profile{
		Name:        "frompath",
		Description: "loaded via direct path",
		Shell:       "/bin/zsh",
		Packages:    []string{"ripgrep"},
		Ports: []PortMap{
			{Host: 8080, Container: 3000},
		},
	}
	data, _ := json.Marshal(profileData)
	fp := filepath.Join(tmpDir, "my-profile.json")
	os.WriteFile(fp, data, 0644)

	profile, err := LoadProfileFromPath(fp)
	if err != nil {
		t.Fatal(err)
	}
	if profile.Name != "frompath" {
		t.Errorf("Name: got %q, want %q", profile.Name, "frompath")
	}
	if profile.Shell != "/bin/zsh" {
		t.Errorf("Shell: got %q, want %q", profile.Shell, "/bin/zsh")
	}
	if len(profile.Ports) != 1 || profile.Ports[0].Host != 8080 {
		t.Errorf("Ports: got %v, want [{Host:8080 Container:3000}]", profile.Ports)
	}
}

func TestLoadProfileFromPathNotFound(t *testing.T) {
	_, err := LoadProfileFromPath("/nonexistent/path/profile.json")
	if err == nil {
		t.Error("expected error for nonexistent path, got nil")
	}
}
