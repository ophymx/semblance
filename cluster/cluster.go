// Package cluster groups ids connected by pairwise matches — the final
// step of a dedup pipeline: sketch documents, index them, query for
// candidates, verify with minhash.JaccardMany, then [Set.Union] each
// verified pair and read back the connected components.
//
// Everything is deterministic given the same sequence of calls: the
// representative of a cluster is always its earliest-added member ("first
// document seen wins" — the natural keep-the-original rule), clusters are
// reported in order of their representatives' insertion, and members in
// insertion order. No result ever depends on map iteration order.
//
// A Set is not safe for concurrent use — including [Set.Find] and
// [Set.Same], which mutate internal state (path compression).
package cluster

// Set is a union-find over string ids with deterministic,
// earliest-member-wins representatives. The zero value is not usable; use
// [New]. Union and Find run in near-constant amortized time (path
// halving; union order is fixed by insertion age rather than rank, which
// keeps the representative semantics at a negligible practical cost).
type Set struct {
	index  map[string]int // id → insertion index
	ids    []string       // insertion order
	parent []int
}

// New returns an empty Set.
func New() *Set {
	return &Set{index: make(map[string]int)}
}

// Add registers id, initially in a cluster of its own. Adding an existing
// id is a no-op. Union registers its arguments itself; Add exists so that
// unmatched documents still appear as singleton clusters.
func (s *Set) Add(id string) {
	s.add(id)
}

func (s *Set) add(id string) int {
	if i, ok := s.index[id]; ok {
		return i
	}
	i := len(s.ids)
	s.index[id] = i
	s.ids = append(s.ids, id)
	s.parent = append(s.parent, i)
	return i
}

func (s *Set) find(i int) int {
	for s.parent[i] != i {
		s.parent[i] = s.parent[s.parent[i]] // path halving
		i = s.parent[i]
	}
	return i
}

// Union merges the clusters of a and b, registering either if new. The
// merged cluster's representative is its earliest-added member.
func (s *Set) Union(a, b string) {
	ra, rb := s.find(s.add(a)), s.find(s.add(b))
	if ra == rb {
		return
	}
	if rb < ra {
		ra, rb = rb, ra
	}
	s.parent[rb] = ra
}

// Find returns the representative of id's cluster — the earliest-added
// member — and whether id is known.
func (s *Set) Find(id string) (string, bool) {
	i, ok := s.index[id]
	if !ok {
		return "", false
	}
	return s.ids[s.find(i)], true
}

// Same reports whether a and b are known and in the same cluster.
func (s *Set) Same(a, b string) bool {
	ia, oka := s.index[a]
	ib, okb := s.index[b]
	return oka && okb && s.find(ia) == s.find(ib)
}

// Len returns the number of registered ids.
func (s *Set) Len() int { return len(s.ids) }

// Clusters returns every cluster (singletons included), deterministically:
// clusters in order of their representatives' insertion, members in
// insertion order, the representative first.
func (s *Set) Clusters() [][]string {
	groups := make(map[int][]string)
	var order []int
	for i, id := range s.ids {
		r := s.find(i)
		if _, ok := groups[r]; !ok {
			order = append(order, r)
		}
		groups[r] = append(groups[r], id)
	}
	out := make([][]string, 0, len(order))
	for _, r := range order {
		out = append(out, groups[r])
	}
	return out
}
