package sample_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/ophymx/semblance/sample"
)

func TestBelowCapacity(t *testing.T) {
	r := sample.New[string](5, 0)
	for _, s := range []string{"a", "b", "c"} {
		r.Add(s)
	}
	if got := r.Sample(); !slices.Equal(got, []string{"a", "b", "c"}) {
		t.Errorf("Sample = %v, want all items in order", got)
	}
	if r.N() != 3 || r.Len() != 3 {
		t.Errorf("N=%d Len=%d, want 3, 3", r.N(), r.Len())
	}
}

// TestUniformity checks that over many seeds, every stream item is
// selected at close to the expected k/n rate.
func TestUniformity(t *testing.T) {
	const (
		n      = 100
		k      = 10
		trials = 2000
	)
	counts := make([]int, n)
	for seed := range uint64(trials) {
		r := sample.New[int](k, seed)
		for i := range n {
			r.Add(i)
		}
		for _, item := range r.Sample() {
			counts[item]++
		}
	}
	// Expected trials*k/n = 200 per item; sd = sqrt(2000*0.1*0.9) ~ 13.4.
	// Allow ~4 sd.
	for item, c := range counts {
		if c < 145 || c > 255 {
			t.Errorf("item %d selected %d times, expected ~200 ± 55", item, c)
		}
	}
}

func TestDeterminismAndReset(t *testing.T) {
	run := func() []int {
		r := sample.New[int](4, 42)
		for i := range 1000 {
			r.Add(i)
		}
		return r.Sample()
	}
	first := run()
	if got := run(); !slices.Equal(got, first) {
		t.Fatal("identical streams and seed produced different samples")
	}

	// Reset restarts the random stream: resampling the same input gives
	// the identical sample.
	r := sample.New[int](4, 42)
	for i := range 1000 {
		r.Add(i)
	}
	r.Reset()
	if r.N() != 0 || r.Len() != 0 {
		t.Fatal("Reset did not empty the reservoir")
	}
	for i := range 1000 {
		r.Add(i)
	}
	if got := r.Sample(); !slices.Equal(got, first) {
		t.Error("Reset+replay diverged from a fresh reservoir")
	}

	// Different seeds sample differently (with overwhelming probability).
	other := sample.New[int](4, 43)
	for i := range 1000 {
		other.Add(i)
	}
	if slices.Equal(other.Sample(), first) {
		t.Error("different seeds produced identical samples")
	}
}

func TestMidStreamRead(t *testing.T) {
	r := sample.New[int](3, 7)
	for i := range 50 {
		r.Add(i)
	}
	mid := r.Sample()
	if len(mid) != 3 {
		t.Fatalf("mid-stream sample size %d, want 3", len(mid))
	}
	for i := 50; i < 100; i++ {
		r.Add(i)
	}
	if r.N() != 100 {
		t.Errorf("N = %d, want 100", r.N())
	}
	// The mid-stream copy is unaffected by later adds.
	if len(mid) != 3 {
		t.Error("returned sample mutated")
	}
}

func TestPanics(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("New(0) did not panic")
		}
	}()
	sample.New[int](0, 0)
}

// Pick reproducible representatives from a large group for triage.
func Example() {
	r := sample.New[string](3, 1)
	for i := range 10000 {
		r.Add(fmt.Sprintf("msg-%04d", i))
	}
	fmt.Println(r.Sample())
	fmt.Println(r.N(), "seen")
	// Output:
	// [msg-3058 msg-9269 msg-6440]
	// 10000 seen
}
