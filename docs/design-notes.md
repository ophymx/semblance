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
| 3  | HyperLogLog, dense 8-bit registers (post-v0) |

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
values. Key collisions merely add false-positive candidates, which callers
must verify anyway. As of the post-v0 `lsh.BandKeys` export, key
derivation is **frozen and persistable** (pinned by `TestBandKeysGolden`):
changing it breaks externally stored buckets and is a major-version event.

**No pluggable store interface (post-v0 decision).** Considered and
declined: a synchronous, error-less store interface is correct for the
in-memory map and wrong for every real backend (error propagation would
infect Add/Query signatures for all callers; per-band round-trips are
catastrophic without batching, which needs a different interface shape).
Instead, BYO-persistence gets primitives: serialized signatures,
`BandKeys` for external bucket layouts (equivalence with `lsh.Index`
proven in `TestExternalIndexEquivalence`), and `Range` for rebuild. The
unexported bucketStore interface remains reserved for a future
purpose-designed disk index, should one ever be warranted.

**Query result order.** Both indexes return deduplicated ids in
(first matching band/block, insertion) order — never map-iteration order,
per determinism principle. Buckets are append-only slices.

**Hamming index.** Block-permutation variant of Manku et al.: maxDist+1
near-equal blocks (d=3 → 4×16 bits; d=2 → 22/21/21; d=1 → 2×32), pigeonhole
guarantees a full-block match within distance d, exact-match table per
block, hits verified against the stored fingerprint. So Query is exact
(no false positives or negatives), unlike the banding index's candidates.
Cost: d+1 table entries per Add.

**Containment & Cardinality (post-v0).** Estimated from signatures alone:
Cardinality via k/Σ(vᵢ/2⁶⁴) − 1 (datasketch's estimator), Containment by
combining it with the Jaccard estimate. Known limitation, documented and
tested against its own error model: absolute error grows ~sqrt(R·c/k)
with the size ratio R = (|A|+|B|)/|A|, so small-in-large containment is a
coarse signal at default k — the precise fix is LSH-Ensemble-style
sketching, deferred until needed. Estimator formulas may improve within a
major version (they derive from stored signatures; changing them does not
break stored data).

**Winnowing (post-v0).** `winnow` implements standard winnowing (rightmost
minimum per window), NOT the paper's "robust" tie-break variant: standard
keeps the strict guarantee (any shared run of w+k−1 bytes matches) that
robust trades for fewer fingerprints in repetitive text. Selection scheme
is frozen (golden test); implementation verified against a naive
every-window oracle including tie-heavy inputs, and the guarantee is
fuzz-tested.

**LSH Forest (post-v0).** Top-k retrieval per Bawa et al., datasketch-
style: trees disjoint signature slices, prefix levels are whole 64-bit
minhash values, queries descend from the deepest shared prefix. Lazy
sort-on-query (dirty flag) instead of datasketch's explicit index() call,
so bulk loads are one sort. Candidate order is deterministic — descending
maximum prefix depth, then tree/key/insertion order — and verified against
a brute-force depth oracle. Results are similarity-proxy-ordered
candidates; callers re-rank with JaccardMany.

**Index lifecycle (post-v0).** Both indexes support Remove/Len/Range.
`Index` tracks per-id bucket keys (bands×8 bytes per Add) so Remove can
find its entries without storing signatures; consequently `Index.Range`
yields ids only, and rebuild/migration requires the caller's signature
store (documented). `HammingIndex` already stores fingerprints, so its
Range yields (id, fingerprint) pairs and suffices to rebuild. Range
enumeration order is unspecified (map order) — an accepted, documented
exception to the deterministic-order rule, which continues to hold for
Query results; enumeration is not a hot path and produces no stored
artifacts.

**Bytes inputs (post-v0).** The `*Bytes` variants bridge to the string
implementations via `unsafe.String` — zero-copy, nothing retained or
mutated; callers must not mutate the slice until iteration completes.
Single implementation means golden behavior is shared by construction;
equivalence is fuzz-verified.

**Streaming (post-v0).** `shingle.WordScanner` and `semblance.Stream`
sketch chunked input with chunking-independence guaranteed: chunks are cut
into segments at *definite* token boundaries — ASCII non-alphanumeric
bytes, which can never occur inside a token or a multi-byte UTF-8 rune —
and the boundary-free tail is carried to the next Write. Each complete
segment reuses the ordinary tokenHashes scan, so partial runes and
tokens spanning chunks need no special handling; equivalence with Words
over arbitrary splits is fuzz-verified. The carry buffer grows with the
longest boundary-free span (documented). Steady-state Write is
zero-alloc.

**Root package.** `Similarity` detects the degenerate case via the
empty-set signature (all MaxUint64) rather than counting tokens — no second
pass over the text. A non-empty document producing that signature would
require a permuted hash of MaxUint64 in all k slots (probability ~2^-64k).
`Config.Bands/Rows` may be left zero when no index is needed; validation of
Bands×Rows==K happens in NewSketcher when set, and NewIndex panics if unset.

## Measured constants (go1.26; post-kernel-prototype)

Sketch pipelines run at a small exact alloc constant per document,
independent of document size, asserted in `TestSketchIntoAllocs` /
`TestSketchTextAllocs` (a compiler change in inlining may legitimately move
the constants — update the tests after verifying allocs are still O(1) in
document size):

- minhash `SketchInto` + `Char`/`Words`: 3 allocs (two pipeline closures +
  the 2 KB block buffer, which escapes because the range-over-func body
  captures it).
- `simhash.SketchText`: 4 allocs (three pipeline closures + the weight-1
  block buffer).

Throughput on the amd64 dev box (one core, k=128): char shingles k=8
≈ 2.8 MB/s (xxhash per window is O(n·k) by design); word shingles w=3
≈ 12 MB/s; `SketchText` w=3 ≈ 18 MB/s. On an Apple M1 Pro: words
≈ 82 MB/s, `SketchText` ≈ 169 MB/s.

Kernel structure (post-v0 prototype, see docs/simd-analysis.md for the
full measurements): minhash sketches via a batched element-major
`sketchBlock` with AVX2 and NEON variants, plus an AVX2 `eqCount` behind
`Jaccard`; simhash's weight-1 path reduces to positional popcount
(counts[i] = 2·s[i] − n) computed by a carry-save adder over 8 bit-planes,
with AVX2 and NEON kernels. AVX2 kernels dispatch at runtime via
`internal/cpuinfo` (hand-rolled CPUID/XGETBV, zero dependencies), so
default builds get them on capable CPUs; NEON is unconditional on arm64.
All kernels are verified against naive oracles, both dispatch branches are
tested (`TestDispatchBothPaths`), and everything is bit-identical to the
scalar path. None of this affects frozen semantics: kernels change speed,
never signatures.

## Deviations from spec

None yet.
