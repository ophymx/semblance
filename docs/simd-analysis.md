# SIMD acceleration analysis

Where SIMD can accelerate the current algorithms on x86_64 (AVX2 baseline;
nothing older) and arm64 (NEON). Written against the v0 code (M1–M4);
profile numbers from `go1.26` on the 100 KB benchmark documents. This is
post-v0 backlog planning — no assembly exists yet, per spec.

## Where the time actually is (measured)

| Pipeline | Throughput | Dominant cost | Share |
|---|---|---|---|
| minhash Words (k=128, w=3) | 10.7 MB/s | `SketchInto` permutation loop | 69% flat; tokenize+xxhash ≈ 20% |
| minhash Char (k=128, k-gram 8) | 2.1 MB/s | same loop (~128 perms per byte) | ~85%; xxhash itself only ~5% |
| simhash SketchText (w=3) | 6.7 MB/s | `Sketch`'s 64-bit counter loop | 69% flat |

The analysis reduces to three loops plus one structural blocker.

## Target 1 — MinHash permutation loop (highest value, ISA-asymmetric)

The loop: for each input hash `x`, compute `a[i]*x + b[i]` and unsigned-min
into `dst[i]`, k=128 times. `x` is loop-invariant (broadcast once);
`a`/`b`/`dst` are contiguous. Ideal SIMD shape.

**AVX2.** No 64-bit lane multiply (`vpmullq` is AVX-512DQ) and no unsigned
64-bit min (`vpminuq` is AVX-512), but both are synthesizable:

- 64-bit mullo from 3× `vpmuludq` + 2 shifts + 2 adds per 4 lanes
  (lo·lo + ((lo·hi + hi·lo) << 32)).
- Unsigned min via the sign-bias trick: keep `dst` sign-flipped for the
  whole document, `vpcmpgtq` + `vpblendvb` per step, flip once at the end.

Roughly 12 µops per 4 lanes vs ~4–5 per lane scalar. Expect **2–3× on the
loop** → by Amdahl ~1.8–2.3× end-to-end for Words, ~2.3× for Char.

**NEON.** The weak spot: NEON has **no 64-bit lane multiply at all**.
Synthesizing from `umull`/`umlal` costs ~6–8 instructions per 2 lanes,
while big ARM cores (Apple M-series, Neoverse) issue 2–4 scalar 64-bit
`mul`/`madd` per cycle with excellent ILP. Expect **parity to 1.3×** —
likely not worth the assembly. On arm64, prefer scalar unroll-and-interleave
and spend vector effort on targets 2–4. (SVE2 has proper 64-bit ops; out of
scope here.)

Algorithmic alternative: one-permutation hashing (post-v0 backlog) removes
the O(n·k) factor entirely — a bigger win than any vectorization of k-perm,
but it produces incompatible signatures (hence the algo-id byte in the wire
format). SIMD here accelerates the frozen v0 format; OPH is a separate
product decision, and they don't exclude each other.

## Target 2 — SimHash counter loop (best cross-ISA win)

Currently 64 test-bit/add-sub iterations per feature. This is the
**positional popcount** problem (count of set bits per position across a
word stream), with well-known fast kernels:

- **AVX2:** expand each hash's bits into 0/-1 masks over four registers of
  16×int16 counters and add the weight; for the weight-1 `SketchText` path,
  pospopcnt-style (Harley–Seal / `vpshufb`) blocks with periodic flush to
  int32 to avoid int16 overflow. Expect **4–8× on the loop**.
- **NEON:** genuinely good here — byte ops are NEON's strength (`cmtst`
  against a bit-mask vector, accumulate int8/int16, widen periodically via
  `uadalp`). Expect **3–5×**.

Specialize weight==1 (the common path); the general weighted path keeps the
current loop. Free scalar note: the `if h>>i&1 == 1` branch can be made
branchless — check whether the compiler already emits cmov before doing
anything fancier.

## Target 3 — batch Jaccard verification (easy, situational)

O(k) equality counting: `vpcmpeqq` + movemask + popcount on AVX2
(**4–6×**), `cmeq` + horizontal add on NEON (**2–3×**). Irrelevant for one
comparison, but LSH workloads verify every candidate with `Jaccard` — hot
in a dedup pass over millions of pairs. Cheapest kernel of the four;
`Union` and the root package's `isEmptySet` vectorize identically.

## Target 4 — tokenizer scan (sequence after target 1)

simdjson-style classification: 32 bytes per op on AVX2 (range compares for
alnum + case-fold OR 0x20), movemask + tzcnt for token boundaries; NEON
equivalent via `shrn`-based mask narrowing. Today tokenization is only
~20% of the Words pipeline (end-to-end cap ~1.2×), but once target 1 lands
its share roughly doubles — do it then.

## Non-targets

- **xxhash of char windows:** only ~5% of the char pipeline; lane-parallel
  xxhash hits the same AVX2 64-bit-multiply pain for little payoff.
- **`simhash.Distance`:** already one xor + popcount instruction. Batch
  nibble-LUT popcount only if a profile ever shows huge Hamming candidate
  sets.
- **`lsh` bandKeys Mix fold:** O(k) per Add/Query, serial dependency chain,
  not hot.

## Cross-cutting constraints

1. **The `iter.Seq` boundary is the real prerequisite.** Hashes arrive one
   per yield; every SIMD kernel needs contiguous batches. Restructure:
   buffer ~256 hashes into a stack array inside `SketchInto`/`Sketch`,
   invoke a kernel per block, keep the current loop as the pure-Go fallback
   kernel. API and signatures unchanged; batching alone should claw back a
   few percent of yield overhead before any assembly exists. Start here
   regardless of ISA.
2. **Vehicle:** Go has no intrinsics and the spec bans cgo → Go assembly,
   ideally generated with `avo` on x86. Dispatch: NEON is baseline on
   arm64 (no detection needed); on x86_64 either runtime dispatch via
   `golang.org/x/sys/cpu` (one light, standard dep) or `GOAMD64=v3` build
   tags (zero deps, opt-in at build time). Minimal-deps principle slightly
   favors build tags; runtime dispatch is friendlier. Decide at
   implementation time.
3. **Determinism is safe.** Every candidate kernel is exact integer
   arithmetic with commutative, order-insensitive reductions (min, add) —
   bit-identical to scalar by construction; the golden tests enforce it.
   No floats, no FMA reassociation risk.
4. **AVX-512 aside:** `vpmullq` + `vpminuq` make target 1 native and
   roughly double its ceiling — a cheap incremental variant once the AVX2
   kernel and dispatch scaffolding exist.

## Priority order

| # | Kernel | AVX2 | NEON | Effort | Note |
|---|--------|------|------|--------|------|
| 0 | Batch buffering in Sketch paths | — | — | small | Prerequisite; pure Go |
| 1 | SimHash pospopcnt (weight-1) | 4–8× | 3–5× | medium | Best cross-ISA ROI |
| 2 | MinHash perm loop | 2–3× | ~1× | medium-high | x86-only win; weigh against OPH |
| 3 | Batch Jaccard/Union | 4–6× | 2–3× | small | Matters in verify-heavy dedup |
| 4 | Tokenizer classify | 2–4× | 2–4× | medium | Only after #2 shifts the profile |

Key asymmetry worth remembering: **NEON is nearly useless for the MinHash
loop** (no 64-bit multiply) but better-suited than expected for SimHash —
the two sketch types want opposite per-ISA investment.

## Prototype results (targets 0 and 1)

Implemented post-M4. Linux box is amd64 with AVX2 (no AVX-512); arm64
numbers from an Apple M1 Pro (`jeffreys-mac-mini`), full test suite
cross-compiled and run there — golden signatures match across platforms.

**Target 0 (batching).** `SketchInto` buffers 256 hashes and calls
`sketchBlock`; `simhash.Sketch` buffers weight-1 features (255, the CSA
cap) and processes other weights directly. Words 100 KB: 9.5→8.7 ms
(+9%), Char: ~42→36.7 ms (+13%) from yield-overhead amortization alone.
Cost: the block buffer escapes (range-over-func body captures it), +1
alloc/doc — ratified in the alloc tests (minhash 3, simhash 4). Negative
finding: a permutation-major scalar kernel lost 10–40% to element-major
(tried single-accumulator and 4-accumulator variants); the batching still
gives a future asm kernel its contiguous block, but the scalar fallback
stays element-major.

**Target 1 (SimHash positional popcount).** The reformulation is the
whole story: for n unit-weight features, counts[i] = 2·s[i] − n where s is
the positional popcount, computed with a carry-save adder over 8
bit-planes (`pospopcnt.go`). Kernel throughput on a 252-word block:

| Kernel | amd64 dev box | M1 Pro |
|---|---|---|
| naive per-bit loop (old code) | 10 MB/s | 61 MB/s |
| scalar CSA (pure Go) | 449 MB/s (~44×) | 2379 MB/s (~39×) |
| AVX2 (`GOAMD64=v3`) | 709 MB/s (1.6× CSA) | — |
| NEON | — | 2751 MB/s (1.16× CSA) |

End-to-end `SketchText` 100 KB: 15.3→5.6 ms (**2.7×**) on the dev box,
0.60 ms on the M1. The headline finding: **the predicted 4–8× SIMD win was
mostly capturable in pure Go** — the CSA reformulation delivers ~40× at
the kernel level, and the vector kernels add only 1.2–1.7× on top because
the pipeline is now tokenizer-bound (revising the priority table: target 4
is the next bottleneck for SimHash; target 2 still dominates MinHash).
The AVX2 kernel is gated on the `amd64.v3` build tag (zero new
dependencies; default `GOAMD64=v1` builds use the CSA kernel), NEON is
unconditional on arm64. Both kernels are verified against a naive oracle
across tail lengths and saturation patterns (`TestPospopcnt`), and produce
bit-identical fingerprints (golden tests).

## Prototype results, round 2 (targets 2, 3, 4)

**Target 2 (MinHash permutation loop, AVX2).** `sketchBlockAVX2` runs four
permutations per YMM register, permutation-block-major (a/b/minima in
registers for the whole 256-element block), with the synthesized 64-bit
multiply (3× vpmuludq) and sign-biased unsigned min, two accumulators to
halve the compare/blend chain. Kernel: 87.8→36.9 µs per block at k=128
(**2.4×** — the 2–3× prediction held here). End-to-end 100 KB: Char
34.8→19.7 ms (**1.77×**), Words 8.4→6.5 ms (1.28×, tokenizer-bound).

**NEON MinHash kernel — a correction.** Round 2 initially dismissed NEON
by comparing the M1's scalar kernel (13.4 µs/block) against this box's
AVX2 (36.9 µs) — a cross-machine comparison that says nothing about NEON.
Measured properly (same M1, NEON vs scalar): kernel 13.4→10.8 µs
(**1.24×**), end-to-end Char 6.2→5.1 ms (**1.21×**), Words 1.05→0.90 ms
(1.17×). That lands at the top of the original "parity to 1.3×"
prediction and is worth its ~90 lines: `sketchBlockNEON` mirrors the AVX2
structure, synthesizing the 64-bit multiply from umull/umlal cross terms
and the unsigned min from cmhi+bit. XTN/UMULL/UMLAL/CMHI are outside the
Go assembler's arm64 vocabulary and are emitted as hand-encoded WORD
directives, verified by the kernel oracle test on hardware.
Methodology lesson recorded: **ISA verdicts require same-machine
baselines.**

**Target 4 (tokenizer, pure Go — no SIMD needed yet).** All-lowercase-ASCII
tokens (the common case) now hash directly from the string, zero-copy; only
tokens with uppercase or non-ASCII runes take the fold buffer. Identical
hashes by construction (golden + fuzz verified). `SketchText` 5.7→4.1 ms
(**1.38×**); on the M1, 605→405 µs (**1.49×**). The CSA lesson repeated:
measure the scalar rewrite before reaching for vpcmpgtb.

**Target 3 (Jaccard equality count, AVX2).** `eqCountAVX2`: vpcmpeqq with
vector-accumulated mask subtraction. `Jaccard` at k=128: 282→100 ns
(**2.8×**). M1 scalar: 53 ns (again faster than our vectorized amd64).

**Cumulative on the dev box, 100 KB docs, since the pre-prototype
baseline** (GOAMD64=v3 builds; v1 builds keep the pure-Go wins only):

| Pipeline | baseline | now (v3) | speedup |
|---|---|---|---|
| simhash SketchText | 15.3 ms | 4.1 ms | **3.8×** |
| minhash Words | 9.5 ms | 5.2 ms | **1.8×** |
| minhash Char | ~42 ms | 20.2 ms | **~2.1×** |
| Jaccard (k=128) | 282 ns | 100 ns | **2.8×** |

Standing conclusions: (1) algorithmic reformulation and scalar fast paths
beat assembly twice (CSA, tokenizer) — always prototype pure Go first;
(2) vector kernels pay where 64-bit arithmetic is unavoidable and dominant
(the MinHash loop, equality counting): 2.4× on AVX2, 1.24× on NEON;
(3) ISA verdicts require same-machine baselines — the first "NEON not
worth it" call compared across machines and was wrong; (4) all kernels
remain bit-identical to scalar, enforced by oracle tests and cross-platform
golden runs on the M1.
