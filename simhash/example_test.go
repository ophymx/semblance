package simhash_test

import (
	"fmt"

	"github.com/ophymx/semblance/simhash"
)

func Example() {
	a := simhash.SketchText("the quick brown fox jumps over the lazy dog", 3)
	b := simhash.SketchText("the quick brown fox leaps over the lazy dog", 3)
	c := simhash.SketchText("entirely unrelated text about simhash fingerprints", 3)
	fmt.Printf("similar:   %d bits\n", simhash.Distance(a, b))
	fmt.Printf("unrelated: %d bits\n", simhash.Distance(a, c))
	// Output:
	// similar:   15 bits
	// unrelated: 31 bits
}
