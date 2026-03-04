package metrics

import "math"

// FilterOutliers returns values within mean ± stddevs*σ.
// If all values would be filtered or len < 2, returns the original slice unchanged.
func FilterOutliers(values []int, stddevs float64) []int {
	if len(values) < 2 {
		return values
	}

	mean, stddev := meanStddev(values)
	if stddev == 0 {
		return values
	}

	lo := mean - stddevs*stddev
	hi := mean + stddevs*stddev

	var filtered []int
	for _, v := range values {
		if float64(v) >= lo && float64(v) <= hi {
			filtered = append(filtered, v)
		}
	}

	if len(filtered) == 0 {
		return values
	}
	return filtered
}

func meanStddev(values []int) (float64, float64) {
	n := float64(len(values))
	var sum float64
	for _, v := range values {
		sum += float64(v)
	}
	mean := sum / n

	var variance float64
	for _, v := range values {
		d := float64(v) - mean
		variance += d * d
	}
	variance /= n

	return mean, math.Sqrt(variance)
}
