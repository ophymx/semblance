// Package lsh provides in-memory locality-sensitive-hash indexes over the
// sketches produced by the minhash and simhash packages: a banding index
// for MinHash signatures and a Hamming-ball index for SimHash fingerprints.
//
// Neither index is safe for concurrent use; guard with a lock if you share
// one across goroutines. Both return results in deterministic order.
package lsh

import (
	"math"

	"github.com/ophymx/semblance/internal/hashutil"
	"github.com/ophymx/semblance/minhash"
)

// Index is a MinHash banding index. A signature of length bands*rows is
// split into bands of rows values each; two signatures become candidates if
// they agree exactly on at least one band. For sets with Jaccard similarity
// J the candidate probability is 1-(1-J^rows)^bands, an S-curve centered
// near [Index.Threshold].
//
// Query returns candidates only: false positives are expected (verify with
// [minhash.Jaccard] or exact comparison), and near-threshold pairs can be
// missed. Not safe for concurrent use.
type Index struct {
	bands, rows int
	store       bucketStore
}

// bucketStore is the bucket backend. Unexported so a KV-backed store can be
// added later without API changes; the only implementation is in-memory.
type bucketStore interface {
	add(band int, key uint64, id string)
	get(band int, key uint64) []string
}

type memStore []map[uint64][]string

func (s memStore) add(band int, key uint64, id string) {
	if s[band] == nil {
		s[band] = make(map[uint64][]string)
	}
	s[band][key] = append(s[band][key], id)
}

func (s memStore) get(band int, key uint64) []string { return s[band][key] }

// NewIndex returns an empty banding index for signatures of length
// bands*rows. Panics if bands <= 0 or rows <= 0.
func NewIndex(bands, rows int) *Index {
	if bands <= 0 || rows <= 0 {
		panic("lsh: bands and rows must be positive")
	}
	return &Index{bands: bands, rows: rows, store: make(memStore, bands)}
}

// Threshold returns the approximate Jaccard similarity at which the
// candidate probability crosses 1/2: (1/bands)^(1/rows).
func (ix *Index) Threshold() float64 {
	return math.Pow(1/float64(ix.bands), 1/float64(ix.rows))
}

// Add indexes id under the signature. Ids need not be unique: adding the
// same id again (with the same or a different signature) is allowed, and a
// query matching it still returns it once.
// Panics if len(sig) != bands*rows.
func (ix *Index) Add(id string, sig minhash.Signature) {
	for band, key := range ix.bandKeys(sig) {
		ix.store.add(band, key, id)
	}
}

// Query returns the ids of signatures that agree with sig on at least one
// band, deduplicated, in deterministic order (first matching band, then
// insertion order). Candidates must be verified by the caller; a returned
// id may have low true similarity. Returns nil if there are none.
func (ix *Index) Query(sig minhash.Signature) []string {
	var out []string
	seen := make(map[string]bool)
	for band, key := range ix.bandKeys(sig) {
		for _, id := range ix.store.get(band, key) {
			if !seen[id] {
				seen[id] = true
				out = append(out, id)
			}
		}
	}
	return out
}

// bandKeys yields (band, bucket key) for each band of sig. The key is a
// Mix-fold of the band's row values; key collisions only add false-positive
// candidates, which queries must tolerate anyway.
func (ix *Index) bandKeys(sig minhash.Signature) func(yield func(int, uint64) bool) {
	if len(sig) != ix.bands*ix.rows {
		panic("lsh: signature length does not match bands*rows")
	}
	return func(yield func(int, uint64) bool) {
		for band := 0; band < ix.bands; band++ {
			acc := hashutil.MixInit
			for _, v := range sig[band*ix.rows : (band+1)*ix.rows] {
				acc = hashutil.Mix(acc, v)
			}
			if !yield(band, acc) {
				return
			}
		}
	}
}
