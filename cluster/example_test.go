package cluster_test

import (
	"fmt"

	semblance "github.com/ophymx/semblance"
	"github.com/ophymx/semblance/cluster"
	"github.com/ophymx/semblance/minhash"
)

// The complete dedup pipeline: sketch, query-then-add against the index,
// verify candidates, union verified pairs, and read one representative
// per cluster. Querying before adding makes clustering incremental — each
// document only compares against those before it.
func Example() {
	docs := []struct{ id, text string }{
		{"orig", "the city council voted last night to approve the new marina expansion project"},
		{"repost", "FWD: the city council voted last night to approve the new marina expansion project"},
		{"unrelated", "our weekly gardening column returns with advice on pruning roses in late summer"},
		{"repost2", "the city council voted last night to approve the new marina expansion project wow"},
	}

	sk := semblance.NewSketcher(semblance.Defaults())
	ix := sk.NewIndex()
	sigs := map[string]minhash.Signature{}
	cs := cluster.New()

	for _, d := range docs {
		sig := sk.Sketch(d.text)
		cs.Add(d.id)

		ids := ix.Query(sig) // candidates among earlier documents
		cands := make([]minhash.Signature, len(ids))
		for i, id := range ids {
			cands[i] = sigs[id]
		}
		for i, est := range minhash.JaccardMany(nil, sig, cands) {
			if est >= 0.5 {
				cs.Union(ids[i], d.id)
			}
		}

		ix.Add(d.id, sig)
		sigs[d.id] = sig
	}

	for _, c := range cs.Clusters() {
		fmt.Printf("%s (+%d duplicates)\n", c[0], len(c)-1)
	}
	// Output:
	// orig (+2 duplicates)
	// unrelated (+0 duplicates)
}
