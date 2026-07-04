package cluster_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/ophymx/semblance/cluster"
	"github.com/ophymx/semblance/internal/hashutil"
)

func TestBasics(t *testing.T) {
	s := cluster.New()
	s.Add("lonely")
	s.Union("a", "b")
	s.Union("c", "d")
	s.Union("b", "c") // transitively joins all four

	if got := s.Len(); got != 5 {
		t.Errorf("Len = %d, want 5", got)
	}
	if !s.Same("a", "d") {
		t.Error("a and d should be in the same cluster")
	}
	if s.Same("a", "lonely") || s.Same("a", "never-added") {
		t.Error("unrelated or unknown ids reported as same")
	}
	if rep, ok := s.Find("d"); !ok || rep != "a" {
		t.Errorf("Find(d) = %q, %v; want a (earliest member), true", rep, ok)
	}
	if _, ok := s.Find("never-added"); ok {
		t.Error("Find of unknown id reported ok")
	}

	want := [][]string{{"lonely"}, {"a", "b", "c", "d"}}
	if got := s.Clusters(); !slices.EqualFunc(got, want, slices.Equal) {
		t.Errorf("Clusters = %v, want %v", got, want)
	}
}

func TestEarliestMemberWins(t *testing.T) {
	// Regardless of union direction and order, the representative is the
	// earliest-added id of the merged set.
	s := cluster.New()
	s.Add("third") // inserted first despite the name
	s.Union("x", "y")
	s.Union("y", "third") // merging into a set whose oldest member is "third"
	if rep, _ := s.Find("x"); rep != "third" {
		t.Errorf("representative = %q, want third (earliest added)", rep)
	}

	// Idempotent unions change nothing.
	before := s.Clusters()
	s.Union("x", "third")
	s.Union("y", "x")
	if got := s.Clusters(); !slices.EqualFunc(got, before, slices.Equal) {
		t.Error("idempotent unions changed clusters")
	}
}

// TestOracle compares against naive label-propagation connected components
// over random union sequences.
func TestOracle(t *testing.T) {
	rng := hashutil.SplitMix64(121)
	const n = 200
	id := func(i uint64) string { return fmt.Sprintf("id%02d", i%n) }

	s := cluster.New()
	labels := map[string]string{} // naive: id → label (label = any member)
	relabel := func(from, to string) {
		for k, v := range labels {
			if v == from {
				labels[k] = to
			}
		}
	}
	for range 500 {
		a, b := id(rng.Next()), id(rng.Next())
		s.Union(a, b)
		la, oka := labels[a]
		lb, okb := labels[b]
		switch {
		case !oka && !okb:
			labels[a], labels[b] = a, a
		case oka && !okb:
			labels[b] = la
		case !oka && okb:
			labels[a] = lb
		case la != lb:
			relabel(lb, la)
		}
	}
	for x := range labels {
		for y := range labels {
			if got, want := s.Same(x, y), labels[x] == labels[y]; got != want {
				t.Fatalf("Same(%s, %s) = %v, oracle %v", x, y, got, want)
			}
		}
	}
	// Cluster count agrees.
	distinct := map[string]bool{}
	for _, l := range labels {
		distinct[l] = true
	}
	if got := len(s.Clusters()); got != len(distinct) {
		t.Errorf("cluster count %d, oracle %d", got, len(distinct))
	}
}

func TestDeterministicOutput(t *testing.T) {
	build := func() [][]string {
		s := cluster.New()
		s.Union("m", "n")
		s.Add("q")
		s.Union("o", "p")
		s.Union("p", "m")
		return s.Clusters()
	}
	first := build()
	for range 10 {
		if got := build(); !slices.EqualFunc(got, first, slices.Equal) {
			t.Fatalf("Clusters output varies across identical runs: %v vs %v", got, first)
		}
	}
	// Spec: cluster order by representative insertion, members in
	// insertion order, representative first.
	want := [][]string{{"m", "n", "o", "p"}, {"q"}}
	if !slices.EqualFunc(first, want, slices.Equal) {
		t.Errorf("Clusters = %v, want %v", first, want)
	}
}
