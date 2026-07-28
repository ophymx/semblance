// Package winnow implements winnowing document fingerprints (Schleimer,
// Wilkerson & Aiken, "Winnowing: Local Algorithms for Document
// Fingerprinting", 2003 — the MOSS algorithm): a position-aware selection
// of shingle hashes for locating where two documents overlap, not just
// how much.
//
// From a document's sequence of k-gram hashes, every window of w
// consecutive hashes contributes its minimum (rightmost on ties) as a
// fingerprint, recorded with the position it came from. The guarantee:
// any common substring of length at least w+k-1 bytes shares at least one
// fingerprint hash between the two documents, and no match shorter than k
// is ever detected. Expected fingerprint density is 2/(w+1) per hash.
//
// This is standard winnowing, not the paper's "robust" tie-breaking
// variant: standard keeps the strict match guarantee, which robust trades
// for fewer fingerprints in highly repetitive text. The selection scheme
// is frozen — stored fingerprints remain comparable within a major
// version.
//
// [Text] uses byte k-grams ([shingle.Char]), so its output is
// unconditionally stable across Unicode versions.
package winnow

import (
	"iter"

	"github.com/ophymx/semblance/shingle"
)

// Fingerprint is a selected shingle hash and where it was selected: the
// byte offset of the k-gram for [Text]/[TextBytes], or the sequence index
// for [Hashes].
type Fingerprint struct {
	Pos  int    // byte offset ([Text]) or sequence index ([Hashes]) of the selected shingle
	Hash uint64 // the selected shingle hash
}

// Text returns an iterator over the winnowing fingerprints of text using
// byte k-grams, with Pos the byte offset of each selected k-gram
// (Hash == xxhash of text[Pos:Pos+k]). Documents sharing a substring of
// w+k-1 or more bytes share at least one fingerprint hash. Text shorter
// than w+k-1 bytes yields no fingerprints. Panics if k <= 0 or w <= 0.
func Text(text string, k, w int) iter.Seq[Fingerprint] {
	return Hashes(shingle.Char(text, k), w)
}

// TextBytes is [Text] for a byte slice, without copying. The slice must
// not be mutated until iteration completes.
func TextBytes(b []byte, k, w int) iter.Seq[Fingerprint] {
	return Hashes(shingle.CharBytes(b, k), w)
}

// Hashes selects winnowing fingerprints from any shingle hash sequence:
// each window of w consecutive hashes contributes its minimum (rightmost
// on ties), deduplicated, with Pos the 0-based index into the sequence.
// Fingerprints are yielded in increasing Pos order. Sequences shorter
// than w yield nothing. Panics if w <= 0.
func Hashes(hashes iter.Seq[uint64], w int) iter.Seq[Fingerprint] {
	if w <= 0 {
		panic("winnow: w must be positive")
	}
	return func(yield func(Fingerprint) bool) {
		ring := make([]uint64, w)
		idx := 0     // index of the incoming hash
		minPos := -1 // position of the current selection
		for h := range hashes {
			ring[idx%w] = h
			switch {
			case idx < w-1:
				// First window not yet complete.
			case minPos < idx-w+1:
				// First complete window, or the selection just slid out:
				// rescan for the rightmost minimum. The new selection is
				// at a position newer than any previously selected, so it
				// is always a new fingerprint.
				minPos = idx - w + 1
				for p := minPos + 1; p <= idx; p++ {
					if ring[p%w] <= ring[minPos%w] {
						minPos = p
					}
				}
				if !yield(Fingerprint{Pos: minPos, Hash: ring[minPos%w]}) {
					return
				}
			case h <= ring[minPos%w]:
				// The incoming hash is a new rightmost minimum.
				minPos = idx
				if !yield(Fingerprint{Pos: idx, Hash: h}) {
					return
				}
			}
			idx++
		}
	}
}
