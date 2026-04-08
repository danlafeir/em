package metrics

import (
	"io"
	"os"
	"testing"

	"github.com/spf13/cobra"
)

// setupMock starts all mock upstream servers and injects in-memory config.
// It registers cleanup to stop servers and reset global state when the test ends.
func setupMock(t *testing.T) {
	t.Helper()
	if err := startMockUpstream(); err != nil {
		t.Fatalf("startMockUpstream: %v", err)
	}
	skipBrowserOpen = true

	// Redirect output files to a temp directory so tests don't pollute ~/.em/output.
	tmp := t.TempDir()
	orig := os.Getenv("DEVCTL_OUTPUT_DIR")
	os.Setenv("DEVCTL_OUTPUT_DIR", tmp)

	t.Cleanup(func() {
		stopMockUpstream()
		skipBrowserOpen = false
		os.Setenv("DEVCTL_OUTPUT_DIR", orig)
		// Reset all flags that commands read from global vars.
		fromFlag, toFlag = "", ""
		outputFlag, formatFlag = "", ""
		jqlFlag, projectFlag, jiraTeamFlag = "", "", ""
		ddFromFlag, ddToFlag, ddTeamFlag, ddOutputFlag, ddFormatFlag = "", "", "", "", ""
		frequencyFlag = "weekly"
		useSavedDataFlag = false
		saveRawDataFlag = false
	})
}

// silenceStdout swallows stdout for the duration of the test so command output
// doesn't clutter test output. Returns a restore function (called by t.Cleanup).
func silenceStdout(t *testing.T) {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	t.Cleanup(func() {
		w.Close()
		io.Copy(io.Discard, r)
		r.Close()
		os.Stdout = orig
	})
}

// dummyCmd returns a minimal cobra.Command suitable for passing to RunE functions
// that don't use the command argument.
func dummyCmd() *cobra.Command {
	return &cobra.Command{}
}

func TestRunCycleTime_Mock(t *testing.T) {
	setupMock(t)
	silenceStdout(t)
	if err := runCycleTime(dummyCmd(), nil); err != nil {
		t.Errorf("runCycleTime: %v", err)
	}
}

func TestRunThroughput_Mock(t *testing.T) {
	setupMock(t)
	silenceStdout(t)
	if err := runThroughput(dummyCmd(), nil); err != nil {
		t.Errorf("runThroughput: %v", err)
	}
}

func TestRunDatadogMonitors_Mock(t *testing.T) {
	setupMock(t)
	silenceStdout(t)
	if err := runDatadogMonitors(dummyCmd(), nil); err != nil {
		t.Errorf("runDatadogMonitors: %v", err)
	}
}

func TestRunDatadogSLOs_Mock(t *testing.T) {
	setupMock(t)
	silenceStdout(t)
	if err := runDatadogSLOs(dummyCmd(), nil); err != nil {
		t.Errorf("runDatadogSLOs: %v", err)
	}
}

func TestRunDeploymentFrequency_Mock(t *testing.T) {
	setupMock(t)
	silenceStdout(t)
	if err := runDeploymentFrequency(dummyCmd(), nil); err != nil {
		t.Errorf("runDeploymentFrequency: %v", err)
	}
}

func TestRunSnykIssues_Mock(t *testing.T) {
	setupMock(t)
	silenceStdout(t)
	if err := runSnykIssues(dummyCmd(), nil); err != nil {
		t.Errorf("runSnykIssues: %v", err)
	}
}
