package metrics

import (
	"testing"
	"time"
)

func TestFilterOutliers(t *testing.T) {
	tests := []struct {
		name     string
		values   []int
		stddevs  float64
		expected []int
	}{
		{
			name:     "no filtering needed",
			values:   []int{5, 6, 7, 5, 6},
			stddevs:  2.0,
			expected: []int{5, 6, 7, 5, 6},
		},
		{
			name:     "outliers removed",
			values:   []int{5, 6, 5, 6, 5, 6, 50},
			stddevs:  2.0,
			expected: []int{5, 6, 5, 6, 5, 6},
		},
		{
			name:     "single value returns unchanged",
			values:   []int{42},
			stddevs:  2.0,
			expected: []int{42},
		},
		{
			name:     "empty slice returns unchanged",
			values:   []int{},
			stddevs:  2.0,
			expected: []int{},
		},
		{
			name:     "nil slice returns nil",
			values:   nil,
			stddevs:  2.0,
			expected: nil,
		},
		{
			name:     "all same values returns unchanged",
			values:   []int{3, 3, 3, 3},
			stddevs:  2.0,
			expected: []int{3, 3, 3, 3},
		},
		{
			name:     "all would be filtered returns original",
			values:   []int{1, 100},
			stddevs:  0.1,
			expected: []int{1, 100},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FilterOutliers(tt.values, tt.stddevs)
			if len(result) != len(tt.expected) {
				t.Fatalf("got %v, want %v", result, tt.expected)
			}
			for i := range result {
				if result[i] != tt.expected[i] {
					t.Fatalf("got %v, want %v", result, tt.expected)
				}
			}
		})
	}
}

func makeCTResult(key string, days float64) CycleTimeResult {
	return CycleTimeResult{
		IssueKey:  key,
		CycleTime: time.Duration(days * 24 * float64(time.Hour)),
		StartDate: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		EndDate:   time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Add(time.Duration(days * 24 * float64(time.Hour))),
	}
}

func TestFilterCycleTimeOutliers(t *testing.T) {
	tests := []struct {
		name         string
		results      []CycleTimeResult
		wantKeptKeys []string
		wantOutKeys  []string
	}{
		{
			name:         "empty slice",
			results:      nil,
			wantKeptKeys: nil,
			wantOutKeys:  nil,
		},
		{
			name:         "fewer than 4 results returns all in kept",
			results:      []CycleTimeResult{makeCTResult("A-1", 5), makeCTResult("A-2", 50)},
			wantKeptKeys: []string{"A-1", "A-2"},
			wantOutKeys:  nil,
		},
		{
			name: "no outliers",
			results: []CycleTimeResult{
				makeCTResult("A-1", 5),
				makeCTResult("A-2", 6),
				makeCTResult("A-3", 7),
				makeCTResult("A-4", 5),
				makeCTResult("A-5", 6),
			},
			wantKeptKeys: []string{"A-1", "A-2", "A-3", "A-4", "A-5"},
			wantOutKeys:  nil,
		},
		{
			name: "single outlier removed",
			results: []CycleTimeResult{
				makeCTResult("A-1", 5),
				makeCTResult("A-2", 6),
				makeCTResult("A-3", 5),
				makeCTResult("A-4", 6),
				makeCTResult("A-5", 5),
				makeCTResult("A-6", 6),
				makeCTResult("A-7", 50),
			},
			wantKeptKeys: []string{"A-1", "A-2", "A-3", "A-4", "A-5", "A-6"},
			wantOutKeys:  []string{"A-7"},
		},
		{
			// Masking test: two extreme outliers inflate σ enough to survive stddev-based
			// filtering, but IQR correctly rejects both because it is resistant to extreme
			// values in the upper tail (Q3 stays at 7, fence = 11.5, so 60 is excluded).
			name: "multiple outliers not masked by each other",
			results: []CycleTimeResult{
				makeCTResult("A-1", 3),
				makeCTResult("A-2", 3),
				makeCTResult("A-3", 4),
				makeCTResult("A-4", 4),
				makeCTResult("A-5", 5),
				makeCTResult("A-6", 5),
				makeCTResult("A-7", 6),
				makeCTResult("A-8", 7),
				makeCTResult("A-9", 60),
				makeCTResult("A-10", 60),
			},
			wantKeptKeys: []string{"A-1", "A-2", "A-3", "A-4", "A-5", "A-6", "A-7", "A-8"},
			wantOutKeys:  []string{"A-9", "A-10"},
		},
		{
			name: "all same values returns all in kept",
			results: []CycleTimeResult{
				makeCTResult("A-1", 3),
				makeCTResult("A-2", 3),
				makeCTResult("A-3", 3),
				makeCTResult("A-4", 3),
			},
			wantKeptKeys: []string{"A-1", "A-2", "A-3", "A-4"},
			wantOutKeys:  nil,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			kept, outliers := FilterCycleTimeOutliers(tt.results)

			keptKeys := make([]string, len(kept))
			for i, r := range kept {
				keptKeys[i] = r.IssueKey
			}
			outKeys := make([]string, len(outliers))
			for i, r := range outliers {
				outKeys[i] = r.IssueKey
			}

			if len(keptKeys) != len(tt.wantKeptKeys) {
				t.Fatalf("kept: got %v, want %v", keptKeys, tt.wantKeptKeys)
			}
			for i := range keptKeys {
				if keptKeys[i] != tt.wantKeptKeys[i] {
					t.Fatalf("kept: got %v, want %v", keptKeys, tt.wantKeptKeys)
				}
			}
			if len(outKeys) != len(tt.wantOutKeys) {
				t.Fatalf("outliers: got %v, want %v", outKeys, tt.wantOutKeys)
			}
			for i := range outKeys {
				if outKeys[i] != tt.wantOutKeys[i] {
					t.Fatalf("outliers: got %v, want %v", outKeys, tt.wantOutKeys)
				}
			}
		})
	}
}
