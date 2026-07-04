# semblance

[![Go Reference](https://pkg.go.dev/badge/github.com/ophymx/semblance.svg)](https://pkg.go.dev/github.com/ophymx/semblance)

Deterministic, heuristic, fast text-similarity sketching for Go: shingling,
MinHash, SimHash, and LSH indexing under one coherent API.

The name is a nod to Andrei Broder's *"On the resemblance and containment of
documents"* (1997), which introduced shingle-based resemblance and minwise
hashing.

```go
import "github.com/ophymx/semblance"

sim := semblance.Similarity(
    "the quick brown fox jumps over the lazy dog",
    "the quick brown fox leaps over the lazy dog",
) // ≈ 0.42 — estimated Jaccard similarity of the texts' word-shingle sets
```

## Install

```
go get github.com/ophymx/semblance
```

Requires Go 1.25+. One dependency: `github.com/cespare/xxhash/v2`.

## What it does

Every layer is usable on its own; the root package wires them together with
frozen defaults (word shingles of width 3, 128-value signatures, seed 0,
16×8 LSH banding ≈ 0.71 candidate threshold).

| Package   | What it gives you |
|-----------|-------------------|
| `shingle` | Text → stream of shingle hashes (`iter.Seq[uint64]`): character k-grams (`Char`, `CharRunes`) or word w-grams (`Words`), hashed incrementally with zero per-shingle allocations |
| `minhash` | Shingle stream → fixed-size `Signature`; `Jaccard` estimates set similarity with standard error ≈ 1/(2√k); signatures are mergeable (`Union`) |
| `simhash` | Weighted features → 64-bit `Fingerprint`; Hamming `Distance` tracks cosine similarity |
| `lsh`     | `Index` (MinHash banding: sub-linear candidate retrieval above a similarity threshold) and `HammingIndex` (all stored fingerprints within distance ≤ 3, exact) |

Reusable, configurable pipeline:

```go
sk := semblance.NewSketcher(semblance.Defaults())
ix := sk.NewIndex()
ix.Add("doc1", sk.Sketch(doc1))
candidates := ix.Query(sk.Sketch(query)) // verify candidates with minhash.Jaccard
```

## Design

- **Deterministic.** Same input + parameters + seed → same signature, on
  every platform, in every process. Signatures are made to be stored and
  compared later, elsewhere. (Word tokenization uses the Go toolchain's
  Unicode tables, so it is stable per Unicode version; ASCII is
  unconditionally stable — see the `shingle` docs.)
- **Fast.** Hot paths do zero allocations per shingle and O(1) small
  allocations per document, asserted in tests.
- **Heuristic, not exact.** Everything is an estimator with documented
  error bounds; LSH returns candidates for the caller to verify.
- **Storable.** Signatures and fingerprints have a stable, versioned binary
  encoding (`minhash.MinHasher.MarshalSignature`,
  `simhash.Fingerprint.MarshalBinary`); stored signatures are
  self-describing and remain comparable across releases within a major
  version.
- **Not** an NLP toolkit (no stemming, stopwords, language detection), not
  a search engine, nothing non-deterministic.

## References

- A. Z. Broder. *On the resemblance and containment of documents.*
  Compression and Complexity of Sequences, 1997.
- M. Charikar. *Similarity estimation techniques from rounding algorithms.*
  STOC 2002.
- G. S. Manku, A. Jain, A. Das Sarma. *Detecting near-duplicates for web
  crawling.* WWW 2007.
- J. Leskovec, A. Rajaraman, J. Ullman. *Mining of Massive Datasets*, ch. 3
  (shingling, minhashing, LSH banding).

## License

MIT
