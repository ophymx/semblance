package semblance

import (
	"slices"

	"github.com/ophymx/semblance/minhash"
	"github.com/ophymx/semblance/shingle"
)

// Stream sketches a document that arrives in chunks — an io.Writer for
// text too large to hold as one string. Any chunking, including splits
// inside tokens or multi-byte runes, produces exactly the signature of
// [Sketcher.Sketch] over the whole text.
//
// Lifecycle: Write/WriteString the document, call [Stream.Signature] once
// to finalize, then [Stream.Reset] to start the next document. Writing
// after Signature (without Reset) panics. Not safe for concurrent use.
type Stream struct {
	hasher *minhash.MinHasher
	sig    minhash.Signature
	sc     *shingle.WordScanner
	done   bool
}

// NewStream returns a Stream sketching with this Sketcher's configuration.
// The Stream reuses its buffers across [Stream.Reset], so one Stream per
// worker amortizes all allocation.
func (s *Sketcher) NewStream() *Stream {
	st := &Stream{hasher: s.hasher, sig: make(minhash.Signature, s.cfg.K)}
	block := make([]uint64, 256)
	st.sc = shingle.NewWordScanner(s.cfg.W, block, func(hashes []uint64) bool {
		st.hasher.Update(st.sig, hashes)
		return true
	})
	s.hasher.Reset(st.sig)
	return st
}

// Write implements io.Writer. It always accepts all of p and never
// returns an error; the bytes are not retained.
func (st *Stream) Write(p []byte) (int, error) { return st.sc.Write(p) }

// WriteString implements io.StringWriter.
func (st *Stream) WriteString(text string) (int, error) { return st.sc.WriteString(text) }

// Signature finalizes the document (emitting any in-progress token) and
// returns its signature. The returned slice is a copy, valid after Reset.
// Idempotent: repeated calls return the same signature; Write between
// Signature and Reset panics, because the final token was already
// terminated.
func (st *Stream) Signature() minhash.Signature {
	if !st.done {
		st.sc.Finish()
		st.done = true
	}
	return slices.Clone(st.sig)
}

// Reset readies the Stream for a new document.
func (st *Stream) Reset() {
	st.hasher.Reset(st.sig)
	st.sc.Reset()
	st.done = false
}
