package minhash_test

import (
	"fmt"

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
