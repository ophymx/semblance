package semblance_test

import (
	"fmt"
	"io"
	"strings"

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

// Documents too large to hold in memory stream through an io.Writer; any
// chunking produces the same signature as sketching the whole text.
func ExampleSketcher_NewStream() {
	sk := semblance.NewSketcher(semblance.Defaults())

	text := "the quick brown fox jumps over the lazy dog"
	st := sk.NewStream()
	io.Copy(st, strings.NewReader(text)) // e.g. an os.File in practice

	fmt.Println(semblance.NewSketcher(semblance.Defaults()).Sketch(text)[0] == st.Signature()[0])
	// Output: true
}
