package shingle

import (
	"unicode/utf8"

	"github.com/ophymx/semblance/internal/hashutil"
)

// WordScanner is the streaming form of [WordsBlocks]: text arrives in
// chunks via Write/WriteString, and the w-word shingle hashes are
// delivered in blocks through the flush callback. Any chunking of a
// document — including splits inside tokens or multi-byte runes —
// produces exactly the hash sequence of [Words] over the whole text.
//
// Internally, chunks are cut into segments at definite token boundaries
// (ASCII non-alphanumeric bytes, which can never occur inside a token or
// a multi-byte rune); the boundary-free tail of each chunk is carried
// until the next boundary arrives. The carry grows with the longest
// boundary-free span in the input, so pathological inputs (megabytes
// without an ASCII separator) buffer accordingly.
//
// Lifecycle: Write/WriteString any number of times, then [WordScanner.Finish]
// exactly once to emit the final token and flush the remainder;
// [WordScanner.Reset] readies the scanner for a new document. Writing
// after Finish (without Reset) panics. Not safe for concurrent use.
type WordScanner struct {
	w     int
	flush func(hashes []uint64) bool
	block []uint64
	// emit is s.token bound once at construction, so the per-token
	// indirect call does not allocate per Write.
	emit   func(uint64) bool
	window []uint64
	carry  []byte
	nTok   int  // tokens seen
	nb     int  // shingles buffered in block
	ok     bool // flush has not stopped the scan
	done   bool // Finish has been called
}

// NewWordScanner returns a scanner delivering w-word shingle hashes into
// block via flush, with [WordsBlocks] semantics (flush per full block and
// once for any remainder at Finish; returning false stops the scan).
// Panics if w <= 0, block is empty, or flush is nil.
func NewWordScanner(w int, block []uint64, flush func(hashes []uint64) bool) *WordScanner {
	if w <= 0 {
		panic("shingle: w must be positive")
	}
	if len(block) == 0 {
		panic("shingle: block must not be empty")
	}
	if flush == nil {
		panic("shingle: flush must not be nil")
	}
	s := &WordScanner{
		w:      w,
		flush:  flush,
		block:  block,
		window: make([]uint64, w),
		ok:     true,
	}
	s.emit = s.token
	return s
}

// Write implements io.Writer. It always accepts all of p (after flush has
// returned false, input is discarded). The bytes are not retained.
func (s *WordScanner) Write(p []byte) (int, error) {
	s.write(bytesToString(p))
	return len(p), nil
}

// WriteString implements io.StringWriter.
func (s *WordScanner) WriteString(text string) (int, error) {
	s.write(text)
	return len(text), nil
}

func (s *WordScanner) write(p string) {
	if s.done {
		panic("shingle: WordScanner written after Finish without Reset")
	}
	if !s.ok || len(p) == 0 {
		return
	}
	if len(s.carry) > 0 {
		i := firstASCIIBoundary(p)
		if i < 0 {
			s.carry = append(s.carry, p...)
			return
		}
		s.carry = append(s.carry, p[:i+1]...)
		tokenHashes(bytesToString(s.carry), s.emit)
		s.carry = s.carry[:0]
		if !s.ok {
			return
		}
		p = p[i+1:]
	}
	j := lastASCIIBoundary(p)
	if j < 0 {
		s.carry = append(s.carry, p...)
		return
	}
	tokenHashes(p[:j+1], s.emit)
	if s.ok {
		s.carry = append(s.carry, p[j+1:]...)
	}
}

// Finish emits the final in-progress token and flushes any buffered
// shingles. It must be called exactly once per document; panics if called
// again before Reset.
func (s *WordScanner) Finish() {
	if s.done {
		panic("shingle: WordScanner.Finish called twice without Reset")
	}
	s.done = true
	if !s.ok {
		return
	}
	if len(s.carry) > 0 {
		tokenHashes(bytesToString(s.carry), s.emit)
		s.carry = s.carry[:0]
	}
	if s.ok && s.nb > 0 {
		s.flush(s.block[:s.nb])
		s.nb = 0
	}
}

// Reset readies the scanner for a new document, keeping w, block, and
// flush. The carry buffer's capacity is retained.
func (s *WordScanner) Reset() {
	s.carry = s.carry[:0]
	s.nTok = 0
	s.nb = 0
	s.ok = true
	s.done = false
}

// token folds one token hash into the shingle window and block — the same
// arithmetic as WordsBlocks, kept in lockstep by fuzz equivalence tests.
func (s *WordScanner) token(th uint64) bool {
	s.window[s.nTok%s.w] = th
	s.nTok++
	if s.nTok < s.w {
		return true
	}
	acc := hashutil.MixInit
	for j := 0; j < s.w; j++ {
		acc = hashutil.Mix(acc, s.window[(s.nTok+j)%s.w])
	}
	s.block[s.nb] = acc
	s.nb++
	if s.nb == len(s.block) {
		s.ok = s.flush(s.block)
		s.nb = 0
		return s.ok
	}
	return true
}

func isASCIIAlnumByte(c byte) bool {
	return 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z' || '0' <= c && c <= '9'
}

// firstASCIIBoundary returns the index of the first byte that is a
// definite token boundary (ASCII, non-alphanumeric), or -1. Bytes >= 0x80
// are never definite boundaries: they may belong to a token rune or a
// multi-byte separator, so they stay with the carried segment.
func firstASCIIBoundary(p string) int {
	for i := 0; i < len(p); i++ {
		if c := p[i]; c < utf8.RuneSelf && !isASCIIAlnumByte(c) {
			return i
		}
	}
	return -1
}

func lastASCIIBoundary(p string) int {
	for i := len(p) - 1; i >= 0; i-- {
		if c := p[i]; c < utf8.RuneSelf && !isASCIIAlnumByte(c) {
			return i
		}
	}
	return -1
}
