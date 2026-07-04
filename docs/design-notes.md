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
