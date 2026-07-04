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
