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
	p := &Profile{}
	if result := p.buildDirsBlock("myproject"); result != "" {
		t.Errorf("expected empty string for empty workspace, got %q", result)
	}
}

func TestBuildDirsBlockWithWorkspace(t *testing.T) {
	p := &Profile{
		Workspace: Workspace{
			DirectMounts: []string{"cmd", "go.mod"},
			VirtualVolumes: []VirtualVolume{
				{Name: "go-mod-cache", Path: "/go/pkg/mod"},
			},
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
	p := &Profile{
		Workspace: Workspace{
			VirtualVolumes: []VirtualVolume{
				{Name: "go-mod-cache", Path: "/go/pkg/mod"},
				{Name: "go-build-cache", Path: "/home/dev/.cache/go-build"},
			},
		},
	}
	result := p.buildDirsBlock("myproject")
	if strings.Contains(result, "/home/dev/myproject") {
		t.Errorf("workdir should not appear without direct mounts, got %q", result)
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
	os.MkdirAll(filepath.Join(workDir, "cmd"), 0755)
	os.WriteFile(filepath.Join(workDir, "go.mod"), []byte("module test"), 0644)
	os.Chdir(workDir)

	p := &Profile{
		Name:  "golang",
		Shell: "/bin/zsh",
		Workspace: Workspace{
			DirectMounts: []string{"cmd", "go.mod"},
			VirtualVolumes: []VirtualVolume{
				{Name: "go-mod-cache", Path: "/go/pkg/mod"},
			},
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
	if len(spec.DirectMounts) != 2 {
		t.Fatalf("DirectMounts: got %d, want 2", len(spec.DirectMounts))
	}
	if spec.DirectMounts[0].HostPath != filepath.Join(workDir, "cmd") {
		t.Errorf("DirectMounts[0].HostPath: got %q", spec.DirectMounts[0].HostPath)
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

	if len(spec.DirectMounts) != 0 {
		t.Errorf("DirectMounts: got %d, want 0", len(spec.DirectMounts))
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

func TestNewPodmanSpecMountPathNotExist(t *testing.T) {
	os.Stdout = nil
	workDir := t.TempDir()

	p := &Profile{
		Name: "test",
		Workspace: Workspace{
			DirectMounts: []string{"nonexistent_dir"},
		},
	}

	spec, err := p.NewPodmanSpec(workDir)
	if err != nil {
		t.Fatalf("expected nil error for nonexistent mount path, got %v", err)
	}
	if len(spec.DirectMounts) != 0 {
		t.Errorf("expected DirectMounts to be empty, got %d entries", len(spec.DirectMounts))
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
		BuildCtx:      "/tmp/.stormdrain",
		DotfileDir:    "/tmp/.stormdrain/dots",
		BuildArgs: map[string]string{
			"UID": "1000",
			"GID": "1000",
		},
		DirectMounts: []MountSpec{
			{HostPath: "/home/user/project/cmd", ContainerPath: "/home/dev/myproject/cmd"},
		},
		VirtualVolumes: []VirtualVolume{
			{Name: "go-mod-cache", Path: "/go/pkg/mod"},
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
	if len(loaded.DirectMounts) != len(original.DirectMounts) {
		t.Errorf("DirectMounts length: got %d, want %d", len(loaded.DirectMounts), len(original.DirectMounts))
	}
	if loaded.DirectMounts[0].HostPath != original.DirectMounts[0].HostPath {
		t.Errorf("DirectMounts[0].HostPath: got %q, want %q", loaded.DirectMounts[0].HostPath, original.DirectMounts[0].HostPath)
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
		Workspace: Workspace{
			DirectMounts:   []string{"src", "Makefile"},
			VirtualVolumes: []VirtualVolume{{Name: "cache", Path: "/var/cache"}},
		},
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
	if profile.Workspace.DirectMounts[0] != "src" {
		t.Errorf("DirectMounts[0]: got %q, want %q", profile.Workspace.DirectMounts[0], "src")
	}
	if profile.Workspace.VirtualVolumes[0].Name != "cache" {
		t.Errorf("VirtualVolumes[0].Name: got %q, want %q", profile.Workspace.VirtualVolumes[0].Name, "cache")
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
