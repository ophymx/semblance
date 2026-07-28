package shingle_test

import (
	"fmt"
	"slices"

	"github.com/ophymx/semblance/minhash"
	"github.com/ophymx/semblance/shingle"
)

// Shingle streams plug directly into the sketchers: hash a document's
// word shingles, sketch them, and compare signatures.
func Example() {
	m := minhash.New(128, 0)
	a := m.Sketch(shingle.Words("the quick brown fox jumps over the lazy dog", 3))
	b := m.Sketch(shingle.Words("the quick brown fox leaps over the lazy dog", 3))
	fmt.Printf("%.2f\n", minhash.Jaccard(a, b))
	// Output:
	// 0.42
}

// Words tokenizes on Unicode letter/number runs and lowercases, so
// punctuation and case differences do not change the shingle stream.
func ExampleWords() {
	a := slices.Collect(shingle.Words("The quick brown Fox!", 3))
	b := slices.Collect(shingle.Words("the quick brown fox", 3))
	fmt.Println(len(a), slices.Equal(a, b))
	// Output:
	// 2 true
}

// Char hashes every overlapping k-byte window; identical windows yield
// identical hashes wherever they occur.
func ExampleChar() {
	hashes := slices.Collect(shingle.Char("banana", 3)) // ban ana nan ana
	fmt.Println(len(hashes), hashes[1] == hashes[3])
	// Output:
	// 4 true
}
