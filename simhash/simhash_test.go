package simhash_test

import (
	"iter"
	"math"
	"testing"

	"github.com/ophymx/semblance/internal/hashutil"
	"github.com/ophymx/semblance/simhash"
)

func weighted(pairs map[uint64]int) iter.Seq2[uint64, int] {
	// Map order does not matter: Sketch is a commutative sum per bit.
	return func(yield func(uint64, int) bool) {
		for h, w := range pairs {
			if !yield(h, w) {
				return
			}
		}
	}
}

func unit(hashes []uint64) iter.Seq2[uint64, int] {
	return func(yield func(uint64, int) bool) {
		for _, h := range hashes {
			if !yield(h, 1) {
				return
			}
		}
	}
}

func randHashes(rng *hashutil.SplitMix64, n int) []uint64 {
	out := make([]uint64, n)
	for i := range out {
		out[i] = rng.Next()
	}
	return out
}

func TestDistance(t *testing.T) {
	tests := []struct {
		name string
		a, b simhash.Fingerprint
		want int
	}{
		{"identical", 0xDEADBEEF, 0xDEADBEEF, 0},
		{"zero", 0, 0, 0},
		{"complement", 0, math.MaxUint64, 64},
		{"one bit", 0b1000, 0b0000, 1},
		{"nibble", 0b1111, 0b0000, 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := simhash.Distance(tt.a, tt.b); got != tt.want {
				t.Errorf("Distance(%#x, %#x) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}

func TestSketchEmpty(t *testing.T) {
	if got := simhash.Sketch(unit(nil)); got != 0 {
		t.Errorf("Sketch of no features = %#x, want 0", got)
	}
	if got := simhash.SketchText("", 3); got != 0 {
		t.Errorf("SketchText of empty text = %#x, want 0", got)
	}
	if got := simhash.SketchText("too short", 3); got != 0 {
		t.Errorf("SketchText below w tokens = %#x, want 0", got)
	}
}

func TestSketchSingleFeature(t *testing.T) {
	// With one positively-weighted feature every counter takes the sign of
	// the corresponding hash bit, so the fingerprint is the hash itself.
	const h simhash.Fingerprint = 0xDEADBEEFCAFEF00D
	if got := simhash.Sketch(weighted(map[uint64]int{uint64(h): 3})); got != h {
		t.Errorf("Sketch of single feature = %#x, want %#x", got, h)
	}
}

func TestWeightEqualsRepetition(t *testing.T) {
	rng := hashutil.SplitMix64(3)
	hs := randHashes(&rng, 10)
	byWeight := map[uint64]int{}
	var repeated []uint64
	for i, h := range hs {
		byWeight[h] = i + 1
		for range i + 1 {
			repeated = append(repeated, h)
		}
	}
	a := simhash.Sketch(weighted(byWeight))
	b := simhash.Sketch(unit(repeated))
	if a != b {
		t.Errorf("weight-n sketch %#x != n-repetition sketch %#x", a, b)
	}
}

func TestZeroAndNegativeWeights(t *testing.T) {
	rng := hashutil.SplitMix64(4)
	hs := randHashes(&rng, 8)
	base := simhash.Sketch(unit(hs))

	withZero := map[uint64]int{rng.Next(): 0}
	for _, h := range hs {
		withZero[h] = 1
	}
	if got := simhash.Sketch(weighted(withZero)); got != base {
		t.Errorf("zero-weight feature changed fingerprint: %#x != %#x", got, base)
	}

	// A feature added then subtracted cancels exactly.
	extra := rng.Next()
	cancelled := func(yield func(uint64, int) bool) {
		for _, h := range hs {
			if !yield(h, 1) {
				return
			}
		}
		_ = yield(extra, 2) && yield(extra, -2)
	}
	if got := simhash.Sketch(cancelled); got != base {
		t.Errorf("cancelled feature changed fingerprint: %#x != %#x", got, base)
	}
}

func TestOrderIndependent(t *testing.T) {
	rng := hashutil.SplitMix64(5)
	hs := randHashes(&rng, 50)
	a := simhash.Sketch(unit(hs))
	rev := make([]uint64, len(hs))
	for i, h := range hs {
		rev[len(hs)-1-i] = h
	}
	if b := simhash.Sketch(unit(rev)); a != b {
		t.Errorf("feature order changed fingerprint: %#x != %#x", a, b)
	}
}

func TestSketchTextPanics(t *testing.T) {
	for _, w := range []int{0, -1} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("SketchText(_, %d) did not panic", w)
				}
			}()
			simhash.SketchText("some text", w)
		}()
	}
}

// TestAccuracy checks the estimator against unit-weight feature sets of
// known cosine similarity (shared s, unique u per side: cos = s/(s+u)).
// The observed Hamming distance must fall within 3 standard deviations
// (sqrt(64·p·(1-p)), p = θ/π) of its expectation 64·p.
func TestAccuracy(t *testing.T) {
	const unique = 400
	for _, cosTarget := range []float64{0.2, 0.5, 0.8, 0.95} {
		for _, seed := range []uint64{0, 1, 2} {
			shared := int(math.Round(unique * cosTarget / (1 - cosTarget)))
			rng := hashutil.SplitMix64(1000*uint64(cosTarget*100) + seed)
			all := randHashes(&rng, shared+2*unique)
			setA := all[:shared+unique]
			setB := append(append([]uint64{}, all[:shared]...), all[shared+unique:]...)
			trueCos := float64(shared) / float64(shared+unique)

			d := simhash.Distance(simhash.Sketch(unit(setA)), simhash.Sketch(unit(setB)))
			p := math.Acos(trueCos) / math.Pi
			mean, sd := 64*p, math.Sqrt(64*p*(1-p))
			if math.Abs(float64(d)-mean) > 3*sd {
				t.Errorf("cos=%.2f seed=%d: distance %d outside %.1f ± %.1f",
					cosTarget, seed, d, mean, 3*sd)
			}
		}
	}
}

func FuzzSketchText(f *testing.F) {
	f.Add("the quick brown fox jumps over the lazy dog", 3)
	f.Add("", 1)
	f.Add("Ĝis la revido — ĝis!", 2)
	f.Add("\xff\xfe a\xffb", 1)
	f.Fuzz(func(t *testing.T, text string, w int) {
		if w <= 0 || w > 64 {
			t.Skip()
		}
		a := simhash.SketchText(text, w)
		b := simhash.SketchText(text, w)
		if a != b {
			t.Fatal("SketchText is nondeterministic")
		}
		if simhash.Distance(a, b) != 0 {
			t.Fatal("Distance of identical fingerprints != 0")
		}
	})
}

func TestSketchTextBytes(t *testing.T) {
	for _, text := range []string{"", "one two three four five", "HÉLLO wörld naïve", "\xffbad utf8\xfe words here"} {
		if got, want := simhash.SketchTextBytes([]byte(text), 2), simhash.SketchText(text, 2); got != want {
			t.Errorf("SketchTextBytes(%q) = %#x, want %#x", text, got, want)
		}
	}
}
