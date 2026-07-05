package topk_test

import (
	"fmt"

	"github.com/ophymx/semblance/topk"
)

// Flood detection: track the most frequent subjects in a stream with a
// bounded sketch. An entry whose lower bound (Count-Err) clears your
// threshold is a guaranteed flood signature, not a candidate.
func Example() {
	seen := topk.New[string](3)
	stream := append(
		make([]string, 0, 64),
		"MAKE MONEY FAST", "meeting minutes", "MAKE MONEY FAST", "lunch?",
		"MAKE MONEY FAST", "re: patch review", "MAKE MONEY FAST", "weekend plans",
		"MAKE MONEY FAST", "MAKE MONEY FAST", "build broken", "MAKE MONEY FAST",
	)
	for _, subject := range stream {
		seen.Add(subject)
	}

	for _, e := range seen.Top(1) {
		fmt.Printf("%q at least %d times\n", e.Item, e.Count-e.Err)
	}
	// Output:
	// "MAKE MONEY FAST" at least 7 times
}
