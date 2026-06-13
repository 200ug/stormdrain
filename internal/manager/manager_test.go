package manager

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestNewSpecWithContainerName_BuildCtxScopedByContainerName(t *testing.T) {
	projectPath := "/tmp/testproject"
	containerName := "myproject-atomics"
	hostname := "atomics"

	profile := &Profile{
		Name:         "golang",
		Shell:        "/bin/bash",
		ProjectMount: boolPtr(true),
	}

	spec, err := NewSpecWithContainerName(profile, projectPath, containerName, hostname)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedBuildCtx := filepath.Join(projectPath, ".stormdrain", containerName)
	if spec.BuildCtx != expectedBuildCtx {
		t.Errorf("BuildCtx: got %q, want %q", spec.BuildCtx, expectedBuildCtx)
	}

	expectedConfigsDir := filepath.Join(projectPath, ".stormdrain", containerName, "configs")
	if spec.ConfigsDir != expectedConfigsDir {
		t.Errorf("ConfigsDir: got %q, want %q", spec.ConfigsDir, expectedConfigsDir)
	}
}

func TestNewSpecWithContainerName_DifferentContainersDifferentPaths(t *testing.T) {
	projectPath := "/tmp/testproject"
	container1 := "myproject-atomics"
	container2 := "myproject-sietch"

	profile := &Profile{
		Name:         "golang",
		Shell:        "/bin/bash",
		ProjectMount: boolPtr(true),
	}

	spec1, err := NewSpecWithContainerName(profile, projectPath, container1, "atomics")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	spec2, err := NewSpecWithContainerName(profile, projectPath, container2, "sietch")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if spec1.BuildCtx == spec2.BuildCtx {
		t.Errorf("two different containers share the same BuildCtx: %q", spec1.BuildCtx)
	}
	if spec1.ConfigsDir == spec2.ConfigsDir {
		t.Errorf("two different containers share the same ConfigsDir: %q", spec1.ConfigsDir)
	}

	if spec1.ContainerName != container1 {
		t.Errorf("ContainerName: got %q, want %q", spec1.ContainerName, container1)
	}
	if spec2.ContainerName != container2 {
		t.Errorf("ContainerName: got %q, want %q", spec2.ContainerName, container2)
	}
}

func TestNewSpecWithContainerName_DefaultShell(t *testing.T) {
	projectPath := "/tmp/testproject"
	profile := &Profile{
		Name:         "rust",
		Shell:        "",
		ProjectMount: boolPtr(true),
	}

	spec, err := NewSpecWithContainerName(profile, projectPath, "proj-laza", "laza")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if spec.Shell != DefaultShell {
		t.Errorf("Shell: got %q, want %q", spec.Shell, DefaultShell)
	}
}

func TestNewSpecWithContainerName_ImageTag(t *testing.T) {
	projectPath := "/tmp/testproject"
	profile := &Profile{
		Name:         "python",
		Shell:        "/bin/zsh",
		ProjectMount: boolPtr(true),
	}

	spec, err := NewSpecWithContainerName(profile, projectPath, "testproj-mentat", "mentat")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	expectedTag := "stormdrain-python-testproject"
	if spec.ImageTag != expectedTag {
		t.Errorf("ImageTag: got %q, want %q", spec.ImageTag, expectedTag)
	}
}

func TestWriteToDisk_WritesToContainerScopedPath(t *testing.T) {
	projectDir := t.TempDir()
	containerName := "myproject-atomics"

	spec := &Spec{
		ContainerName: containerName,
		Hostname:      "atomics",
		ImageTag:      "stormdrain-golang-myproject",
		Shell:         "/bin/zsh",
		ProjectPath:   projectDir,
		ProjectMount:  true,
		BuildCtx:      filepath.Join(projectDir, ".stormdrain", containerName),
		ConfigsDir:    filepath.Join(projectDir, ".stormdrain", containerName, "configs"),
		BuildArgs:     map[string]string{"UID": "1000", "GID": "1000"},
	}

	if err := os.MkdirAll(spec.BuildCtx, 0755); err != nil {
		t.Fatalf("failed to create BuildCtx dir: %v", err)
	}

	if err := spec.WriteToDisk(); err != nil {
		t.Fatalf("WriteToDisk failed: %v", err)
	}

	expectedPath := filepath.Join(projectDir, ".stormdrain", containerName, "pod_spec.json")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("pod_spec.json not found at container-scoped path %q", expectedPath)
	}

	oldPath := filepath.Join(projectDir, ".stormdrain", "pod_spec.json")
	if _, err := os.Stat(oldPath); err == nil {
		t.Error("pod_spec.json should NOT exist at old unscoped path .stormdrain/pod_spec.json")
	}
}

func TestLoadSpec_ReadsFromContainerScopedPath(t *testing.T) {
	projectDir := t.TempDir()
	containerName := "myproject-sietch"

	spec := &Spec{
		ContainerName: containerName,
		Hostname:      "sietch",
		ImageTag:      "stormdrain-golang-myproject",
		Shell:         "/bin/zsh",
		ProjectPath:   projectDir,
		ProjectMount:  true,
		BuildCtx:      filepath.Join(projectDir, ".stormdrain", containerName),
		ConfigsDir:    filepath.Join(projectDir, ".stormdrain", containerName, "configs"),
		BuildArgs:     map[string]string{"UID": "1000", "GID": "1000"},
	}

	containerDir := filepath.Join(projectDir, ".stormdrain", containerName)
	if err := os.MkdirAll(containerDir, 0755); err != nil {
		t.Fatalf("failed to create container dir: %v", err)
	}
	if err := spec.WriteToDisk(); err != nil {
		t.Fatalf("WriteToDisk failed: %v", err)
	}

	loaded, err := LoadSpec(projectDir, containerName)
	if err != nil {
		t.Fatalf("LoadSpec failed: %v", err)
	}

	if loaded.ContainerName != containerName {
		t.Errorf("ContainerName: got %q, want %q", loaded.ContainerName, containerName)
	}
	if loaded.Hostname != "sietch" {
		t.Errorf("Hostname: got %q, want %q", loaded.Hostname, "sietch")
	}
	if loaded.ProjectPath != projectDir {
		t.Errorf("ProjectPath: got %q, want %q", loaded.ProjectPath, projectDir)
	}
}

func TestLoadSpec_NotFoundError(t *testing.T) {
	projectDir := t.TempDir()

	_, err := LoadSpec(projectDir, "nonexistent-container")
	if err == nil {
		t.Error("expected error for nonexistent spec, got nil")
	}
}

func TestWriteToDisk_LoadSpec_RoundTrip(t *testing.T) {
	projectDir := t.TempDir()
	containerName := "roundtrip-ghola"

	original := &Spec{
		ContainerName:  containerName,
		Hostname:       "ghola",
		ImageTag:       "stormdrain-rust-roundtrip",
		Shell:          "/bin/bash",
		ProjectPath:    projectDir,
		ProjectMount:   true,
		BuildCtx:       filepath.Join(projectDir, ".stormdrain", containerName),
		ConfigsDir:     filepath.Join(projectDir, ".stormdrain", containerName, "configs"),
		BuildArgs:      map[string]string{"UID": "1000", "GID": "1000"},
		VirtualVolumes: []VirtualVolume{{Name: "myvol", Path: "/data"}},
		Ports:          []PortMap{{Host: 8080, Container: 80}},
		EnvFiles:       []string{"/tmp/env"},
	}

	containerDir := filepath.Join(projectDir, ".stormdrain", containerName)
	if err := os.MkdirAll(containerDir, 0755); err != nil {
		t.Fatalf("failed to create container dir: %v", err)
	}
	if err := original.WriteToDisk(); err != nil {
		t.Fatalf("WriteToDisk failed: %v", err)
	}

	loaded, err := LoadSpec(projectDir, containerName)
	if err != nil {
		t.Fatalf("LoadSpec failed: %v", err)
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
	if loaded.ProjectMount != original.ProjectMount {
		t.Errorf("ProjectMount: got %v, want %v", loaded.ProjectMount, original.ProjectMount)
	}
	if len(loaded.VirtualVolumes) != 1 || loaded.VirtualVolumes[0].Name != "myvol" {
		t.Errorf("VirtualVolumes: got %v, want [{myvol /data}]", loaded.VirtualVolumes)
	}
	if len(loaded.Ports) != 1 || loaded.Ports[0].Host != 8080 {
		t.Errorf("Ports: got %v, want [{8080 80}]", loaded.Ports)
	}
}

func TestWriteToDisk_MultipleContainersNoConflict(t *testing.T) {
	projectDir := t.TempDir()

	container1 := "proj-atomics"
	container2 := "proj-sietch"

	spec1 := &Spec{
		ContainerName: container1,
		Hostname:      "atomics",
		ImageTag:      "stormdrain-golang-proj",
		Shell:         "/bin/zsh",
		ProjectPath:   projectDir,
		ProjectMount:  true,
		BuildCtx:      filepath.Join(projectDir, ".stormdrain", container1),
		ConfigsDir:    filepath.Join(projectDir, ".stormdrain", container1, "configs"),
		BuildArgs:     map[string]string{"UID": "1000", "GID": "1000"},
	}
	spec2 := &Spec{
		ContainerName: container2,
		Hostname:      "sietch",
		ImageTag:      "stormdrain-rust-proj",
		Shell:         "/bin/bash",
		ProjectPath:   projectDir,
		ProjectMount:  false,
		BuildCtx:      filepath.Join(projectDir, ".stormdrain", container2),
		ConfigsDir:    filepath.Join(projectDir, ".stormdrain", container2, "configs"),
		BuildArgs:     map[string]string{"UID": "1000", "GID": "1000"},
	}

	if err := os.MkdirAll(spec1.BuildCtx, 0755); err != nil {
		t.Fatalf("failed to create dir for container1: %v", err)
	}
	if err := os.MkdirAll(spec2.BuildCtx, 0755); err != nil {
		t.Fatalf("failed to create dir for container2: %v", err)
	}

	if err := spec1.WriteToDisk(); err != nil {
		t.Fatalf("WriteToDisk for container1 failed: %v", err)
	}
	if err := spec2.WriteToDisk(); err != nil {
		t.Fatalf("WriteToDisk for container2 failed: %v", err)
	}

	path1 := filepath.Join(projectDir, ".stormdrain", container1, "pod_spec.json")
	path2 := filepath.Join(projectDir, ".stormdrain", container2, "pod_spec.json")

	data1, err := os.ReadFile(path1)
	if err != nil {
		t.Fatalf("failed to read spec1: %v", err)
	}
	data2, err := os.ReadFile(path2)
	if err != nil {
		t.Fatalf("failed to read spec2: %v", err)
	}

	var loaded1, loaded2 Spec
	if err := json.Unmarshal(data1, &loaded1); err != nil {
		t.Fatalf("failed to unmarshal spec1: %v", err)
	}
	if err := json.Unmarshal(data2, &loaded2); err != nil {
		t.Fatalf("failed to unmarshal spec2: %v", err)
	}

	if loaded1.ContainerName == loaded2.ContainerName {
		t.Error("two containers sharing same project path should NOT overwrite each other's specs")
	}
	if loaded1.Hostname != "atomics" {
		t.Errorf("spec1 Hostname: got %q, want %q", loaded1.Hostname, "atomics")
	}
	if loaded2.Hostname != "sietch" {
		t.Errorf("spec2 Hostname: got %q, want %q", loaded2.Hostname, "sietch")
	}
	if loaded1.ImageTag != "stormdrain-golang-proj" {
		t.Errorf("spec1 ImageTag: got %q, want %q", loaded1.ImageTag, "stormdrain-golang-proj")
	}
	if loaded2.ImageTag != "stormdrain-rust-proj" {
		t.Errorf("spec2 ImageTag: got %q, want %q", loaded2.ImageTag, "stormdrain-rust-proj")
	}
}

func TestCleanupStagedConfigs_RemovesContainerScopedConfigsDir(t *testing.T) {
	projectDir := t.TempDir()
	containerName := "cleanup-test-ghola"

	configsDir := filepath.Join(projectDir, ".stormdrain", containerName, "configs")
	if err := os.MkdirAll(configsDir, 0755); err != nil {
		t.Fatalf("failed to create configs dir: %v", err)
	}

	dummyFile := filepath.Join(configsDir, "test.cfg")
	if err := os.WriteFile(dummyFile, []byte("test"), 0644); err != nil {
		t.Fatalf("failed to write dummy file: %v", err)
	}

	if err := CleanupStagedConfigs(projectDir, containerName); err != nil {
		t.Fatalf("CleanupStagedConfigs failed: %v", err)
	}

	if _, err := os.Stat(configsDir); err == nil {
		t.Errorf("configs dir should have been removed: %q", configsDir)
	}

	containerDir := filepath.Join(projectDir, ".stormdrain", containerName)
	if _, err := os.Stat(containerDir); os.IsNotExist(err) {
		t.Error("container-scoped .stormdrain/<containerName> dir should still exist after config cleanup")
	}
}

func TestCleanupStagedConfigs_DoesNotAffectOtherContainer(t *testing.T) {
	projectDir := t.TempDir()

	container1 := "proj-atomics"
	container2 := "proj-sietch"

	configsDir1 := filepath.Join(projectDir, ".stormdrain", container1, "configs")
	configsDir2 := filepath.Join(projectDir, ".stormdrain", container2, "configs")

	if err := os.MkdirAll(configsDir1, 0755); err != nil {
		t.Fatalf("failed to create configs dir for container1: %v", err)
	}
	if err := os.MkdirAll(configsDir2, 0755); err != nil {
		t.Fatalf("failed to create configs dir for container2: %v", err)
	}

	dummyFile1 := filepath.Join(configsDir1, "test1.cfg")
	dummyFile2 := filepath.Join(configsDir2, "test2.cfg")
	os.WriteFile(dummyFile1, []byte("test1"), 0644)
	os.WriteFile(dummyFile2, []byte("test2"), 0644)

	if err := CleanupStagedConfigs(projectDir, container1); err != nil {
		t.Fatalf("CleanupStagedConfigs for container1 failed: %v", err)
	}

	if _, err := os.Stat(configsDir1); err == nil {
		t.Error("container1 configs should have been removed")
	}
	if _, err := os.Stat(configsDir2); err != nil {
		t.Error("container2 configs should still exist after removing container1 configs")
	}
}

func TestSubstituteDockerfileTemplate_WritesToContainerScopedPath(t *testing.T) {
	projectDir := t.TempDir()
	configsDir := t.TempDir()
	containerName := "proj-mentat"

	dockerfilePath := filepath.Join(configsDir, "Dockerfile.base")
	dockerfileContent := []byte("# test dockerfile\n# {{PROFILE_PKGS}}\n# {{PROFILE_INSTALLERS}}\n# {{PROFILE_CONFIGS}}\n# {{PROFILE_DIRS}}\n")
	if err := os.WriteFile(dockerfilePath, dockerfileContent, 0644); err != nil {
		t.Fatalf("failed to write Dockerfile.base: %v", err)
	}

	profile := &Profile{
		Name:         "test",
		Shell:        "/bin/zsh",
		ProjectMount: boolPtr(true),
	}

	err := profile.SubstituteDockerfileTemplate(configsDir, projectDir, containerName)
	if err != nil {
		t.Fatalf("SubstituteDockerfileTemplate failed: %v", err)
	}

	expectedPath := filepath.Join(projectDir, ".stormdrain", containerName, "Dockerfile.sd")
	if _, err := os.Stat(expectedPath); os.IsNotExist(err) {
		t.Errorf("Dockerfile.sd not found at container-scoped path %q", expectedPath)
	}

	oldPath := filepath.Join(projectDir, ".stormdrain", "Dockerfile.sd")
	if _, err := os.Stat(oldPath); err == nil {
		t.Error("Dockerfile.sd should NOT exist at old unscoped path .stormdrain/Dockerfile.sd")
	}
}

func TestStageConfigs_StagestoContainerScopedPath(t *testing.T) {
	projectDir := t.TempDir()
	containerName := "proj-arrakeen"

	userHome := t.TempDir()
	configDir := filepath.Join(userHome, ".config", "nvim")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "init.lua"), []byte("vim.cmd('set nu')"), 0644); err != nil {
		t.Fatalf("failed to create init.lua: %v", err)
	}

	profile := &Profile{
		Name:  "test",
		Shell: "/bin/zsh",
		Configs: []Config{
			{SourcePattern: "~/.config/nvim", DestinationPath: "~/.config/nvim"},
		},
	}

	err := profile.StageConfigs(userHome, projectDir, containerName)
	if err != nil {
		t.Fatalf("StageConfigs failed: %v", err)
	}

	expectedConfigPath := filepath.Join(projectDir, ".stormdrain", containerName, "configs", ".config", "nvim", "init.lua")
	if _, err := os.Stat(expectedConfigPath); os.IsNotExist(err) {
		t.Errorf("staged config not found at container-scoped path %q", expectedConfigPath)
	}

	oldConfigPath := filepath.Join(projectDir, ".stormdrain", "configs", ".config", "nvim", "init.lua")
	if _, err := os.Stat(oldConfigPath); err == nil {
		t.Error("staged config should NOT exist at old unscoped path .stormdrain/configs/")
	}
}

func TestStageConfigs_MultipleContainersNoConflict(t *testing.T) {
	projectDir := t.TempDir()
	container1 := "proj-atomics"
	container2 := "proj-sietch"

	userHome := t.TempDir()
	configDir := filepath.Join(userHome, ".config", "nvim")
	if err := os.MkdirAll(configDir, 0755); err != nil {
		t.Fatalf("failed to create config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "init.lua"), []byte("vim.cmd('set nu')"), 0644); err != nil {
		t.Fatalf("failed to create init.lua: %v", err)
	}

	profile := &Profile{
		Name:  "test",
		Shell: "/bin/zsh",
		Configs: []Config{
			{SourcePattern: "~/.config/nvim", DestinationPath: "~/.config/nvim"},
		},
	}

	if err := profile.StageConfigs(userHome, projectDir, container1); err != nil {
		t.Fatalf("StageConfigs for container1 failed: %v", err)
	}
	if err := profile.StageConfigs(userHome, projectDir, container2); err != nil {
		t.Fatalf("StageConfigs for container2 failed: %v", err)
	}

	path1 := filepath.Join(projectDir, ".stormdrain", container1, "configs", ".config", "nvim", "init.lua")
	path2 := filepath.Join(projectDir, ".stormdrain", container2, "configs", ".config", "nvim", "init.lua")

	if _, err := os.Stat(path1); os.IsNotExist(err) {
		t.Error("container1 staged config not found")
	}
	if _, err := os.Stat(path2); os.IsNotExist(err) {
		t.Error("container2 staged config not found")
	}
}

func TestSubstituteDockerfileTemplate_MultipleContainersNoOverride(t *testing.T) {
	projectDir := t.TempDir()
	configsDir := t.TempDir()
	container1 := "proj-atomics"
	container2 := "proj-sietch"

	dockerfilePath := filepath.Join(configsDir, "Dockerfile.base")
	dockerfileContent := []byte("# test dockerfile\n# {{PROFILE_PKGS}}\n# {{PROFILE_INSTALLERS}}\n# {{PROFILE_CONFIGS}}\n# {{PROFILE_DIRS}}\n")
	if err := os.WriteFile(dockerfilePath, dockerfileContent, 0644); err != nil {
		t.Fatalf("failed to write Dockerfile.base: %v", err)
	}

	profile1 := &Profile{
		Name:         "golang",
		Shell:        "/bin/zsh",
		ProjectMount: boolPtr(true),
		Packages:     []string{"golang-go"},
	}
	profile2 := &Profile{
		Name:         "rust",
		Shell:        "/bin/bash",
		ProjectMount: boolPtr(false),
		Packages:     []string{"rustc"},
	}

	if err := profile1.SubstituteDockerfileTemplate(configsDir, projectDir, container1); err != nil {
		t.Fatalf("SubstituteDockerfileTemplate for container1 failed: %v", err)
	}
	if err := profile2.SubstituteDockerfileTemplate(configsDir, projectDir, container2); err != nil {
		t.Fatalf("SubstituteDockerfileTemplate for container2 failed: %v", err)
	}

	path1 := filepath.Join(projectDir, ".stormdrain", container1, "Dockerfile.sd")
	path2 := filepath.Join(projectDir, ".stormdrain", container2, "Dockerfile.sd")

	data1, err := os.ReadFile(path1)
	if err != nil {
		t.Fatalf("failed to read Dockerfile.sd for container1: %v", err)
	}
	data2, err := os.ReadFile(path2)
	if err != nil {
		t.Fatalf("failed to read Dockerfile.sd for container2: %v", err)
	}

	if string(data1) == string(data2) {
		t.Error("two different profiles should produce different Dockerfiles, but they are the same")
	}
}

func TestLoadSpec_RequiresContainerName(t *testing.T) {
	projectDir := t.TempDir()

	_, err := LoadSpec(projectDir, "")
	if err == nil {
		t.Error("expected error when containerName is empty, got nil")
	}
}

func TestCleanupStagedConfigs_NoConfigsDirIsNoop(t *testing.T) {
	projectDir := t.TempDir()
	containerName := "noop-container"

	err := CleanupStagedConfigs(projectDir, containerName)
	if err != nil {
		t.Errorf("CleanupStagedConfigs on nonexistent dir should be a no-op, got: %v", err)
	}
}

func boolPtr(b bool) *bool {
	return &b
}
