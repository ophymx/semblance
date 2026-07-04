// Package semblance provides deterministic, heuristic text-similarity
// sketching: shingling, MinHash, SimHash, and LSH indexing under one API.
// The name nods to Broder's "On the resemblance and containment of
// documents" (1997), which introduced shingle-based resemblance and minwise
// hashing.
//
// This package is the convenience layer: two strings to a similarity score
// with sensible frozen defaults. The sub-packages (shingle, minhash,
// simhash, lsh) are each usable alone for anything more custom.
//
// Everything is heuristic with known error bounds (see the sub-package
// docs) and deterministic: same input, same parameters, same seed → same
// signature, on every platform. Word tokenization shares the unicode-tables
// caveat documented in the shingle package.
package semblance

import (
	"math"

	"github.com/ophymx/semblance/lsh"
	"github.com/ophymx/semblance/minhash"
	"github.com/ophymx/semblance/shingle"
)

// Config parameterizes a Sketcher. The zero value is not valid; start from
// [Defaults].
type Config struct {
	// W is the word-shingle width.
	W int
	// K is the MinHash signature length. Standard error of similarity
	// estimates is about 1/(2*sqrt(K)).
	K int
	// Seed selects the permutation family. Signatures are only comparable
	// across equal (K, Seed).
	Seed uint64
	// Bands and Rows shape LSH indexes created by [Sketcher.NewIndex];
	// Bands*Rows must equal K. Leave both zero if you never build an index.
	Bands, Rows int
}

// Defaults returns the frozen default configuration: word shingles of
// width 3, signatures of length 128 with seed 0, and 16 bands of 8 rows
// (LSH candidate threshold ≈ 0.71). These values will not change within a
// major version; signatures sketched with Defaults today remain comparable
// with signatures sketched with Defaults in any future v1 release.
func Defaults() Config {
	return Config{W: 3, K: 128, Seed: 0, Bands: 16, Rows: 8}
}

// Sketcher turns texts into MinHash signatures using a fixed configuration.
// It is immutable after creation and safe for concurrent use, provided
// concurrent SketchInto calls use distinct dst buffers.
type Sketcher struct {
	cfg    Config
	hasher *minhash.MinHasher
}

// NewSketcher returns a Sketcher for the configuration. Panics if W <= 0,
// K <= 0, or Bands/Rows are set (nonzero) but invalid or inconsistent with
// K.
func NewSketcher(cfg Config) *Sketcher {
	if cfg.W <= 0 {
		panic("semblance: Config.W must be positive")
	}
	if cfg.Bands != 0 || cfg.Rows != 0 {
		if cfg.Bands <= 0 || cfg.Rows <= 0 {
			panic("semblance: Config.Bands and Config.Rows must both be positive or both zero")
		}
		if cfg.Bands*cfg.Rows != cfg.K {
			panic("semblance: Config.Bands*Config.Rows must equal Config.K")
		}
	}
	return &Sketcher{cfg: cfg, hasher: minhash.New(cfg.K, cfg.Seed)} // New validates K
}

// Config returns the configuration the Sketcher was created with.
func (s *Sketcher) Config() Config { return s.cfg }

// Sketch returns the MinHash signature of text's word shingles.
// Text with fewer than W words produces the empty-set signature (all
// math.MaxUint64); see [Similarity] for how the convenience layer treats
// those.
func (s *Sketcher) Sketch(text string) minhash.Signature {
	return s.hasher.Sketch(shingle.Words(text, s.cfg.W))
}

// SketchBytes is [Sketcher.Sketch] for a byte slice, without copying. The
// slice is not retained or mutated.
func (s *Sketcher) SketchBytes(b []byte) minhash.Signature {
	return s.hasher.Sketch(shingle.WordsBytes(b, s.cfg.W))
}

// SketchInto sketches text into dst, overwriting it — the low-allocation
// path for bulk sketching. Panics if len(dst) != K.
func (s *Sketcher) SketchInto(dst minhash.Signature, text string) {
	s.hasher.SketchInto(dst, shingle.Words(text, s.cfg.W))
}

// SketchIntoBytes is [Sketcher.SketchInto] for a byte slice, without
// copying. The slice is not retained or mutated. Panics if len(dst) != K.
func (s *Sketcher) SketchIntoBytes(dst minhash.Signature, b []byte) {
	s.hasher.SketchInto(dst, shingle.WordsBytes(b, s.cfg.W))
}

// Similarity estimates the Jaccard similarity of the two texts' word
// shingle sets using this Sketcher's configuration, in [0, 1]. If either
// text has fewer than W words it has no shingles, and Similarity returns 0
// — two degenerate texts do not count as identical.
func (s *Sketcher) Similarity(a, b string) float64 {
	sa, sb := s.Sketch(a), s.Sketch(b)
	if isEmptySet(sa) || isEmptySet(sb) {
		return 0
	}
	return minhash.Jaccard(sa, sb)
}

// NewIndex returns an empty LSH candidate index shaped by the
// configuration's Bands and Rows, for signatures produced by this Sketcher.
// Panics if Bands/Rows were left zero.
func (s *Sketcher) NewIndex() *lsh.Index {
	if s.cfg.Bands == 0 {
		panic("semblance: Config.Bands/Rows not set")
	}
	return lsh.NewIndex(s.cfg.Bands, s.cfg.Rows)
}

// Similarity estimates the Jaccard similarity of the two texts' word
// shingle sets with the [Defaults] configuration, in [0, 1]. Standard
// error is about 0.044 at the default K=128. Returns 0 if either text is
// too short to shingle (fewer than 3 words).
func Similarity(a, b string) float64 {
	return defaultSketcher.Similarity(a, b)
}

var defaultSketcher = NewSketcher(Defaults())

// isEmptySet reports whether sig is the empty-set signature (no shingles
// were sketched into it).
func isEmptySet(sig minhash.Signature) bool {
	for _, v := range sig {
		if v != math.MaxUint64 {
			return false
		}
	}
	return true
}
