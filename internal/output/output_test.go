package output

import (
	"os"
	"path/filepath"
	"testing"
)

// -- Dir ----------------------------------------------------------------------

func TestDir_DefaultsToOut(t *testing.T) {
	os.Unsetenv("DEVCTL_EM_OUTPUT_DIR")
	if got := Dir(); got != "out" {
		t.Errorf("expected 'out', got %q", got)
	}
}

func TestDir_ReadsEnvVar(t *testing.T) {
	t.Setenv("DEVCTL_EM_OUTPUT_DIR", "/tmp/custom-output")
	if got := Dir(); got != "/tmp/custom-output" {
		t.Errorf("expected '/tmp/custom-output', got %q", got)
	}
}

func TestDir_EmptyEnvVarFallsBackToDefault(t *testing.T) {
	t.Setenv("DEVCTL_EM_OUTPUT_DIR", "")
	if got := Dir(); got != "out" {
		t.Errorf("expected 'out' when env var is empty, got %q", got)
	}
}

// -- Path ---------------------------------------------------------------------

func TestPath_JoinsDirAndName(t *testing.T) {
	os.Unsetenv("DEVCTL_EM_OUTPUT_DIR")
	got := Path("report.csv")
	expected := filepath.Join("out", "report.csv")
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestPath_UsesCustomDir(t *testing.T) {
	t.Setenv("DEVCTL_EM_OUTPUT_DIR", "/data")
	got := Path("metrics.xlsx")
	expected := filepath.Join("/data", "metrics.xlsx")
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

func TestPath_NestedName(t *testing.T) {
	os.Unsetenv("DEVCTL_EM_OUTPUT_DIR")
	got := Path("subdir/file.txt")
	expected := filepath.Join("out", "subdir", "file.txt")
	if got != expected {
		t.Errorf("expected %q, got %q", expected, got)
	}
}

// -- Create -------------------------------------------------------------------

func TestCreate_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "test.txt")

	f, err := Create(path)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	f.Close()

	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected file to exist: %v", err)
	}
}

func TestCreate_CreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "c", "file.txt")

	f, err := Create(path)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	f.Close()

	if _, err := os.Stat(path); err != nil {
		t.Errorf("expected file to exist after creating parent dirs: %v", err)
	}
}

func TestCreate_ReturnsWritableFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "writable.txt")

	f, err := Create(path)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	defer f.Close()

	content := []byte("hello")
	if _, err := f.Write(content); err != nil {
		t.Errorf("expected file to be writable: %v", err)
	}
}

func TestCreate_TruncatesExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "existing.txt")

	// Write initial content
	if err := os.WriteFile(path, []byte("original content"), 0o644); err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	f, err := Create(path)
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	f.Write([]byte("new"))
	f.Close()

	data, _ := os.ReadFile(path)
	if string(data) != "new" {
		t.Errorf("expected file to be truncated, got %q", string(data))
	}
}
