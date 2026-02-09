/*
Copyright © 2025 NAME HERE <EMAIL ADDRESS>
*/
package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"devctl-em/cmd"
)

// BuildGitHash is set at build time via -ldflags
var BuildGitHash = "dev"

// BuildLatestHash is set at build time via -ldflags to the latest available hash
var BuildLatestHash = "dev"

func checkUpgrade() {
	configDir, err := os.UserConfigDir()
	if err != nil {
		return // fail silently
	}
	checkFile := filepath.Join(configDir, "devctl-em", "upgrade-check")
	os.MkdirAll(filepath.Dir(checkFile), 0o755)

	today := time.Now().Format("2006-01-02")
	var lastDate, lastHash string
	if f, err := os.Open(checkFile); err == nil {
		fmt.Fscanf(f, "%s %s", &lastDate, &lastHash)
		f.Close()
	}
	if lastDate == today {
		return // already checked today
	}

	// Check remote for latest hash
	remoteHash := BuildLatestHash
	if remoteHash != "" && remoteHash != BuildGitHash {
		fmt.Fprintf(os.Stderr, "A new version of devctl-em is available (hash: %s). Run 'devctl-em update' to upgrade.\n", remoteHash)
	}

	// Write today's check
	f, err := os.Create(checkFile)
	if err == nil {
		fmt.Fprintf(f, "%s %s", today, BuildGitHash)
		f.Close()
	}
}

func main() {
	checkUpgrade()
	cmd.BuildGitHash = BuildGitHash
	cmd.BuildLatestHash = BuildLatestHash
	cmd.Execute()
}
