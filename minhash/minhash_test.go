package minhash_test

import (
	"iter"
	"math"
	"slices"
	"testing"

	"github.com/ophymx/semblance/internal/hashutil"
	"github.com/ophymx/semblance/minhash"
)

func seq(xs []uint64) iter.Seq[uint64] { return slices.Values(xs) }

// randSet returns n distinct pseudo-random elements from a seeded stream.
func randSet(rng *hashutil.SplitMix64, n int) []uint64 {
	seen := make(map[uint64]bool, n)
	out := make([]uint64, 0, n)
	for len(out) < n {
		x := rng.Next()
		if !seen[x] {
			seen[x] = true
			out = append(out, x)
		}
	}
	return out
}

func TestNewPanics(t *testing.T) {
	for _, k := range []int{0, -1} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("New(%d, 0) did not panic", k)
				}
			}()
			minhash.New(k, 0)
		}()
	}
}

func TestAccessors(t *testing.T) {
	m := minhash.New(64, 99)
	if m.K() != 64 {
		t.Errorf("K() = %d, want 64", m.K())
	}
	if m.Seed() != 99 {
		t.Errorf("Seed() = %d, want 99", m.Seed())
	}
}

func TestEmptyInput(t *testing.T) {
	m := minhash.New(8, 0)
	sig := m.Sketch(seq(nil))
	for i, v := range sig {
		if v != math.MaxUint64 {
			t.Errorf("empty-set signature slot %d = %#x, want MaxUint64", i, v)
		}
	}
	if got := minhash.Jaccard(sig, m.Sketch(seq(nil))); got != 1 {
		t.Errorf("Jaccard(empty, empty) = %v, want 1", got)
	}
}

func TestSketchIntoPanics(t *testing.T) {
	m := minhash.New(8, 0)
	defer func() {
		if recover() == nil {
			t.Error("SketchInto with wrong dst length did not panic")
		}
	}()
	m.SketchInto(make(minhash.Signature, 7), seq(nil))
}

func TestJaccardPanics(t *testing.T) {
	tests := []struct {
		name string
		a, b minhash.Signature
	}{
		{"length mismatch", make(minhash.Signature, 4), make(minhash.Signature, 5)},
		{"empty", minhash.Signature{}, minhash.Signature{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("expected panic, got none")
				}
			}()
			minhash.Jaccard(tt.a, tt.b)
		})
	}
}

func TestIdenticalSets(t *testing.T) {
	rng := hashutil.SplitMix64(7)
	set := randSet(&rng, 1000)
	m := minhash.New(128, 0)
	a := m.Sketch(seq(set))
	shuffled := slices.Clone(set)
	slices.Reverse(shuffled)
	b := m.Sketch(seq(shuffled))
	if got := minhash.Jaccard(a, b); got != 1 {
		t.Errorf("Jaccard of same set (different order) = %v, want 1", got)
	}
}

func TestDisjointSets(t *testing.T) {
	rng := hashutil.SplitMix64(7)
	all := randSet(&rng, 2000)
	m := minhash.New(128, 0)
	a := m.Sketch(seq(all[:1000]))
	b := m.Sketch(seq(all[1000:]))
	if got := minhash.Jaccard(a, b); got > 0.05 {
		t.Errorf("Jaccard of disjoint sets = %v, want ~0", got)
	}
}

func TestUnionExact(t *testing.T) {
	rng := hashutil.SplitMix64(11)
	all := randSet(&rng, 1500)
	setA, setB := all[:1000], all[500:] // overlap of 500
	m := minhash.New(128, 3)
	a := m.Sketch(seq(setA))
	b := m.Sketch(seq(setB))
	union := make(minhash.Signature, 128)
	minhash.Union(union, a, b)
	direct := m.Sketch(seq(all))
	if !slices.Equal(union, direct) {
		t.Fatal("Union(sketch(A), sketch(B)) != sketch(A ∪ B)")
	}
}

func TestUnionPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("Union with mismatched lengths did not panic")
		}
	}()
	minhash.Union(make(minhash.Signature, 4), make(minhash.Signature, 4), make(minhash.Signature, 5))
}

// TestAccuracy checks the estimator against synthetic sets of known Jaccard
// similarity: the estimate must fall within 3 standard errors
// (3 * sqrt(J(1-J)/k)) of the true value.
func TestAccuracy(t *testing.T) {
	const k = 256
	const unique = 500
	for _, target := range []float64{0.1, 0.3, 0.5, 0.7, 0.9} {
		for _, seed := range []uint64{0, 1, 2} {
			shared := int(math.Round(2 * unique * target / (1 - target)))
			rng := hashutil.SplitMix64(1000*uint64(target*10) + seed)
			all := randSet(&rng, shared+2*unique)
			setA := all[:shared+unique]
			setB := append(slices.Clone(all[:shared]), all[shared+unique:]...)
			trueJ := float64(shared) / float64(shared+2*unique)

			m := minhash.New(k, seed)
			est := minhash.Jaccard(m.Sketch(seq(setA)), m.Sketch(seq(setB)))
			tol := 3 * math.Sqrt(trueJ*(1-trueJ)/k)
			if math.Abs(est-trueJ) > tol {
				t.Errorf("J=%.2f seed=%d: estimate %.4f outside %.4f ± %.4f",
					target, seed, est, trueJ, tol)
			}
		}
	}
}

func TestConcurrentSketch(t *testing.T) {
	rng := hashutil.SplitMix64(5)
	set := randSet(&rng, 500)
	m := minhash.New(64, 0)
	want := m.Sketch(seq(set))
	done := make(chan minhash.Signature)
	for range 8 {
		go func() {
			dst := make(minhash.Signature, 64)
			m.SketchInto(dst, seq(set))
			done <- dst
		}()
	}
	for range 8 {
		if got := <-done; !slices.Equal(got, want) {
			t.Error("concurrent SketchInto produced a different signature")
		}
	}
}

func TestResetUpdateMatchesSketch(t *testing.T) {
	rng := hashutil.SplitMix64(71)
	elems := make([]uint64, 700)
	for i := range elems {
		elems[i] = rng.Next()
	}
	m := minhash.New(64, 9)
	want := m.Sketch(seq(elems))

	// Any partitioning of the elements into Update calls is equivalent.
	for _, chunk := range []int{1, 3, 256, 700} {
		dst := make(minhash.Signature, 64)
		m.Reset(dst)
		for i := 0; i < len(elems); i += chunk {
			m.Update(dst, elems[i:min(i+chunk, len(elems))])
		}
		if !slices.Equal(dst, want) {
			t.Errorf("chunk=%d: Reset+Update diverges from Sketch", chunk)
		}
	}

	// Update with no hashes is a no-op; Reset yields the empty signature.
	dst := make(minhash.Signature, 64)
	m.Reset(dst)
	m.Update(dst, nil)
	for _, v := range dst {
		if v != math.MaxUint64 {
			t.Fatal("Reset+empty Update is not the empty-set signature")
		}
	}
}

func TestResetUpdatePanics(t *testing.T) {
	m := minhash.New(8, 0)
	for name, fn := range map[string]func(){
		"Reset":  func() { m.Reset(make(minhash.Signature, 7)) },
		"Update": func() { m.Update(make(minhash.Signature, 7), []uint64{1}) },
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("expected panic, got none")
				}
			}()
			fn()
		})
	}
}
