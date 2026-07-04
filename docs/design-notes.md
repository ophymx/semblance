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

## Deviations from spec

None yet.
