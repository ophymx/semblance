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
numbers from an Apple M1 Pro, full test suite
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

## Round 3: multi-threaded scaling (core saturation)

All rounds 0–2 numbers were single-threaded. `*Parallel` benchmark
variants (shared read-only inputs, per-goroutine outputs, `b.RunParallel`)
measure aggregate throughput under `-cpu` saturation. Topologies: the
amd64 dev box behaves as 4 physical cores + SMT (near-linear to `-cpu 4`,
+10–15% more at 8); the M1 Pro is 8P+2E (near-linear to 8, ~+10% from
E-cores).

Aggregate MB/s (kernel benches on one 256/252-word block; pipelines on
100 KB docs):

| Benchmark | 1 thread | saturated | scaling |
|---|---|---|---|
| amd64 v3 sketchBlock AVX2 | 55 | 178 (@8) | 3.3× |
| amd64 v3 sketchBlock scalar | 32 | 116 (@8) | 3.6× |
| amd64 v3 pospopcnt AVX2 | 826 | 2644 (@8) | 3.2× |
| amd64 v3 pospopcnt scalar | 363 | 1798 (@8) | 4.9× |
| amd64 v3 minhash Words pipeline | 21 | 78 (@8) | 3.7× |
| amd64 SketchText pipeline (v1≈v3) | 30 | 115 (@8) | 3.9× |
| M1 sketchBlock NEON | 190 | 1288 (@10) | 6.8× |
| M1 sketchBlock scalar | 154 | 1114 (@10) | 7.2× |
| M1 pospopcnt NEON | 3119 | 22165 (@10) | 7.1× |
| M1 pospopcnt scalar | 2288 | 17238 (@10) | 7.5× |
| M1 minhash Words pipeline | 114 | 782 (@10) | 6.8× |
| M1 SketchText pipeline | 249 | 1797 (@10) | 7.2× |

Findings:

1. **SIMD advantages survive saturation but compress, and more under SMT.**
   On the amd64 box, sibling hyperthreads share vector ports, so scalar
   gains more from SMT than vector code: the AVX2 pospopcnt edge falls
   from 2.3× (single) to 1.5× (saturated); sketchBlock from 1.7× to 1.5×.
   On the M1 (no SMT) the NEON edge only drifts: 1.24×→1.16× sketchBlock,
   1.36×→1.29× pospopcnt.
2. **Nothing became memory-bandwidth-bound.** Kernels are L1-resident and
   pipeline inputs are shared read-only; scaling is limited by physical
   core count (and power), not DRAM. Verdicts from single-threaded
   benchmarks stand — SIMD never inverts to a loss under load here.
3. **The M1 scales almost ideally** (~7× on 8 P-cores, E-cores adding
   ~10%): aggregate SimHash sketching hits 1.8 GB/s, the full MinHash
   words pipeline 780 MB/s.
4. Caveat: with every core busy, per-core dynamic clocks drop on both
   machines; single-thread × core-count over-predicts by ~10–20%. Size
   fleets from the saturated numbers, not the single-thread ones.

## The minio multi-buffer/server pattern, considered

`minio/md5-simd` (and `minio/sha256-simd`'s AVX-512 path) run N
*independent* hash streams in the N lanes of wide vector registers, with a
server goroutine that collects blocks from concurrent `hash.Hash` clients
and transposes them into lane-major buffers. That design is a workaround
for **intra-stream serialism**: MD5/SHA block N depends on block N−1, so
one stream cannot be vectorized at all (and MD5 has no hardware
instructions to fall back on). The costs are batching latency, per-block
channel synchronization, and lane stalls when fewer than N streams are
active. (sha256-simd's plain-AVX2 path is single-stream, and SHA-NI has
largely superseded it — the server pattern is really md5-simd's.)

semblance does not need it: our hot loops are data-parallel *within one
document* (128 independent permutations; 64 independent bit-planes), so
the kernels fill their lanes from a single stream, and round 3 shows
goroutine-per-document saturates all cores with zero coordination. A
sketch server would add md5-simd's synchronization for a lane-occupancy
problem we don't have.

Two scaled-down descendants of the idea do apply, both serverless because
one document supplies unlimited independent work items:

- **Lane-parallel xxhash** over 4 shingle windows/tokens per AVX2 op —
  multi-buffer with a free batch supply. Ceiling ~2× on the hashing share
  (64-bit lanes, synthesized multiplies — half minio's 32-bit lane
  economics), which is now a minority of both pipelines.
- **One-vs-many Jaccard** with candidate signatures stored slot-major
  (structure-of-arrays): each vpcmpeqq lane is a *candidate*, the direct
  analog of minio's lane-is-a-stream. `minhash.JaccardMany` now exists as
  the API seam (AoS loop over the dispatched eqCount, ~115 ns/candidate at
  k=128, zero allocs with a reused dst; bit-identical to Jaccard); the
  slot-major kernel and a signature-storing QueryVerified index remain
  gated on evidence of a verification-bound workload.

## Round 4: runtime AVX2 dispatch (reach)

The `amd64.v3` build-tag gating meant default `go build` binaries (v1)
never executed any AVX2 kernel — the speedups existed only for the ~nobody
who sets GOAMD64=v3. Replaced with runtime dispatch: `internal/cpuinfo`
detects AVX2 with a hand-rolled CPUID + OSXSAVE/XGETBV check (~40 lines,
zero new dependencies, honoring the spec's minimal-deps principle over
x/sys/cpu), and each kernel package gates on a `useAVX2` package var —
a var, not a const, so `TestDispatchBothPaths` flips it to cover the
generic fallback branch even on AVX2 CI machines. The branch cost is
amortized over a block per call: not measurable.

Default-build benchmarks now match the old v3 numbers (Jaccard 92 ns,
Words 100 KB ≈ 5.6 ms). NEON needed no equivalent — it is baseline on
arm64 and always shipped by default. GOAMD64=v3 builds remain valid but
no longer select different source files; the dedicated CI step is gone.

## Round 5: fused sketch path (the iterator tax, measured honestly)

Profiles showed ~45% of SketchText flat time in the word-shingle closure
chain, suggesting a large win from fusing tokenize→shingle→sketch.
Implemented as `shingle.WordsBlocks` (scan + fold + block delivery, one
flush callback per block instead of per-shingle yields) feeding new public
`minhash.Reset`/`Update` primitives; `simhash.SketchText`,
`SketchTextBytes`, and the root `Sketcher` now use it. The composable
`iter.Seq` API is unchanged, and golden tests pin that outputs are
identical.

Result — smaller than the profile suggested: SketchText **1.15×** on both
amd64 and the M1 (the 45% flat entry was mostly *real fold work* that
fusion keeps, not call overhead), Sketcher minhash path ~1.05×. The
solid win is allocations: SketchText 4→1, Sketcher.SketchInto 3→1 per
document (the block lives inside the fused scan; only one closure
escape remains). Lesson: flat profile percentages attribute *work located
in* a closure, not *overhead caused by* the closure — the hand-off tax
was only the 2–3 indirect calls per token, worth ~15% with two extra
layers (simhash) and ~5% with one (minhash). Reset/Update also give
callers chunked/incremental sketching, a step toward the streaming
use case.

## Round 6: AVX-512 (the aside, cashed in)

Round 4's cross-cutting note 4 predicted AVX-512 would make the MinHash
loop native and "roughly double its ceiling." Measured on an 11th-gen
Intel i7-11850H (Tiger Lake, AVX-512F/DQ/BW/VL + VPOPCNTDQ + GFNI), Go
1.26, single-thread. Full single-thread + saturated tables for this
machine, including cross-machine notes, are in `benchmarks-tigerlake.md`. Detection extends `internal/cpuinfo` with
`HasAVX512` (CPUID leaf-7 AVX512F+DQ, plus the XCR0 opmask/ZMM_Hi256/
Hi16_ZMM bits — mask 0xE6 — so the OS preserves the wide state); each
amd64 kernel package gains a `useAVX512` var beside `useAVX2`, dispatch
preferring the widest available path, and `TestDispatchBothPaths` /
`TestEqCountPaths` force all three so every branch is covered on one
machine. All kernels stay bit-identical (oracle + golden).

**Kernel level (k=128, single block):**

| Kernel | scalar | AVX2 | AVX-512 | 512 vs AVX2 |
|---|---|---|---|---|
| MinHash `sketchBlock` (256-word) | 125 MB/s | 244 MB/s | **652 MB/s** | **2.66×** |
| Jaccard `eqCount` (k=128) | 62 ns | 19 ns | **13 ns** | **1.47×** |
| SimHash CSA, raw kernel (248-word) | — | 78 ns | **55 ns** | 1.42× |
| SimHash `pospopcnt`, full (252-word) | 1.08 µs | **0.84 µs** | 1.40 µs | *0.60×* |

**End-to-end MinHash, 100 KB doc:**

| Pipeline | scalar | AVX2 | AVX-512 | 512 vs AVX2 |
|---|---|---|---|---|
| Char (k-gram 8) | 14.4 MB/s | 26.0 MB/s | **58.0 MB/s** | **2.2×** |
| Words (w=3) | 82 MB/s | 127 MB/s | **196 MB/s** | 1.55× |

**Target 1 (MinHash) — the headline, over-delivered.** `sketchBlockAVX512`
is the AVX2 kernel with the two synthesized operations replaced by native
instructions — `VPMULLQ` (AVX512DQ) for the 64-bit lane multiply, `VPMINUQ`
(AVX512F) for the unsigned min — so the whole apparatus goes away: no
3×`vpmuludq` multiply, no sign-bias trick, no unbias at the end, and eight
permutations per ZMM instead of four. Kernel **2.66× over AVX2** (5.2× over
scalar) — better than the predicted ~2×, because AVX2 was paying for the
synthesis, not just the narrower lanes. End-to-end the Char pipeline (which
is ~85% permutation loop) rides most of that, **2.2×** over AVX2; Words is
Amdahl-capped by the shared tokenizer at 1.55×, as predicted.

**Target 3 (Jaccard) — clean win.** `eqCountAVX512`: `VPCMPEQQ` to an 8-bit
mask, then a merge-masked `VPADDQ` adds one to the lane counters in exactly
the equal lanes (no −1-mask subtraction). No reduction bottleneck, so the
width shows through directly: **1.47× over AVX2**, 4.8× over scalar.

**Target 2 (SimHash pospopcnt) — a negative result, kept.** The AVX-512 CSA
(eight banks per ZMM) *is* faster in isolation — 55 vs 78 ns raw, **1.42×**
— but `pospopcnt` is dominated by the scalar plane-expand (~90% of the
call), and doubling the bank count from four to eight *doubles* the
partial-count plane-words the expand must walk. Splitting each position's
count across more lanes yields more total set bits in the partial
representation (`popcount(31·d)` per lane × 8 lanes > `popcount(63·d)` × 4),
so the wider kernel optimizes the cheap part and inflates the expensive
one: **net 0.60× end-to-end**. Not shipped — AVX2 stays the pospopcnt
path, with the reasoning recorded in `csaAVX2`'s comment. The real lever
here is vectorizing the *expand* (a bit-plane→count transpose, a natural
fit for GFNI `gf2p8affineqb`, which this CPU has), not the CSA; that is a
separate, larger effort left as future work.

**Caveats.** Numbers are single-thread; Tiger Lake is a mobile part with
mild AVX-512 downclocking, so a Xeon/EPYC server will shift the ratios
(usually further in AVX-512's favor for the multiply-bound MinHash kernel,
and the pospopcnt verdict may differ where the 512-bit ALU ports are
wider). Round 3's saturation lesson still applies: size fleets from
saturated numbers, not these. The standing conclusions hold — width pays
where 64-bit arithmetic is unavoidable and dominant (MinHash multiply,
equality count), and reformulating the scalar bottleneck beats widening the
vector when the bottleneck is elsewhere (pospopcnt expand).

## Round 7: lane-parallel window hashing (a non-target, re-measured)

Round 6 inverted the profiles: with the permutation kernel 5.2× faster,
xxhash of Char's overlapping windows — dismissed in the original analysis
at ~5% share — had grown to **22%** of the Char pipeline. The "non-target"
verdict was economics, and the economics moved 4×. (Re-profiling after
each kernel round, not trusting remembered shares, is what caught this.)

`charHash8AVX512` hashes eight overlapping 8-byte windows per iteration:
one `VBROADCASTI32X4` replicates 16 text bytes to all lanes, one `VPSHUFB`
(AVX512BW, now part of the `HasAVX512` gate — every F+DQ chip has it)
builds the eight shifted windows, and the exact xxhash 8-byte path runs in
the 64-bit lanes — 5 `VPMULLQ`, 2 native `VPROLQ` rotates, and the
shift/xor avalanche. This is the doc's "lane-parallel xxhash" descendant
of the minio multi-buffer pattern, with one document supplying unlimited
independent lanes and none of minio's synchronization. Bit-identical to
`xxhash.Sum64` by construction (exact integer ops; oracle-tested across
patterns, golden tests unchanged).

The go/no-go gate was set at 1.5× over scalar at the kernel level;
measured **5.4×** (4096 windows: 12.9 → 2.35 µs, 0.57 ns/window — the
scalar loop pays xxhash's short-input path plus per-call overhead per
window; the kernel amortizes both across lanes). An AVX2 twin
(`charHash8AVX2`, same contract: one `VBROADCASTI128` feeding two
4-window YMM groups, the five multiplies synthesized from 3×`vpmuludq`
each and the rotates from shift pairs, prime constants as broadcast-table
memory operands) cleared the same gate at **2.4×** (5.65 µs, 1.38
ns/window) — AVX2-only machines get roughly half the AVX-512 win rather
than none. Full kernel ladder: scalar 300 → AVX2 725 → AVX-512
1615 MB/s. `Char` dispatches to the widest available path for k=8 (the
default char width) via a 256-hash stack buffer, scalar tail for the
last ≤8 windows and every other k; `CharBytes` shares the path.
Allocations unchanged — with one catch the alloc-pin test caught:
selecting the kernel through an indirect func value defeated escape
analysis and heap-allocated the buffer; the dispatch must stay a branch
over direct calls.

End-to-end Char 100 KB: 57.4 → **66–69 MB/s (~1.15–1.2×)**, and the
pipeline profile is now 76% permutation kernel + 15% yield/buffer loop,
with window hashing at 4% — hashing is off the table. `winnow.Text` at
k=8 rides the same path free; other k stay scalar (the kernel's
window-construction trick needs the 8-window group to fit a 16-byte
load, so generalizing to k ≤ 9 is possible but unmotivated until a
profile says so).

Remaining candidates, in profile order: the Words/SimHash front-end
(tokenizer classify + Mix-chain shingle fold + variable-length token
hashing, ~64% of Words), the GFNI plane-expand transpose for SimHash
(13%, would also un-shelve round 6's AVX-512 CSA), and trivial
`VPMINUQ`/`VPMAXUB` kernels for `minhash.Union`/`hll.Merge` if a
merge-heavy workload ever shows up. Float estimators (`Cardinality`,
`hll.Estimate`) are permanently out: vectorizing reorders the FP
summation, so a runtime-dispatched vector path would return different
bits on different CPUs — the one place SIMD would break the
determinism promise.

## Round 8: vectorized shingle fold (both kernels shipped, gate at 1.4×)

The Mix-chain shingle fold was ~28% of the Words pipeline and ~38% of
SketchText after round 7. Each w=3 shingle is three serial Mix steps
(rol31 + xor + multiply), but adjacent shingles are independent — so
lanes are just three shifted 64-byte views of the token stream, and each
Mix step is one native VPROLQ + VPXORQ + VPMULLQ. The first step's
rol31(MixInit) folds into a broadcast constant computed at setup with a
scalar ROLQ.

Kernels were gated over the *batch* scalar fold (a tight loop with full
ILP — the honest baseline, not the pipeline's per-token-arrival fold).
The gate opened at 1.5× and was settled at **1.4×**: the AVX2 twin's
1.46× sits between the two, and keeping it means AVX2-only machines get
part of the fold win rather than none.

| Kernel | 4096 shingles | vs scalar | gate (1.4×) |
|---|---|---|---|
| scalar batch fold | 4.66 µs (1.13 ns/shingle) | 1.0× | — |
| AVX2 (synthesized mul+rot) | 3.20 µs | **1.46×** | passed |
| AVX-512 | 1.42 µs (0.35 ns/shingle) | **3.3×** | passed |

The AVX2 margin is the counterpoint to round 7's: there the scalar
baseline paid xxhash's per-call overhead per window and AVX2 won 2.4×;
here the baseline is nine tight ALU ops per shingle, and the ~11
synthesized ops per Mix step leave only the 1.46× — the synthesis tax
almost exactly cancels the lane win.

Integration restructures the w=3 paths of `Words` and `WordsBlocks` from
fold-per-token to batched: token hashes accumulate in a 258-entry buffer
(256 shingles per drain plus the two carried window tokens), the kernel
folds eight at a time into a shingle buffer, and the scalar fold takes
the final non-multiple-of-8 tail. `WordsBlocks`' flush cadence is
bit-preserved (same shingle sequence, same block boundaries — pinned by
a dispatch test that compares cadence, not just hashes). The batching
state lives in one struct captured by the emit closure, so the alloc
pins hold unchanged (words 3, SketchText 1). Other w keep the ring fold;
`WordScanner` stays scalar (streaming chunk boundaries make batching
messy, and the fuzz-equivalence tests pin its output to WordsBlocks').

End-to-end, 100 KB: minhash Words 200 → **230 MB/s (1.15×)**, SketchText
273 → **307 MB/s (1.13×)**. Cumulative since the pre-AVX-512 baseline:
Words 127 → 230 (1.8×), SketchText 273 stands on rounds 1–7's gains.
The front-end remainder is now tokenizer scan/classify plus
variable-length token hashing — the two remaining round-7 candidates.

## Round 9: tokenizer classify (gate failed, not shipped)

The original target-4 prediction (2–4× from simdjson-style
classification) finally got its measurement — and failed the 1.4× gate.

The prototype was structurally sound: an AVX-512BW kernel classifying 64
bytes per op into two masks (definite ASCII boundaries; [a-z0-9] bytes)
via seven `VPCMPUB` range compares and mask-register logic, and a
mask-walk tokenizer exploiting the invariant that a token can never
contain an ASCII-boundary byte — maximal non-boundary spans are
independent segments, a pure-[a-z0-9] span is exactly one zero-copy
token, and impure spans fall to the scalar tokenizer over just that
span. Verified bit-identical against the scalar tokenizer by an
every-byte-value mask oracle plus 15 s of fuzz (830 k execs) over
corpora hitting chunk borders, long tokens, Unicode separators, and
open tails.

Measured at the tokenizer level (`tokenHashes` standalone, emit-only):

| Corpus | scalar | mask-driven | speedup |
|---|---|---|---|
| lowercase (the common case) | 15.4 µs | 12.6 µs | **1.2–1.25×** |
| mixed case/Unicode | 146 µs | 149 µs | **~1.0×** |

Under the gate on the favorable corpus, parity on the unfavorable one —
dropped. Two causes, both structural: (1) round 2's scalar tokenizer
rewrite already removed the per-byte classification burden for the
common case (zero-copy lowercase runs), so the 2–4× prediction was
priced against a baseline that no longer exists; (2) what remains of
`tokenHashes` is dominated by `xxhash.Sum64String` per ~6-byte token,
which classification cannot touch. The mask walk saves the scan but
pays span bookkeeping, netting ~20%.

The tokenizer lesson now has both halves: the scalar rewrite (round 2)
captured the win SIMD classification was predicted to deliver, and the
vector version afterwards has nothing left to accelerate. The
remaining front-end lever is lane-parallel hashing of the *tokens*
(variable length — the hard variant of round 7's fixed-width windows);
classification is exhausted.
