package minhash_test

import (
	"fmt"

	"github.com/ophymx/semblance/lsh"
	"github.com/ophymx/semblance/minhash"
	"github.com/ophymx/semblance/shingle"
)

func Example() {
	m := minhash.New(128, 0)
	a := m.Sketch(shingle.Words("the quick brown fox jumps over the lazy dog", 3))
	b := m.Sketch(shingle.Words("the quick brown fox leaps over the lazy dog", 3))
	c := m.Sketch(shingle.Words("entirely unrelated text about minhash sketches", 3))
	fmt.Printf("similar:   %.2f\n", minhash.Jaccard(a, b))
	fmt.Printf("unrelated: %.2f\n", minhash.Jaccard(a, c))
	// Output:
	// similar:   0.42
	// unrelated: 0.00
}

// ExampleJaccardMany shows the LSH verification pattern: the index returns
// candidate ids, the caller's signature store supplies their signatures,
// and JaccardMany verifies the whole list against a threshold in one call.
func ExampleJaccardMany() {
	m := minhash.New(128, 0)
	docs := map[string]string{
		"a": "the quick brown fox jumps over the lazy dog every morning",
		"b": "the quick brown fox jumps over the lazy dog every evening",
		"c": "an entirely unrelated sentence about locality sensitive hashing",
	}

	sigs := make(map[string]minhash.Signature, len(docs)) // caller's store
	ix := lsh.NewIndex(16, 8)
	for id, text := range docs {
		sigs[id] = m.Sketch(shingle.Words(text, 3))
		ix.Add(id, sigs[id])
	}

	query := m.Sketch(shingle.Words(
		"the quick brown fox jumps over the lazy dog every single morning", 3))
	ids := ix.Query(query)

	candidates := make([]minhash.Signature, len(ids))
	for i, id := range ids {
		candidates[i] = sigs[id]
	}
	for i, est := range minhash.JaccardMany(nil, query, candidates) {
		if est >= 0.5 {
			fmt.Printf("%s: %.2f\n", ids[i], est)
		}
	}
	// Output:
	// a: 0.70
}
