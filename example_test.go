package semblance_test

import (
	"fmt"

	semblance "github.com/ophymx/semblance"
)

// The five-line path: two strings to a similarity score.
func ExampleSimilarity() {
	sim := semblance.Similarity(
		"the quick brown fox jumps over the lazy dog",
		"the quick brown fox leaps over the lazy dog",
	)
	fmt.Printf("%.2f\n", sim)
	// Output: 0.42
}

func ExampleSketcher() {
	sk := semblance.NewSketcher(semblance.Defaults())
	ix := sk.NewIndex()
	ix.Add("doc1", sk.Sketch("the quick brown fox jumps over the lazy dog every single morning"))
	ix.Add("doc2", sk.Sketch("an entirely unrelated document about locality sensitive hashing"))

	candidates := ix.Query(sk.Sketch("the quick brown fox jumps over the lazy dog every single evening"))
	fmt.Println(candidates)
	// Output: [doc1]
}
