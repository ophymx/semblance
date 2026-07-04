package lsh_test

import (
	"fmt"

	"github.com/ophymx/semblance/lsh"
	"github.com/ophymx/semblance/minhash"
	"github.com/ophymx/semblance/shingle"
)

// ExampleBandKeys builds an LSH index over external storage — the
// bring-your-own-persistence pattern. The map stands in for any store:
// with Redis it would be SADD/SMEMBERS on the printed keys, with SQL a
// table keyed by (band, key). The library supplies the frozen banding
// math; the store supplies durability, batching, and error handling on
// the caller's terms.
func ExampleBandKeys() {
	const bands, rows = 16, 8
	m := minhash.New(bands*rows, 0)
	sketch := func(text string) minhash.Signature {
		return m.Sketch(shingle.Words(text, 3))
	}

	store := map[string][]string{} // "lsh:<band>:<key>" -> ids

	add := func(id, text string) {
		for band, key := range lsh.BandKeys(nil, sketch(text), bands, rows) {
			k := fmt.Sprintf("lsh:%d:%016x", band, key)
			store[k] = append(store[k], id) // e.g. SADD in Redis
		}
	}
	query := func(text string) (ids []string, sigs []minhash.Signature, q minhash.Signature) {
		q = sketch(text)
		seen := map[string]bool{}
		for band, key := range lsh.BandKeys(nil, q, bands, rows) {
			k := fmt.Sprintf("lsh:%d:%016x", band, key)
			for _, id := range store[k] { // e.g. SMEMBERS in Redis
				if !seen[id] {
					seen[id] = true
					ids = append(ids, id)
					sigs = append(sigs, docSigs[id])
				}
			}
		}
		return ids, sigs, q
	}

	for id, text := range docTexts {
		docSigs[id] = sketch(text)
		add(id, text)
	}

	ids, sigs, q := query("the quick brown fox jumps over the lazy dog every single morning")
	for i, est := range minhash.JaccardMany(nil, q, sigs) {
		if est >= 0.5 {
			fmt.Printf("%s: %.2f\n", ids[i], est)
		}
	}
	// Output:
	// a: 0.70
}

// The caller's document and signature stores (a database column in
// practice; signatures round-trip via MinHasher.MarshalSignature).
var (
	docTexts = map[string]string{
		"a": "the quick brown fox jumps over the lazy dog every morning",
		"b": "an entirely unrelated sentence about locality sensitive hashing",
	}
	docSigs = map[string]minhash.Signature{}
)
