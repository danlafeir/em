package charts

import (
	"os/exec"
	"path/filepath"
	"runtime"
)

// OpenBrowser opens the given file path in the default browser.
func OpenBrowser(path string) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	var cmd string
	switch runtime.GOOS {
	case "darwin":
		cmd = "open"
	default:
		cmd = "xdg-open"
	}

	return exec.Command(cmd, abs).Start()
}
