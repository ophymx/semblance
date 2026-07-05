package lsh

import "math"

// Params chooses banding parameters (bands, rows) with bands*rows == k, for
// a MinHash signature of length k, whose LSH candidate threshold
// (1/bands)^(1/rows) is as close as possible to target.
//
// Use it to align an index's banding with the similarity you intend to
// query at. Querying a [VerifiedIndex] (or verifying [Index] candidates)
// below the index's [Index.Threshold] silently misses pairs the banding
// never surfaces as candidates, so build the index for your threshold:
//
//	bands, rows := lsh.Params(128, 0.85)
//	ix := lsh.NewVerifiedIndex(bands, rows)
//	hits := ix.Query(sig, 0.85) // banding and query threshold agree
//
// k's factors constrain the achievable thresholds, so the returned banding
// is the closest available, not exact — pick a k with many factors (a
// power of two) for fine control. Among equally close factor pairs the one
// with more bands (higher recall) wins. Panics if k <= 0 or target is not
// in the open interval (0, 1).
func Params(k int, target float64) (bands, rows int) {
	if k <= 0 {
		panic("lsh: k must be positive")
	}
	if target <= 0 || target >= 1 {
		panic("lsh: target must be in (0, 1)")
	}
	bands, rows = 1, k
	best := math.Inf(1)
	for b := 1; b <= k; b++ {
		if k%b != 0 {
			continue
		}
		r := k / b
		t := math.Pow(1/float64(b), 1/float64(r))
		// <= so that, scanning bands ascending, ties resolve to more bands.
		if d := math.Abs(t - target); d <= best {
			best, bands, rows = d, b, r
		}
	}
	return bands, rows
}
