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
| `minhash` | Shingle stream → fixed-size `Signature`; `Jaccard` estimates set similarity with standard error ≈ 1/(2√k); asymmetric `Containment` ("how much of A is in B"), `Cardinality`, and `IntersectionCardinality` come free from the same signatures; mergeable (`Union`) |
| `simhash` | Weighted features → 64-bit `Fingerprint`; Hamming `Distance` tracks cosine similarity |
| `lsh`     | `Index` (MinHash banding: candidates above a similarity threshold), `Forest` (top-k most-similar retrieval, no threshold), and `HammingIndex` (all stored fingerprints within distance ≤ 3, exact) |
| `winnow`  | Position-aware winnowing fingerprints (the MOSS algorithm): locate *where* documents overlap; any shared run of w+k−1 bytes is guaranteed a match |
| `cluster` | Deterministic union-find for grouping verified near-duplicate pairs into clusters, earliest-member-wins representatives |
| `hll`     | HyperLogLog cardinality sketches: distinct-element counts with ~1.04/√2ᵖ error, mergeable and serializable; feed it shingle streams to count distinct words or shingles |
| `topk`    | SpaceSaving frequent-items sketch (generic over item type): the heaviest items of a stream in fixed space with per-item error bounds — flood and burst detection |

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

## Bring your own persistence

There is deliberately no pluggable storage interface: real backends need
their own batching, error handling, and consistency model, which a
synchronous store interface would dictate badly. Instead the library
exposes the frozen primitives and you own the I/O:

- **Signatures and fingerprints serialize** (`MinHasher.MarshalSignature`,
  `Fingerprint.MarshalBinary`) — store them as blobs next to your
  documents.
- **`lsh.BandKeys`** computes the bucket keys an index would use, so an
  LSH index over Redis (`SADD lsh:<band>:<key> id`), SQL, or any KV is
  ~30 lines: write id under each key on add, union the buckets on query,
  verify candidates with `minhash.JaccardMany`. See the `BandKeys`
  example. Key derivation is frozen and safe to persist.
- **In-memory indexes rebuild fast**: `Range` enumerates contents, and
  re-`Add`ing a million stored signatures takes seconds, so a snapshot of
  (id, signature) pairs is usually all the durability an `lsh.Index`
  needs.

## References

- A. Z. Broder. *On the resemblance and containment of documents.*
  Compression and Complexity of Sequences, 1997.
- M. Charikar. *Similarity estimation techniques from rounding algorithms.*
  STOC 2002.
- S. Schleimer, D. Wilkerson, A. Aiken. *Winnowing: local algorithms for
  document fingerprinting.* SIGMOD 2003.
- M. Bawa, T. Condie, P. Ganesan. *LSH Forest: self-tuning indexes for
  similarity search.* WWW 2005.
- G. S. Manku, A. Jain, A. Das Sarma. *Detecting near-duplicates for web
  crawling.* WWW 2007.
- P. Flajolet, É. Fusy, O. Gandouet, F. Meunier. *HyperLogLog: the analysis
  of a near-optimal cardinality estimation algorithm.* AofA 2007.
- A. Metwally, D. Agrawal, A. El Abbadi. *Efficient computation of frequent
  and top-k elements in data streams.* ICDT 2005.
- J. Leskovec, A. Rajaraman, J. Ullman. *Mining of Massive Datasets*, ch. 3
  (shingling, minhashing, LSH banding).

## License

MIT
