package metrics

import (
	"testing"
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
