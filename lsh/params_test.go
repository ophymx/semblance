package lsh_test

import (
	"math"
	"testing"

	"github.com/ophymx/semblance/lsh"
)

func TestParams(t *testing.T) {
	cases := []struct {
		k      int
		target float64
	}{
		{128, 0.71}, {128, 0.85}, {128, 0.5}, {128, 0.3},
		{256, 0.9}, {120, 0.6}, {64, 0.75}, {100, 0.8},
	}
	for _, tc := range cases {
		b, r := lsh.Params(tc.k, tc.target)
		if b*r != tc.k {
			t.Errorf("Params(%d, %v) = %d*%d != %d", tc.k, tc.target, b, r, tc.k)
		}
		got := math.Abs(math.Pow(1/float64(b), 1/float64(r)) - tc.target)

		// No other factor pair is strictly closer to the target.
		best := math.Inf(1)
		for bb := 1; bb <= tc.k; bb++ {
			if tc.k%bb != 0 {
				continue
			}
			rr := tc.k / bb
			best = math.Min(best, math.Abs(math.Pow(1/float64(bb), 1/float64(rr))-tc.target))
		}
		if got > best+1e-12 {
			t.Errorf("Params(%d, %v) threshold distance %v, best achievable %v", tc.k, tc.target, got, best)
		}
	}

	// The frozen default banding is recovered from its threshold.
	if b, r := lsh.Params(128, 0.71); b != 16 || r != 8 {
		t.Errorf("Params(128, 0.71) = %d, %d; want 16, 8", b, r)
	}
	// The chosen banding actually feeds a working index of length k.
	b, r := lsh.Params(128, 0.85)
	if got := lsh.NewVerifiedIndex(b, r); got == nil {
		t.Fatal("Params result did not build a valid index")
	}
}

func TestParamsPanics(t *testing.T) {
	for name, fn := range map[string]func(){
		"k=0":      func() { lsh.Params(0, 0.7) },
		"k<0":      func() { lsh.Params(-1, 0.7) },
		"target=0": func() { lsh.Params(128, 0) },
		"target=1": func() { lsh.Params(128, 1) },
		"target>1": func() { lsh.Params(128, 1.5) },
		"target<0": func() { lsh.Params(128, -0.1) },
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
