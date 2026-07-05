package topk_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/ophymx/semblance/internal/hashutil"
	"github.com/ophymx/semblance/topk"
)

func TestExactBelowCapacity(t *testing.T) {
	s := topk.New[string](8)
	for i, item := range []string{"a", "b", "c", "a", "b", "a"} {
		_ = i
		s.Add(item)
	}
	want := []topk.Entry[string]{
		{Item: "a", Count: 3},
		{Item: "b", Count: 2},
		{Item: "c", Count: 1},
	}
	if got := s.Top(10); !slices.Equal(got, want) {
		t.Errorf("Top = %v, want %v", got, want)
	}
	if c, e := s.Count("b"); c != 2 || e != 0 {
		t.Errorf("Count(b) = %d, %d; want 2, 0", c, e)
	}
	if c, e := s.Count("never"); c != 0 || e != 0 {
		t.Errorf("Count(never) below capacity = %d, %d; want 0, 0", c, e)
	}
	if s.N() != 6 || s.Len() != 3 {
		t.Errorf("N=%d Len=%d, want 6, 3", s.N(), s.Len())
	}
}

func TestEvictionSmallCase(t *testing.T) {
	// capacity 2, stream a a b c: c evicts b (the minimum), inheriting
	// its count 1 as error.
	s := topk.New[string](2)
	for _, item := range []string{"a", "a", "b", "c"} {
		s.Add(item)
	}
	want := []topk.Entry[string]{
		{Item: "a", Count: 2, Err: 0},
		{Item: "c", Count: 2, Err: 1},
	}
	if got := s.Top(2); !slices.Equal(got, want) {
		t.Errorf("Top = %v, want %v", got, want)
	}
	// b is gone; its bound is the minimum counter.
	if c, e := s.Count("b"); c != 2 || e != 2 {
		t.Errorf("Count(b) = %d, %d; want 2, 2", c, e)
	}
}

// zipfStream builds a deterministic stream with known heavy items and a
// long tail of unique noise, interleaved, returning true counts.
func zipfStream(seed uint64, heavy map[string]int, noise int) ([]string, map[string]int) {
	rng := hashutil.SplitMix64(seed)
	var stream []string
	truth := map[string]int{}
	for item, n := range map[string]int{"h1": heavy["h1"], "h2": heavy["h2"], "h3": heavy["h3"]} {
		for range n {
			stream = append(stream, item)
		}
		truth[item] = n
	}
	for i := range noise {
		item := fmt.Sprintf("noise%06d", i)
		stream = append(stream, item)
		truth[item]++
	}
	// Deterministic shuffle.
	for i := len(stream) - 1; i > 0; i-- {
		j := int(rng.Next() % uint64(i+1))
		stream[i], stream[j] = stream[j], stream[i]
	}
	return stream, truth
}

func TestGuarantees(t *testing.T) {
	heavy := map[string]int{"h1": 4000, "h2": 2500, "h3": 900}
	stream, truth := zipfStream(141, heavy, 5000)
	const capacity = 64
	s := topk.New[string](capacity)
	for _, item := range stream {
		s.Add(item)
	}

	n := s.N()
	if int(n) != len(stream) {
		t.Fatalf("N = %d, want %d", n, len(stream))
	}
	threshold := n / capacity

	// Every item with true count > n/capacity must be present with
	// correct bounds.
	top := s.Top(capacity)
	reported := map[string]topk.Entry[string]{}
	for _, e := range top {
		reported[e.Item] = e
	}
	for item, true_ := range truth {
		if uint64(true_) > threshold {
			e, ok := reported[item]
			if !ok {
				t.Fatalf("heavy item %q (count %d > %d) missing", item, true_, threshold)
			}
			if uint64(true_) > e.Count || uint64(true_) < e.Count-e.Err {
				t.Errorf("%q: true %d outside [%d, %d]", item, true_, e.Count-e.Err, e.Count)
			}
		}
	}
	// Bounds hold for everything reported, and Err <= n/capacity.
	for _, e := range top {
		if uint64(truth[e.Item]) > e.Count || uint64(truth[e.Item]) < e.Count-e.Err {
			t.Errorf("%q: true %d outside [%d, %d]", e.Item, truth[e.Item], e.Count-e.Err, e.Count)
		}
		if e.Err > threshold+1 {
			t.Errorf("%q: Err %d exceeds n/capacity %d", e.Item, e.Err, threshold)
		}
	}
	// The three planted heavies lead the report.
	if top[0].Item != "h1" || top[1].Item != "h2" || top[2].Item != "h3" {
		t.Errorf("top 3 = %v %v %v, want h1 h2 h3", top[0].Item, top[1].Item, top[2].Item)
	}
}

func TestMerge(t *testing.T) {
	heavyA := map[string]int{"h1": 3000, "h2": 200, "h3": 100}
	heavyB := map[string]int{"h1": 100, "h2": 2500, "h3": 90}
	streamA, truthA := zipfStream(142, heavyA, 3000)
	streamB, truthB := zipfStream(143, heavyB, 3000)

	a := topk.New[string](64)
	for _, item := range streamA {
		a.Add(item)
	}
	b := topk.New[string](64)
	for _, item := range streamB {
		b.Add(item)
	}
	a.Merge(b)

	if a.N() != uint64(len(streamA)+len(streamB)) {
		t.Fatalf("merged N = %d, want %d", a.N(), len(streamA)+len(streamB))
	}
	// Bounds hold against combined truth; items heavy in either stream
	// (h1 heavy in A, h2 heavy in B) are present.
	combined := map[string]int{}
	for k, v := range truthA {
		combined[k] += v
	}
	for k, v := range truthB {
		combined[k] += v
	}
	top := a.Top(64)
	reported := map[string]topk.Entry[string]{}
	for _, e := range top {
		reported[e.Item] = e
	}
	for _, item := range []string{"h1", "h2"} {
		e, ok := reported[item]
		if !ok {
			t.Fatalf("%q missing after merge", item)
		}
		if uint64(combined[item]) > e.Count || uint64(combined[item]) < e.Count-e.Err {
			t.Errorf("%q: true %d outside [%d, %d]", item, combined[item], e.Count-e.Err, e.Count)
		}
	}
	for _, e := range top {
		if uint64(combined[e.Item]) > e.Count {
			t.Errorf("%q: upper bound violated after merge: true %d > %d", e.Item, combined[e.Item], e.Count)
		}
	}
}

func TestAddNAndReset(t *testing.T) {
	s := topk.New[uint64](4)
	s.AddN(7, 1000)
	s.AddN(8, 0) // no-op
	s.Add(9)
	if got := s.Top(1)[0]; got.Item != 7 || got.Count != 1000 {
		t.Errorf("Top(1) = %v, want item 7 count 1000", got)
	}
	if s.N() != 1001 || s.Len() != 2 {
		t.Errorf("N=%d Len=%d, want 1001, 2", s.N(), s.Len())
	}
	s.Reset()
	if s.N() != 0 || s.Len() != 0 || len(s.Top(5)) != 0 {
		t.Error("Reset did not empty the sketch")
	}
}

func TestDeterminism(t *testing.T) {
	stream, _ := zipfStream(144, map[string]int{"h1": 500, "h2": 300, "h3": 200}, 2000)
	run := func() []topk.Entry[string] {
		s := topk.New[string](32)
		for _, item := range stream {
			s.Add(item)
		}
		return s.Top(32)
	}
	first := run()
	for range 3 {
		if got := run(); !slices.Equal(got, first) {
			t.Fatal("identical streams produced different sketches")
		}
	}
}

func TestPanics(t *testing.T) {
	for name, fn := range map[string]func(){
		"capacity=0": func() { topk.New[int](0) },
		"top k=0":    func() { topk.New[int](4).Top(0) },
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
