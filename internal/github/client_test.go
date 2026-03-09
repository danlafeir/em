package github

import "testing"

func TestParseLinkHeader(t *testing.T) {
	tests := []struct {
		name     string
		header   string
		expected string
	}{
		{
			name:     "next URL present",
			header:   `<https://api.github.com/orgs/myorg/teams/myteam/repos?page=2>; rel="next", <https://api.github.com/orgs/myorg/teams/myteam/repos?page=5>; rel="last"`,
			expected: "https://api.github.com/orgs/myorg/teams/myteam/repos?page=2",
		},
		{
			name:     "no next rel",
			header:   `<https://api.github.com/orgs/myorg/teams/myteam/repos?page=1>; rel="prev", <https://api.github.com/orgs/myorg/teams/myteam/repos?page=5>; rel="last"`,
			expected: "",
		},
		{
			name:     "empty string",
			header:   "",
			expected: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := parseLinkHeader(tt.header)
			if got != tt.expected {
				t.Errorf("parseLinkHeader() = %q, want %q", got, tt.expected)
			}
		})
	}
}
