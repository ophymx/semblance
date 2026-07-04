package minhash_test

import (
	"math"
	"testing"

	"github.com/ophymx/semblance/internal/hashutil"
	"github.com/ophymx/semblance/minhash"
)

// TestCardinality checks the estimator against known set sizes: relative
// error within ~3 standard errors (n/sqrt(k)) plus slack for estimator
// bias at small n.
func TestCardinality(t *testing.T) {
	const k = 256
	m := minhash.New(k, 0)
	for _, n := range []int{1, 10, 100, 1000, 10000} {
		for _, seed := range []uint64{0, 1, 2} {
			rng := hashutil.SplitMix64(uint64(n)*10 + seed)
			est := minhash.Cardinality(m.Sketch(seq(randSet(&rng, n))))
			relErr := math.Abs(est-float64(n)) / float64(n)
			tol := 3.0/math.Sqrt(k) + 0.5/float64(n) // 3σ + small-n bias slack
			if relErr > tol {
				t.Errorf("n=%d seed=%d: estimate %.1f, relative error %.3f > %.3f",
					n, seed, est, relErr, tol)
			}
		}
	}

	// Empty-set signature estimates 0.
	if got := minhash.Cardinality(m.Sketch(seq(nil))); got != 0 {
		t.Errorf("Cardinality(empty) = %v, want 0", got)
	}
}

// TestContainmentAccuracy builds A with a known contained fraction inside
// a larger B and checks the estimate against the documented error model:
// absolute error ~ sqrt(R·c/k), R = (|A|+|B|)/|A|. Estimates must land
// within 3 of those standard errors (plus a floor for the c=0 and
// cardinality terms), and must order correctly across targets.
func TestContainmentAccuracy(t *testing.T) {
	const k = 256
	m := minhash.New(k, 0)
	for _, sizeB := range []int{1000, 4000} { // R = 3 and R = 9
		sizeA := 500
		R := float64(sizeA+sizeB) / float64(sizeA)
		var prev float64 = -1
		for _, target := range []float64{0.0, 0.25, 0.5, 0.75, 1.0} {
			for _, seed := range []uint64{0, 1, 2} {
				shared := int(math.Round(target * float64(sizeA)))
				rng := hashutil.SplitMix64(uint64(target*1000)*7 + uint64(sizeB) + seed)
				all := randSet(&rng, sizeA+sizeB-shared)
				setA := all[:sizeA]                         // shared elements are all[:shared]
				setB := append([]uint64{}, all[:shared]...) // plus the rest
				setB = append(setB, all[sizeA:]...)

				got := minhash.Containment(m.Sketch(seq(setA)), m.Sketch(seq(setB)))
				tol := 3*math.Sqrt(R*max(target, 0.05)/k) + 0.05
				if math.Abs(got-target) > tol {
					t.Errorf("R=%.0f c=%.2f seed=%d: estimate %.3f outside ±%.3f",
						R, target, seed, got, tol)
				}
				if seed == 0 {
					if got <= prev {
						t.Errorf("R=%.0f: estimates not increasing at c=%.2f (%.3f <= %.3f)",
							R, target, got, prev)
					}
					prev = got
				}
			}
		}
		// The motivation check: at high containment with asymmetric sizes,
		// Jaccard stays small while Containment reads high.
	}
}

func TestContainmentEdges(t *testing.T) {
	m := minhash.New(64, 0)
	rng := hashutil.SplitMix64(3)
	set := randSet(&rng, 300)
	sig := m.Sketch(seq(set))
	empty := m.Sketch(seq(nil))

	if got := minhash.Containment(empty, sig); got != 0 {
		t.Errorf("Containment(empty, x) = %v, want 0", got)
	}
	if got := minhash.Containment(sig, empty); got > 0.05 {
		t.Errorf("Containment(x, empty) = %v, want ~0", got)
	}
	if got := minhash.Containment(sig, sig); got < 0.95 || got > 1 {
		t.Errorf("Containment(x, x) = %v, want ~1 (clamped)", got)
	}
	// Subset: A strictly inside B must estimate near 1 and stay clamped.
	sub := m.Sketch(seq(set[:100]))
	if got := minhash.Containment(sub, sig); got < 0.85 || got > 1 {
		t.Errorf("Containment(subset, superset) = %v, want ~1", got)
	}
}

func TestCardinalityPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("Cardinality(zero-length) did not panic")
		}
	}()
	minhash.Cardinality(minhash.Signature{})
}
