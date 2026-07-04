// Package shingle converts text into streams of shingle hashes: xxhash
// values of overlapping character k-grams or word w-grams, suitable for
// sketching with the minhash and simhash packages.
//
// Shingles are hashed incrementally as the text is scanned; no shingle
// substrings are materialized. Each iterator performs a small constant
// number of allocations regardless of document size.
//
// # Determinism
//
// For a given input and parameters, every function in this package produces
// the same hash stream on every platform and in every process. One caveat:
// [Words] classifies and lowercases runes using the stdlib unicode tables,
// which track the Unicode version shipped with the Go toolchain, so its
// output is guaranteed stable per Unicode version. Pure-ASCII text is
// unconditionally stable. [Char] and [CharRunes] do not consult the unicode
// tables and are unconditionally stable.
package shingle

import (
	"iter"
	"unicode"
	"unicode/utf8"
	"unsafe"

	"github.com/cespare/xxhash/v2"

	"github.com/ophymx/semblance/internal/hashutil"
)

// The *Bytes variants share the string implementations through a zero-copy
// view (unsafe.String): no bytes are copied, mutated, or retained beyond
// iteration. In exchange, the caller must not mutate the slice until
// iteration of the returned sequence completes.

func bytesToString(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	return unsafe.String(unsafe.SliceData(b), len(b))
}

// Char returns an iterator over xxhash values of the overlapping k-byte
// windows of text. Windows are byte-based; multi-byte UTF-8 sequences may be
// split across window boundaries, which is fine for similarity heuristics.
// Use [CharRunes] for Unicode-correct k-grams.
//
// Text shorter than k bytes yields no values. Panics if k <= 0.
func Char(text string, k int) iter.Seq[uint64] {
	if k <= 0 {
		panic("shingle: k must be positive")
	}
	return func(yield func(uint64) bool) {
		for i := 0; i+k <= len(text); i++ {
			if !yield(xxhash.Sum64String(text[i : i+k])) {
				return
			}
		}
	}
}

// CharBytes is [Char] for a byte slice, without copying. The slice must not
// be mutated until iteration completes.
func CharBytes(b []byte, k int) iter.Seq[uint64] {
	return Char(bytesToString(b), k)
}

// CharRunes returns an iterator over xxhash values of the overlapping k-rune
// windows of text. Invalid UTF-8 bytes are treated as one rune each
// (utf8.RuneError semantics), deterministically.
//
// Text shorter than k runes yields no values. Panics if k <= 0.
func CharRunes(text string, k int) iter.Seq[uint64] {
	if k <= 0 {
		panic("shingle: k must be positive")
	}
	return func(yield func(uint64) bool) {
		starts := make([]int, k) // ring buffer of window start offsets
		n := 0                   // runes consumed
		for i := 0; i < len(text); {
			_, size := utf8.DecodeRuneInString(text[i:])
			starts[n%k] = i
			n++
			i += size
			// After n runes the oldest offset in the ring is the start of
			// the window of the last k runes: index (n-k)%k == n%k.
			if n >= k {
				if !yield(xxhash.Sum64String(text[starts[n%k]:i])) {
					return
				}
			}
		}
	}
}

// CharRunesBytes is [CharRunes] for a byte slice, without copying. The
// slice must not be mutated until iteration completes.
func CharRunesBytes(b []byte, k int) iter.Seq[uint64] {
	return CharRunes(bytesToString(b), k)
}

// Words returns an iterator over hashes of the overlapping w-word shingles
// of text. Tokens are maximal runs of Unicode letters and numbers,
// lowercased with unicode.ToLower (simple lowercasing, not full case
// folding; ASCII fast path). No stemming, no stopwords — pre-process the
// text if you need that.
//
// Each token is hashed with xxhash over its lowercased UTF-8 bytes; the w
// token hashes of a shingle are combined with a frozen order-sensitive
// mixing function, so "a b c" and "c b a" produce different shingles.
//
// Text with fewer than w tokens yields no values. Panics if w <= 0.
func Words(text string, w int) iter.Seq[uint64] {
	if w <= 0 {
		panic("shingle: w must be positive")
	}
	return func(yield func(uint64) bool) {
		window := make([]uint64, w) // ring buffer of token hashes
		n := 0                      // tokens seen
		tokenHashes(text, func(th uint64) bool {
			window[n%w] = th
			n++
			if n < w {
				return true
			}
			h := hashutil.MixInit
			for j := 0; j < w; j++ {
				h = hashutil.Mix(h, window[(n+j)%w])
			}
			return yield(h)
		})
	}
}

// WordsBytes is [Words] for a byte slice, without copying. The slice must
// not be mutated until iteration completes.
func WordsBytes(b []byte, w int) iter.Seq[uint64] {
	return Words(bytesToString(b), w)
}

// WordsBlocks scans text once and delivers its w-word shingle hashes in
// blocks: it fills block and calls flush(block[:n]) each time the block
// fills, and once more with any remainder. flush is never called with an
// empty slice; returning false stops the scan. The hash sequence is
// exactly that of [Words] — WordsBlocks is the fused path used by the
// convenience sketchers, trading the per-shingle iterator hand-off for one
// callback per block.
//
// Panics if w <= 0 or len(block) == 0.
func WordsBlocks(text string, w int, block []uint64, flush func(hashes []uint64) bool) {
	if w <= 0 {
		panic("shingle: w must be positive")
	}
	if len(block) == 0 {
		panic("shingle: block must not be empty")
	}
	window := make([]uint64, w) // ring buffer of token hashes
	n := 0                      // tokens seen
	nb := 0                     // shingles buffered
	ok := true
	tokenHashes(text, func(th uint64) bool {
		window[n%w] = th
		n++
		if n < w {
			return true
		}
		acc := hashutil.MixInit
		for j := 0; j < w; j++ {
			acc = hashutil.Mix(acc, window[(n+j)%w])
		}
		block[nb] = acc
		nb++
		if nb == len(block) {
			ok = flush(block)
			nb = 0
			return ok
		}
		return true
	})
	if ok && nb > 0 {
		flush(block[:nb])
	}
}

// WordsBlocksBytes is [WordsBlocks] for a byte slice, without copying. The
// slice must not be mutated before the scan completes.
func WordsBlocksBytes(b []byte, w int, block []uint64, flush func(hashes []uint64) bool) {
	WordsBlocks(bytesToString(b), w, block, flush)
}

// tokenHashes scans text for tokens (maximal runs of letters/numbers),
// lowercases them, and calls emit with the xxhash of each token's lowercased
// UTF-8 bytes. Stops early if emit returns false.
//
// Tokens that are entirely lowercase ASCII alphanumerics — the common case
// in real text — are hashed directly from the string with no copying; only
// tokens containing uppercase or non-ASCII runes go through the fold
// buffer. Both paths hash identical byte sequences.
func tokenHashes(text string, emit func(uint64) bool) {
	buf := make([]byte, 0, 64) // reused lowercased-token buffer
	for i := 0; i < len(text); {
		// Find the token start.
		var r rune
		var size int
		if c := text[i]; c < utf8.RuneSelf {
			r, size = rune(c), 1
		} else {
			r, size = utf8.DecodeRuneInString(text[i:])
		}
		if !isTokenRune(r) {
			i += size
			continue
		}

		// Fast path: consume a lowercase-ASCII alphanumeric run.
		start := i
		for i < len(text) && isLowerASCIIToken(text[i]) {
			i++
		}

		// Decide what stopped the run.
		if i >= len(text) {
			emit(xxhash.Sum64String(text[start:i]))
			return
		}
		if c := text[i]; c < utf8.RuneSelf {
			if !isTokenRune(rune(c)) { // ASCII boundary: token was all-fast
				if !emit(xxhash.Sum64String(text[start:i])) {
					return
				}
				i++
				continue
			}
		} else {
			r, size = utf8.DecodeRuneInString(text[i:])
			if !isTokenRune(r) { // non-token rune boundary
				if !emit(xxhash.Sum64String(text[start:i])) {
					return
				}
				i += size
				continue
			}
		}

		// Slow path: the token continues with an uppercase or non-ASCII
		// rune. Carry over the already-consumed prefix (it is lowercase
		// ASCII, so it folds to itself) and finish rune-by-rune.
		buf = append(buf[:0], text[start:i]...)
		for i < len(text) {
			if c := text[i]; c < utf8.RuneSelf {
				r, size = rune(c), 1
			} else {
				r, size = utf8.DecodeRuneInString(text[i:])
			}
			if !isTokenRune(r) {
				break
			}
			buf = appendLowerRune(buf, r)
			i += size
		}
		if !emit(xxhash.Sum64(buf)) {
			return
		}
	}
}

func isLowerASCIIToken(c byte) bool {
	return 'a' <= c && c <= 'z' || '0' <= c && c <= '9'
}

func isTokenRune(r rune) bool {
	switch {
	case 'a' <= r && r <= 'z' || 'A' <= r && r <= 'Z' || '0' <= r && r <= '9':
		return true
	case r < utf8.RuneSelf:
		return false
	}
	return unicode.IsLetter(r) || unicode.IsNumber(r)
}

func appendLowerRune(buf []byte, r rune) []byte {
	if r < utf8.RuneSelf {
		if 'A' <= r && r <= 'Z' {
			r += 'a' - 'A'
		}
		return append(buf, byte(r))
	}
	return utf8.AppendRune(buf, unicode.ToLower(r))
}
