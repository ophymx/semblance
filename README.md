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
| `lsh`     | `Index` (banding: unverified candidate ids), `VerifiedIndex` (banding + stored signatures: verified, ranked near-duplicates in one call), `Forest` (top-k most-similar retrieval), and `HammingIndex` (SimHash within distance ≤ 3, exact) |
| `winnow`  | Position-aware winnowing fingerprints (the MOSS algorithm): locate *where* documents overlap; any shared run of w+k−1 bytes is guaranteed a match. `Index` gives corpus-scale fragment provenance — which documents a text overlaps and exactly where — and boilerplate mining: `Cover` returns the byte ranges of a text that recur across ≥ N indexed documents (shared footers, ad blocks, templated headers), ready to trim |
| `cluster` | Deterministic union-find for grouping verified near-duplicate pairs into clusters, earliest-member-wins representatives |
| `hll`     | HyperLogLog cardinality sketches: distinct-element counts with ~1.04/√2ᵖ error, mergeable and serializable; feed it shingle streams to count distinct words or shingles |
| `topk`    | SpaceSaving frequent-items sketch (generic over item type): the heaviest items of a stream in fixed space with per-item error bounds — flood and burst detection |
| `sample`  | Deterministic reservoir sampling: k uniform representatives from a stream of unknown length, reproducible by seed |

Reusable, configurable pipeline:

```go
sk := semblance.NewSketcher(semblance.Defaults())
ix := sk.NewIndex()
ix.Add("doc1", sk.Sketch(doc1))
candidates := ix.Query(sk.Sketch(query)) // verify candidates with minhash.Jaccard
```

## Design

- **Deterministic.** Same input + parameters + seed → same signature, on
  every platform, in every process, and across Go toolchain versions.
  Signatures are made to be stored and compared later, elsewhere. Word
  tokenization uses a Unicode table frozen in the `shingle` package
  (`shingle.UnicodeVersion`), not the toolchain's, so it does not shift on
  a compiler upgrade.
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

## Untrusted input

semblance is **deterministic by design** — the same input always produces
the same sketch, which is what makes stored signatures comparable across
processes and releases. The flip side is that all hashing is unkeyed and
the parameters are frozen and public, so an attacker who can choose the
input can also predict every hash. Keep this in mind when any of these
structures is fed attacker-controlled documents.

- **Seed the collision-sensitive structures.** `lsh.Index` bucket keys
  derive from the MinHash signature, so a **non-zero, secret seed** passed
  to `minhash.New` randomizes them and defeats deliberate bucket flooding.
  The frozen defaults (`Defaults()`, the top-level `Similarity`) use seed
  0 — fine for trusted corpora, predictable for adversaries. There is no
  such lever for `winnow.Index`, `hll`, or `topk`: they key on the raw
  shingle/element hashes (xxhash seed 0, frozen), so an attacker can force
  worst-case collisions regardless of any seed. Treat their memory and
  per-query cost as attacker-influenced and bound your input sizes.

- **Winnowing overlap work is bounded.** `winnow.Overlaps` and
  `Index.Matches` return aligned `Span`s (diagonal runs of shared
  fingerprints collapsed into one region), so a shared passage is one span,
  not one entry per fingerprint. The underlying match set is still
  quadratic on highly repetitive text (a long run of one byte matches at
  nearly every position), so both bound the matches they examine at
  `winnow.MaxResults`; reaching it means "overlap is at least this large."
  `Index.Overlap` returns a bounded per-document summary and is the safer
  default when you only need how much, not where.

- **Deserialization is bounds-checked.** `DecodeSignature`,
  `hll.UnmarshalBinary`, and `simhash.Fingerprint.UnmarshalBinary`
  validate the version, algorithm, and length fields before allocating, so
  a malformed or hostile blob yields an error, never an oversized
  allocation — but they do not authenticate the bytes. A tampered
  signature decodes to a valid-but-wrong sketch; sign or MAC stored blobs
  if their integrity matters.

- **`*Bytes` zero-copy contract.** The `*Bytes` entry points view the
  slice without copying; mutating it before iteration completes yields
  wrong hashes (never a memory-safety fault). Pass a stable slice.

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
