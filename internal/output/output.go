// Package output provides centralized output directory management.
package output

import (
	"os"
	"path/filepath"
)

// Dir returns the output directory path.
// It checks the DEVCTL_EM_OUTPUT_DIR env var, defaulting to "out".
func Dir() string {
	if d := os.Getenv("DEVCTL_EM_OUTPUT_DIR"); d != "" {
		return d
	}
	return "out"
}

// Path returns a file path within the output directory.
func Path(name string) string {
	return filepath.Join(Dir(), name)
}

// Create creates a file at the given path, ensuring parent directories exist.
func Create(path string) (*os.File, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, err
	}
	return os.Create(path)
}
