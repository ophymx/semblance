# design notes

Decisions that are frozen (change = stored signatures break = major version),
plus deviations from the original design and why. See the godoc for
user-facing documentation; this file is for maintainers.

## Frozen primitives (M1)

**Element hash.** xxhash64 (`github.com/cespare/xxhash/v2`) of the shingle
bytes: the raw window for char shingles, the lowercased UTF-8 token bytes for
word tokens.

**Word-shingle mixing.** Token hashes t1..tw are folded left-to-right:

```
acc = MixInit                      // 0xC2B2AE3D27D4EB4F
for each ti:  acc = rotl64(acc, 31) ^ ti;  acc *= 0x9E3779B97F4A7C15
```

The rotation makes the fold order-sensitive ("a b c" ≠ "c b a"). Pinned by
`TestMixGolden` and the golden signatures.

**Tokenization.** Maximal runs of Unicode letters/numbers (`unicode.IsLetter
|| unicode.IsNumber`, ASCII fast path), lowercased with simple
`unicode.ToLower` (not full case folding). Caveat: stdlib unicode tables
track the Go toolchain's Unicode version, so word-shingle determinism is
per-Unicode-version; ASCII is unconditionally stable. Documented in the
shingle godoc; we deliberately do not vendor frozen tables.

**MinHash permutations.** k functions `a*x + b (mod 2^64)` with odd `a`
(each a bijection of the 64-bit space — no shift, which would break the
bijection). Parameters drawn pairwise (a then b) from a SplitMix64 stream
seeded with the user seed. Pinned by `TestSplitMix64Golden` and the golden
signatures in `minhash/testdata/golden.json` (regenerate only deliberately,
via `go test ./minhash -run TestGoldenSignatures -update`).

**Empty-set signature.** Zero input hashes leave every slot at MaxUint64;
`Jaccard(empty, empty) == 1` by construction.

## Frozen primitives (M2)

**SimHash bit rule.** Bit i of the fingerprint is 1 iff the weighted sum of
feature-hash bits i is strictly positive — ties round to 0. Features are the
same word shingles as minhash uses (`SketchText`), or arbitrary
(hash, weight) pairs via `Sketch`; weight n is exactly equivalent to n
repetitions at weight 1, zero weights are no-ops, negative weights subtract.
Empty input → zero fingerprint (so two empty inputs compare as identical,
mirroring minhash's empty-set signature). Pinned by
`simhash/testdata/golden.json` (regenerate via
`go test ./simhash -run TestGoldenFingerprints -update`).

## Frozen primitives (M4) — serialization

Wire formats (both versioned, little-endian, pinned by hand-written golden
bytes in `TestMarshalGoldenBytes`/`TestFingerprintGoldenBytes`):

```
signature:   [version:1][algo:1][k:2 LE][seed:8 LE][values: k × 8 LE]
fingerprint: [version:1][algo:1][fp:8 LE]
```

**Algorithm-id registry** (shared byte-space across the module, so
incompatible sketch families can never silently mix in storage):

| id | family |
|----|--------|
| 1  | MinHash, k independent multiply-add permutations (v0) |
| 2  | SimHash, 64-bit |

Reserve new ids for post-v0 families (one-permutation hashing, b-bit
compaction) instead of reusing these.

The seed is stored raw (not digested): signatures are self-describing, and
`minhash.DecodeSignature` + `New(len(sig), seed)` reconstructs a compatible
hasher from stored bytes alone. Seeds are not secrets — nothing here is
adversary-resistant regardless.

**Deviation from spec:** the spec put `MarshalBinary`/`UnmarshalBinary` on
`Signature` itself, but `Signature` is a bare `[]uint64` that does not know
its seed, so a self-describing encode is impossible from the value alone.
Marshal/unmarshal therefore live on `MinHasher` (`MarshalSignature`,
`UnmarshalSignature` — which also verifies k/seed match, the check in-memory
comparison can't do) plus package-level `DecodeSignature` for the
hasher-less path. `Fingerprint` carries no parameters, so it implements the
standard `encoding.BinaryMarshaler`/`BinaryAppender`/`BinaryUnmarshaler`
directly.

Consequence of the 16-bit k field: `minhash.New` now rejects k > 65535.

## M3 decisions

**Banding bucket keys.** A band's bucket key is the Mix-fold of its row
values. Keys live only in the in-memory store, so this is not
signature-frozen — but if a KV-backed store lands (post-v0), persisted
buckets freeze it; revisit then. Key collisions merely add false-positive
candidates, which callers must verify anyway.

**Query result order.** Both indexes return deduplicated ids in
(first matching band/block, insertion) order — never map-iteration order,
per determinism principle. Buckets are append-only slices.

**Hamming index.** Block-permutation variant of Manku et al.: maxDist+1
near-equal blocks (d=3 → 4×16 bits; d=2 → 22/21/21; d=1 → 2×32), pigeonhole
guarantees a full-block match within distance d, exact-match table per
block, hits verified against the stored fingerprint. So Query is exact
(no false positives or negatives), unlike the banding index's candidates.
Cost: d+1 table entries per Add.

**Root package.** `Similarity` detects the degenerate case via the
empty-set signature (all MaxUint64) rather than counting tokens — no second
pass over the text. A non-empty document producing that signature would
require a permuted hash of MaxUint64 in all k slots (probability ~2^-64k).
`Config.Bands/Rows` may be left zero when no index is needed; validation of
Bands×Rows==K happens in NewSketcher when set, and NewIndex panics if unset.

## Measured constants (M1, go1.26)

Sketch pipelines (`SketchInto` + `Char`/`Words` iterator) run at exactly
2 allocs per document, independent of document size: the shingler's iterator
closure and the range-over-func yield closure inside `SketchInto`. Ring
buffers stay stack-allocated. Asserted exactly in `TestSketchIntoAllocs`; a
compiler change in inlining may legitimately move the constant — update the
test after verifying the allocs are still O(1) in document size.

Throughput (one core, k=128): char shingles k=8 ≈ 2.4 MB/s (xxhash per
window is O(n·k) by design — acceptable for v0, revisit only with evidence);
word shingles w=3 ≈ 10 MB/s.

`simhash.SketchText` runs at exactly 3 allocs per document independent of
size (Words iterator closure + weight-1 adapter closure + Sketch's yield
closure), asserted in `TestSketchTextAllocs`; ≈ 7 MB/s at w=3 with the
naive 64-iteration inner loop — an unrolled/bit-sliced version is a post-v0
optimization behind the same API.

## Deviations from spec

None yet.
