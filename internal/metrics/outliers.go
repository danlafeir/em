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

// FilterCycleTimeOutliers splits cycle time results into kept and outlier slices
// based on CycleTimeDays() using mean ± stddevs*σ filtering.
// If len < 2, stddev is 0, or all would be filtered, returns everything in kept.
func FilterCycleTimeOutliers(results []CycleTimeResult, stddevs float64) (kept, outliers []CycleTimeResult) {
	if len(results) < 2 {
		return results, nil
	}

	days := make([]float64, len(results))
	for i, r := range results {
		days[i] = r.CycleTimeDays()
	}

	mean, stddev := meanStddevFloat(days)
	if stddev == 0 {
		return results, nil
	}

	lo := mean - stddevs*stddev
	hi := mean + stddevs*stddev

	for _, r := range results {
		if d := r.CycleTimeDays(); d >= lo && d <= hi {
			kept = append(kept, r)
		} else {
			outliers = append(outliers, r)
		}
	}

	if len(kept) == 0 {
		return results, nil
	}
	return kept, outliers
}

func meanStddevFloat(values []float64) (float64, float64) {
	n := float64(len(values))
	var sum float64
	for _, v := range values {
		sum += v
	}
	mean := sum / n

	var variance float64
	for _, v := range values {
		d := v - mean
		variance += d * d
	}
	variance /= n

	return mean, math.Sqrt(variance)
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
