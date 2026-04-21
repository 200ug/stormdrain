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
	if err := CopyDir(srcDir, dstPath); err != nil {
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
	err := CopyDir("/nonexistent/path", filepath.Join(dstDir, "copy"))
	if err == nil {
		t.Error("expected error for nonexistent source dir, got nil")
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
