# Benchmarks: Comet Lake dev box (i7-10610U, AVX2-only)

The "amd64 dev box" of `simd-analysis.md` rounds 2–5, re-measured after
rounds 6–9 landed. This machine has no AVX-512, so its value is the other
half of the round 6–8 story: what the AVX2 twin kernels deliver on
AVX2-only silicon, and a regression check that the AVX-512 dispatch
plumbing and the zero-length kernel guards cost nothing on the paths this
machine actually runs.

## Machine

| | |
|---|---|
| CPU | Intel Core i7-10610U (Comet Lake-U) |
| Cores / threads | 4 physical / 8 logical (SMT) |
| Clocks | 1.8 GHz base, 4.9 GHz turbo |
| ISA | AVX2 (no AVX-512) |
| Toolchain | go1.26.5 linux/amd64 |
| Kernel | Linux 6.12 |

**Caveat (ultrabook thermal/turbo).** A 15 W U-series part: single-core
turbo is far above sustained all-core clocks, so saturated throughput is
well below single-thread × core-count, and absolute numbers drift a few
percent run to run with turbo state. Numbers are medians of `-count=8`;
the before/after deltas are same-machine benchstat comparisons (n=8,
all quoted deltas p<0.01).

## What rounds 6–9 delivered on this box

Same-machine benchstat, pre-round-6 tree vs current, dispatched paths,
100 KB docs:

| Pipeline | before | after | speedup |
|---|---|---|---|
| root `Sketcher` (fused) | 83.7 MB/s | **126.6** | 1.51× |
| minhash Words (w=3) | 76.9 MB/s | **114.6** | 1.49× |
| minhash Char (k-gram 8) | 20.4 MB/s | **22.2** | 1.09× |
| simhash SketchText | 126.7 MB/s | **259.3** | 2.05× |

All of it comes from the round-7/8 AVX2 twins (`charHash8AVX2`,
`foldShingles3AVX2`) accelerating the shingle front-end — this box never
executes the round-6 AVX-512 kernels. SketchText doubles because its
pospopcnt back-end was already vectorized (round 2), leaving shingle
production as the dominant cost; Char moves least because its profile is
~85% MinHash permutation loop, whose AVX2 kernel predates this round.

Regression check, statistically flat as intended: `Jaccard`,
`sketchBlock` (kernel and dispatched), `pospopcnt` all unchanged, and
allocation counts identical — the security-hardening zero-length guards
and the wider dispatch machinery are free on the hot paths.

## Single-thread — per-ISA kernels

Forcing each dispatch path. The scalar and AVX2 columns are directly
comparable to the same columns in `benchmarks-tigerlake.md`.

| Kernel (size) | scalar | AVX2 | AVX2 vs scalar |
|---|---|---|---|
| `charHash8` window hash (4 KB) | 18.7 µs / 219 MB/s | **6.85 µs / 597** | 2.7× |
| `foldShingles3` w=3 fold (4096) | 5.93 µs / 5.49 GB/s | **4.33 µs / 7.63 GB/s** | 1.37–1.39× ¹ |
| `sketchBlock` k=128 (256-word) | 18.5 µs / 110 MB/s | **10.7 µs / 189** | 1.71× |
| `eqCount` k=128 (Jaccard core) | 75.1 ns / 13.8 GB/s | **21.7 ns / 48.1 GB/s** | 3.5× |
| `pospopcnt` 252-word (CSA) | 1.34 µs / 1.50 GB/s ² | **0.93 µs / 2.13 GB/s** | 1.4× |

¹ Under the 1.4× ship gate that round 8 settled on (the gate was measured
at 1.46× on Tiger Lake's AVX2 path). On this older core the synthesis tax
eats almost the entire lane win, exactly the round-8 caveat — but the
kernel stays net-positive end-to-end here (Words +49%), so the gate
decision holds. Data point: the AVX2 fold margin thins with core age.
² generic (pure-Go) path; `naive` per-bit loop for reference: 45.5 µs /
44 MB/s.

Per-path pipelines (100 KB): Char scalar 12.6 → AVX2 22.7 MB/s (1.80×);
Words scalar 75.0 → AVX2 113.1 MB/s (1.51×).

## Saturated vs single-thread scaling

`*Parallel` benchmarks at default `-cpu=8` (4 physical cores + SMT).
Aggregate MB/s.

| Benchmark | 1 thread | 8 threads (sat.) | scaling |
|---|---|---|---|
| `sketchBlock` AVX2 (kernel) | 189 | **509** | 2.7× |
| `pospopcnt` AVX2 (kernel) | 2135 | **6200** | 2.9× |
| minhash Words pipeline | 115 | **306** | 2.7× |
| simhash SketchText pipeline | 259 | **785** | 3.0× |
| winnow `Text` scan | 28 | **93** | 3.3× |
| winnow `Index.Overlap` query | 1.7 | **3.6** | 2.1× |

4 physical cores land at ~2.7–3.3× aggregate — the round-3 lesson again
(this box "behaves as 4 physical cores + SMT"), compressed further by
U-series all-core clocks. Saturated aggregate peaks: **~0.8 GB/s** SimHash
sketching, **~0.3 GB/s** the full MinHash words pipeline, **~6.2 GB/s**
raw positional popcount.

## Winnow (fragment provenance)

Same harness as the Tiger Lake tables (synthetic high-entropy docs,
planted shared passage).

| Operation | size | time/op | throughput |
|---|---|---|---|
| `Text` scan | 1 KB | 33.1 µs | 31.3 MB/s |
| `Text` scan | 100 KB | 3.66 ms | 28.0 MB/s |
| `Overlaps` (spans) | 2×100 KB, 4 KB shared | 12.1 ms | 16.9 MB/s |
| `Index.Add` | 100 KB | 9.03 ms | 11.3 MB/s |
| `Index.Overlap` (score) | 2 KB query / 100 docs | 1.48 ms | — |
| `Index.Matches` (spans) | 2 KB query / 100 docs | 0.48 ms | — |

## Cross-machine notes

Compared with Tiger Lake's AVX2 column (the comparable one), the per-ISA
*ratios* mostly travel: charHash8 2.7× here vs 2.4× there, eqCount 3.5×
vs 3.8×, sketchBlock 1.71× vs 1.92×. The one that doesn't is the round-8
fold (1.37–1.39× vs 1.46×), thin enough that its ship gate would have
failed if measured only on this machine — the concrete case for the
same-machine-baseline rule and for gating on more than one
microarchitecture. Absolute throughput is not comparable across machines;
per round 3, size fleets from saturated numbers on the deployment
hardware.
