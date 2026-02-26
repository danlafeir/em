package github

import (
	"testing"
	"time"
)

func TestCredentials_BaseURL(t *testing.T) {
	tests := []struct {
		name     string
		creds    Credentials
		expected string
	}{
		{
			name:     "default URL",
			creds:    Credentials{Token: "tok"},
			expected: "https://api.github.com",
		},
		{
			name:     "override URL",
			creds:    Credentials{Token: "tok", BaseURLOverride: "https://github.example.com/api/v3"},
			expected: "https://github.example.com/api/v3",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.creds.BaseURL()
			if got != tt.expected {
				t.Errorf("BaseURL() = %q, want %q", got, tt.expected)
			}
		})
	}
}

func TestCalculateBackoff(t *testing.T) {
	c := NewClient(Credentials{Token: "test"})

	t.Run("exponential increase", func(t *testing.T) {
		d0 := c.calculateBackoff(0, "")
		d1 := c.calculateBackoff(1, "")
		d2 := c.calculateBackoff(2, "")

		// With jitter (0.7-1.3), attempt 0 base is 2s, attempt 1 is 4s, attempt 2 is 8s
		// So d0 in [1.4s, 2.6s], d1 in [2.8s, 5.2s], d2 in [5.6s, 10.4s]
		if d0 < 1*time.Second || d0 > 3*time.Second {
			t.Errorf("attempt 0 backoff %v out of expected range [1s, 3s]", d0)
		}
		if d1 < 2*time.Second || d1 > 6*time.Second {
			t.Errorf("attempt 1 backoff %v out of expected range [2s, 6s]", d1)
		}
		if d2 < 5*time.Second || d2 > 11*time.Second {
			t.Errorf("attempt 2 backoff %v out of expected range [5s, 11s]", d2)
		}
	})

	t.Run("retry-after header", func(t *testing.T) {
		d := c.calculateBackoff(0, "10")
		if d != 10*time.Second {
			t.Errorf("with Retry-After=10, got %v, want 10s", d)
		}
	})

	t.Run("max cap", func(t *testing.T) {
		d := c.calculateBackoff(10, "")
		if d > c.rateLimiter.MaxDelay {
			t.Errorf("backoff %v exceeds max %v", d, c.rateLimiter.MaxDelay)
		}
	})
}

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
