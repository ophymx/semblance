package hll_test

import (
	"encoding"
	"encoding/hex"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/ophymx/semblance/hll"
	"github.com/ophymx/semblance/internal/hashutil"
)

var (
	_ encoding.BinaryMarshaler   = (*hll.Sketch)(nil)
	_ encoding.BinaryAppender    = (*hll.Sketch)(nil)
	_ encoding.BinaryUnmarshaler = (*hll.Sketch)(nil)
)

// TestAccuracy checks estimates against known cardinalities within 3
// standard errors (1.04/sqrt(m)), widened near the classic estimator's
// bias hump at n ~ 2.5m.
func TestAccuracy(t *testing.T) {
	for _, p := range []int{10, 14} {
		m := float64(uint64(1) << p)
		stderr := 1.04 / math.Sqrt(m)
		for _, n := range []int{0, 1, 10, 100, 5000, 40000, 200000, 1000000} {
			for _, seed := range []uint64{1, 2} {
				s := hll.New(p)
				rng := hashutil.SplitMix64(uint64(n)*3 + seed)
				for range n {
					s.Add(rng.Next())
				}
				got := s.Estimate()
				tol := 3 * stderr
				if lo, hi := 1.5*m, 4*m; float64(n) > lo && float64(n) < hi {
					tol += 0.02 // transition bias hump
				}
				if n == 0 {
					if got != 0 {
						t.Errorf("p=%d: empty sketch estimates %v, want 0", p, got)
					}
					continue
				}
				if relErr := math.Abs(got-float64(n)) / float64(n); relErr > tol {
					t.Errorf("p=%d n=%d seed=%d: estimate %.0f, relative error %.4f > %.4f",
						p, n, seed, got, relErr, tol)
				}
			}
		}
	}
}

func TestDuplicatesAndOrder(t *testing.T) {
	rng := hashutil.SplitMix64(7)
	elems := make([]uint64, 5000)
	for i := range elems {
		elems[i] = rng.Next()
	}

	a := hll.New(12)
	a.AddSeq(slices.Values(elems))
	single := a.Estimate()

	// Duplicates never move the estimate; order never matters.
	b := hll.New(12)
	rev := slices.Clone(elems)
	slices.Reverse(rev)
	for range 3 {
		b.AddSeq(slices.Values(rev))
	}
	if got := b.Estimate(); got != single {
		t.Errorf("duplicated/reordered stream estimates %v, want %v", got, single)
	}

	ab, _ := a.MarshalBinary()
	bb, _ := b.MarshalBinary()
	if !slices.Equal(ab, bb) {
		t.Error("registers differ despite identical element sets")
	}
}

func TestMergeIsUnion(t *testing.T) {
	rng := hashutil.SplitMix64(8)
	all := make([]uint64, 30000)
	for i := range all {
		all[i] = rng.Next()
	}
	overlapA, overlapB := all[:20000], all[10000:] // 10k shared

	a, b, both := hll.New(12), hll.New(12), hll.New(12)
	a.AddSeq(slices.Values(overlapA))
	b.AddSeq(slices.Values(overlapB))
	both.AddSeq(slices.Values(all))

	a.Merge(b)
	am, _ := a.MarshalBinary()
	bm, _ := both.MarshalBinary()
	if !slices.Equal(am, bm) {
		t.Fatal("Merge(a, b) registers differ from sketching the union directly")
	}
}

func TestMergePanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("Merge with mismatched precision did not panic")
		}
	}()
	hll.New(10).Merge(hll.New(12))
}

func TestNewPanics(t *testing.T) {
	for _, p := range []int{0, 3, 19} {
		func() {
			defer func() {
				if recover() == nil {
					t.Errorf("New(%d) did not panic", p)
				}
			}()
			hll.New(p)
		}()
	}
}

func TestReset(t *testing.T) {
	s := hll.New(10)
	s.Add(42)
	s.Reset()
	if got := s.Estimate(); got != 0 {
		t.Errorf("estimate after Reset = %v, want 0", got)
	}
}

func TestMarshalRoundTrip(t *testing.T) {
	rng := hashutil.SplitMix64(9)
	s := hll.New(10)
	for range 5000 {
		s.Add(rng.Next())
	}
	data, err := s.MarshalBinary()
	if err != nil {
		t.Fatal(err)
	}
	if want := 3 + 1<<10; len(data) != want {
		t.Fatalf("marshaled length %d, want %d", len(data), want)
	}
	var got hll.Sketch
	if err := got.UnmarshalBinary(data); err != nil {
		t.Fatal(err)
	}
	if got.Estimate() != s.Estimate() || got.P() != 10 {
		t.Error("round trip changed the sketch")
	}
	// The restored sketch keeps working (mergeable, addable).
	got.Merge(s)
	got.Add(1)
}

// TestMarshalGoldenBytes pins the wire format with hand-written expected
// bytes: version 01, algo 03, p=4, then the 16 registers.
func TestMarshalGoldenBytes(t *testing.T) {
	s := hll.New(4)
	for _, x := range []uint64{1, 2, 3} {
		s.Add(x)
	}
	data, _ := s.MarshalBinary()
	const wantHeader = "010304"
	got := hex.EncodeToString(data)
	if !strings.HasPrefix(got, wantHeader) || len(got) != 2*(3+16) {
		t.Fatalf("wire bytes = %s, want header %s + 16 registers", got, wantHeader)
	}
	const golden = "01030400010000000000000004000000000000"
	if got != golden {
		t.Errorf("wire bytes\n got %s\nwant %s — stored sketches are BROKEN if intentional", got, golden)
	}
}

func TestUnmarshalErrors(t *testing.T) {
	good, _ := hll.New(4).MarshalBinary()
	tests := map[string]struct {
		data    []byte
		wantErr string
	}{
		"empty":       {nil, "truncated"},
		"bad version": {append([]byte{9}, good[1:]...), "version"},
		"bad algo":    {append([]byte{1, 9}, good[2:]...), "algorithm"},
		"bad p":       {append([]byte{1, 3, 99}, good[3:]...), "precision"},
		"short":       {good[:len(good)-1], "must be"},
		"long":        {append(slices.Clone(good), 0), "must be"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			var s hll.Sketch
			if err := s.UnmarshalBinary(tt.data); err == nil {
				t.Error("UnmarshalBinary succeeded on malformed input")
			} else if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not mention %q", err, tt.wantErr)
			}
		})
	}
}

func TestMix64MatchesSplitMix(t *testing.T) {
	// Mix64 is frozen by its defining relation to the golden-pinned
	// SplitMix64 stream.
	for _, x := range []uint64{0, 1, 42, ^uint64(0)} {
		rng := hashutil.SplitMix64(x)
		if hashutil.Mix64(x) != rng.Next() {
			t.Fatalf("Mix64(%d) != SplitMix64(%d).Next()", x, x)
		}
	}
}
