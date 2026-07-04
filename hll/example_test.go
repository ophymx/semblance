package hll_test

import (
	"fmt"

	"github.com/ophymx/semblance/hll"
	"github.com/ophymx/semblance/shingle"
)

// Feeding shingle streams counts distinct text features — here, the
// distinct words across a small corpus (w=1 shingles are single tokens).
func Example() {
	distinct := hll.New(14)
	for _, doc := range []string{
		"the quick brown fox jumps over the lazy dog",
		"the dog sleeps while the quick cat watches",
	} {
		distinct.AddSeq(shingle.Words(doc, 1))
	}
	fmt.Printf("distinct words: %.0f\n", distinct.Estimate())
	// Output: distinct words: 12
}
