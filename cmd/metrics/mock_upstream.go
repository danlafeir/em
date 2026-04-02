package metrics

import (
	"fmt"
	"net/http/httptest"

	"github.com/danlafeir/cli-go/pkg/config"

	"em/internal/github"
	"em/internal/jira"
	"em/internal/snyk"
	"em/internal/testutil/mockgithub"
	"em/internal/testutil/mockjira"
	"em/internal/testutil/mocksnyk"
)

var mockUpstreamFlag bool

// activeMock holds running servers and pre-built clients while --mock-upstream is active.
var activeMock struct {
	jiraServer   *httptest.Server
	githubServer *httptest.Server
	snykServer   *httptest.Server

	jiraClient   *jira.Client
	githubClient *github.Client
	snykClient   *snyk.Client
}

// startMockUpstream spins up in-process mock servers for JIRA, GitHub, and Snyk
// and injects the minimum config needed for commands to run against them.
func startMockUpstream() error {
	// JIRA (plain HTTP — BaseURLOverride accepts http://)
	jiraDS := mockjira.RealisticDataset()
	jiraSrv := mockjira.New(jiraDS)
	activeMock.jiraServer = jiraSrv.Start()
	activeMock.jiraClient = jira.NewClient(jira.Credentials{
		Domain:          "mock",
		Email:           "mock@test.com",
		APIToken:        "mock-token",
		BaseURLOverride: activeMock.jiraServer.URL,
	})

	// GitHub (TLS — go-gh always uses HTTPS)
	ghDS := mockgithub.RealisticDataset()
	ghSrv := mockgithub.New(ghDS)
	activeMock.githubServer = ghSrv.Start()
	ghClient, err := mockgithub.NewClient(activeMock.githubServer)
	if err != nil {
		return fmt.Errorf("starting mock GitHub server: %w", err)
	}
	activeMock.githubClient = ghClient

	// Snyk (TLS — Snyk client uses HTTPS)
	snykDS := mocksnyk.RealisticDataset()
	snykSrv := mocksnyk.New(snykDS)
	activeMock.snykServer = snykSrv.Start()
	activeMock.snykClient = mocksnyk.NewClient(activeMock.snykServer)

	// Inject in-memory config so commands resolve teams/projects/workflows
	// without real configuration. Nothing is written to disk.
	injectMockConfig()

	fmt.Println("[mock-upstream] Servers running:")
	fmt.Printf("  JIRA   %s  (%d issues)\n", activeMock.jiraServer.URL, len(jiraDS.Issues))
	fmt.Printf("  GitHub %s  (%d repos)\n", activeMock.githubServer.URL, len(ghDS.Repos))
	fmt.Printf("  Snyk   %s  (%d issues)\n\n", activeMock.snykServer.URL, len(snykDS.Issues))

	return nil
}

// stopMockUpstream closes all running mock servers.
func stopMockUpstream() {
	if activeMock.jiraServer != nil {
		activeMock.jiraServer.Close()
	}
	if activeMock.githubServer != nil {
		activeMock.githubServer.Close()
	}
	if activeMock.snykServer != nil {
		activeMock.snykServer.Close()
	}
}

// injectMockConfig sets in-memory config values that let commands resolve
// teams, JQL, and workflows against the mock datasets.
// Values match what the realistic mock datasets produce:
//   - JIRA: project key "PROJ"
//   - GitHub: org "acme-org", repo "api-service", workflow "deploy.yml"
//   - Snyk: org ID "prod-org-id"
func injectMockConfig() {
	initConfig()
	set := func(k string, v any) { config.SetConfigValue(configNamespace, k, v) }

	// Always override selected team and team list so commands use mock data,
	// not any real team configuration the user may have saved.
	set("selected_team", "platform")
	set("team_names", []any{"platform"})

	// JIRA — project key matches mockjira issue keys ("PROJ-*")
	set("jira.domain", "mock")
	set("jira.email", "mock@test.com")
	set("teams.platform.jira.project", "PROJ")

	// GitHub — matches mockgithub.RealisticDataset()
	set("github.org", "acme-org")
	set("teams.platform.github.slug", "platform")
	set("teams.platform.github.workflows.api-service", "deploy.yml")

	// Snyk — matches mocksnyk.RealisticDataset()
	set("snyk.org_id", "prod-org-id")
	set("snyk.org_name", "Production Org")
}
