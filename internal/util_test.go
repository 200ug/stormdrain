package internal

import (
	"os"
	"path/filepath"
	"testing"
)

// copy file

func TestCopyFile(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	content := []byte("hello stormdrain")
	srcPath := filepath.Join(srcDir, "source.txt")
	dstPath := filepath.Join(dstDir, "dest.txt")

	os.WriteFile(srcPath, content, 0644)

	if err := CopyFile(srcPath, dstPath); err != nil {
		t.Fatal(err)
	}

	result, err := os.ReadFile(dstPath)
	if err != nil {
		t.Fatal(err)
	}

	if string(result) != string(content) {
		t.Errorf("content mismatch: got %q, want %q", string(result), string(content))
	}
}

func TestCopyFileNotFound(t *testing.T) {
	dstDir := t.TempDir()
	dstPath := filepath.Join(dstDir, "dest.txt")

	err := CopyFile("/nonexistent/path/file.txt", dstPath)
	if err == nil {
		t.Error("expected error for nonexistent source, got nil")
	}
}

// copy directory

func TestCopyDir(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	os.MkdirAll(filepath.Join(srcDir, "sub", "nested"), 0755)
	os.WriteFile(filepath.Join(srcDir, "root.txt"), []byte("root"), 0644)
	os.WriteFile(filepath.Join(srcDir, "sub", "child.txt"), []byte("child"), 0644)
	os.WriteFile(filepath.Join(srcDir, "sub", "nested", "deep.txt"), []byte("deep"), 0644)

	dstPath := filepath.Join(dstDir, "copy")
	if err := CopyDir(srcDir, dstPath, nil); err != nil {
		t.Fatal(err)
	}

	rootData, err := os.ReadFile(filepath.Join(dstPath, "root.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(rootData) != "root" {
		t.Errorf("root.txt: got %q, want %q", string(rootData), "root")
	}

	childData, err := os.ReadFile(filepath.Join(dstPath, "sub", "child.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(childData) != "child" {
		t.Errorf("child.txt: got %q, want %q", string(childData), "child")
	}

	deepData, err := os.ReadFile(filepath.Join(dstPath, "sub", "nested", "deep.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(deepData) != "deep" {
		t.Errorf("deep.txt: got %q, want %q", string(deepData), "deep")
	}
}

func TestCopyDirNotFound(t *testing.T) {
	dstDir := t.TempDir()
	err := CopyDir("/nonexistent/path", filepath.Join(dstDir, "copy"), nil)
	if err == nil {
		t.Error("expected error for nonexistent source dir, got nil")
	}
}

func TestCopyDirExcludeDir(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	os.MkdirAll(filepath.Join(srcDir, "plugin"), 0755)
	os.WriteFile(filepath.Join(srcDir, "init.lua"), []byte("init"), 0644)
	os.WriteFile(filepath.Join(srcDir, "plugin", "packer_compiled.lua"), []byte("compiled"), 0644)
	os.MkdirAll(filepath.Join(srcDir, "lua"), 0755)
	os.WriteFile(filepath.Join(srcDir, "lua", "plugins.lua"), []byte("plugins"), 0644)

	dstPath := filepath.Join(dstDir, "copy")
	if err := CopyDir(srcDir, dstPath, []string{"plugin"}); err != nil {
		t.Fatal(err)
	}

	initData, err := os.ReadFile(filepath.Join(dstPath, "init.lua"))
	if err != nil {
		t.Fatal(err)
	}
	if string(initData) != "init" {
		t.Errorf("init.lua: got %q, want %q", string(initData), "init")
	}

	pluginsData, err := os.ReadFile(filepath.Join(dstPath, "lua", "plugins.lua"))
	if err != nil {
		t.Fatal(err)
	}
	if string(pluginsData) != "plugins" {
		t.Errorf("plugins.lua: got %q, want %q", string(pluginsData), "plugins")
	}

	if _, err := os.Stat(filepath.Join(dstPath, "plugin")); !os.IsNotExist(err) {
		t.Error("plugin directory should be excluded")
	}
}

func TestCopyDirExcludeFile(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	os.WriteFile(filepath.Join(srcDir, "keep.txt"), []byte("keep"), 0644)
	os.WriteFile(filepath.Join(srcDir, "skip.txt"), []byte("skip"), 0644)

	dstPath := filepath.Join(dstDir, "copy")
	if err := CopyDir(srcDir, dstPath, []string{"skip.txt"}); err != nil {
		t.Fatal(err)
	}

	keepData, err := os.ReadFile(filepath.Join(dstPath, "keep.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(keepData) != "keep" {
		t.Errorf("keep.txt: got %q, want %q", string(keepData), "keep")
	}

	if _, err := os.Stat(filepath.Join(dstPath, "skip.txt")); !os.IsNotExist(err) {
		t.Error("skip.txt should be excluded")
	}
}

func TestCopyDirExcludeGlob(t *testing.T) {
	srcDir := t.TempDir()
	dstDir := t.TempDir()

	os.WriteFile(filepath.Join(srcDir, "keep.lua"), []byte("keep"), 0644)
	os.WriteFile(filepath.Join(srcDir, "skip.log"), []byte("skip"), 0644)
	os.WriteFile(filepath.Join(srcDir, "another.log"), []byte("another"), 0644)

	dstPath := filepath.Join(dstDir, "copy")
	if err := CopyDir(srcDir, dstPath, []string{"*.log"}); err != nil {
		t.Fatal(err)
	}

	keepData, err := os.ReadFile(filepath.Join(dstPath, "keep.lua"))
	if err != nil {
		t.Fatal(err)
	}
	if string(keepData) != "keep" {
		t.Errorf("keep.lua: got %q, want %q", string(keepData), "keep")
	}

	if _, err := os.Stat(filepath.Join(dstPath, "skip.log")); !os.IsNotExist(err) {
		t.Error("skip.log should be excluded")
	}
	if _, err := os.Stat(filepath.Join(dstPath, "another.log")); !os.IsNotExist(err) {
		t.Error("another.log should be excluded")
	}
}

// random hostname

func TestRandomHostname(t *testing.T) {
	host := RandomHostname()
	found := false
	for _, h := range Hostnames {
		if h == host {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("RandomHostname() returned %q, which is not in Hostnames list", host)
	}
}
