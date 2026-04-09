package metrics

import (
	"math"
	"sort"
)

// iqrFenceMultiplier controls how wide the Tukey fence is.
// 1.5 is the standard value; 2.0 is wider, retaining more borderline results.
const iqrFenceMultiplier = 2.0

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
// using Tukey's IQR fence method: outliers are values outside [Q1 - iqrFenceMultiplier×IQR, Q3 + iqrFenceMultiplier×IQR].
// IQR is robust against the masking effect that afflicts stddev-based methods when
// multiple extreme values inflate σ and hide each other from the filter.
// If len < 4 or IQR is 0, returns everything in kept.
func FilterCycleTimeOutliers(results []CycleTimeResult) (kept, outliers []CycleTimeResult) {
	if len(results) < 4 {
		return results, nil
	}

	sorted := make([]float64, len(results))
	for i, r := range results {
		sorted[i] = r.CycleTimeDays()
	}
	sort.Float64s(sorted)

	q1 := percentileFloat(sorted, 25)
	q3 := percentileFloat(sorted, 75)
	iqr := q3 - q1
	if iqr == 0 {
		return results, nil
	}

	lo := q1 - iqrFenceMultiplier*iqr
	hi := q3 + iqrFenceMultiplier*iqr

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


func percentileFloat(sorted []float64, p int) float64 {
	if len(sorted) == 0 {
		return 0
	}
	idx := (p * len(sorted)) / 100
	if idx >= len(sorted) {
		idx = len(sorted) - 1
	}
	return sorted[idx]
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
