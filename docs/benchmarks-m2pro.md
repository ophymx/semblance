# Benchmarks: M2 Pro mac mini (arm64/NEON)

The arm64 reference machine, measured after round 10 (NEON `eqCount` +
portable batch fold) landed. One label correction: this box is an
**Apple M2 Pro** and always was — the round-2/3 arm64 tables in
`simd-analysis.md` said "M1 Pro" by mistake (corrected in round 10).
Those older rows are same-machine, just older commits and toolchains;
for deltas, prefer the fresh baselines here.

## Machine

| | |
|---|---|
| CPU | Apple M2 Pro (T6020), mac mini |
| Cores | 6 performance + 4 efficiency |
| ISA | NEON (baseline arm64; no SVE) |
| Toolchain | go1.26.5 (cross-compiled darwin/arm64 test binaries) |
| OS | macOS 26.5 |

Numbers are medians of `-count=8`; deltas are same-machine benchstat
(n=8). Apple silicon runs remarkably quiet (±0–1% between counts), but
fixed-work pipeline benches still show a ~±3% placement/layout band
between *binaries* — confirmed bimodal on interleaved A/B runs, so
single-benchmark deltas under ~4% between builds are not conclusions.

## What round 10 delivered on this box

| Benchmark | before | after | change |
|---|---|---|---|
| `Jaccard` k=128 | 52.9 ns | **20.4 ns** | 2.6× |
| `JaccardMany` | 4.69 µs | **1.97 µs** | 2.4× |
| simhash SketchText 100 KB | 295.7 MB/s | **332.9** | +12.6% |
| minhash Words 100 KB | 114.8 MB/s | **120.5** | +4.8% |
| Words w=3 allocations | 1 alloc | **0** | window alloc gone |

The rest of the suite is flat, as intended (Char moves ±3.6% between
runs — inside the placement band above; `sketchBlock` itself is flat at
±0.1%).

## Single-thread — per-ISA kernels

| Kernel (size) | scalar/generic | NEON | NEON vs scalar |
|---|---|---|---|
| `eqCount` k=128 (Jaccard core) | 48.7 ns / 21.0 GB/s | **19.5 ns / 52.4 GB/s** | **2.49×** |
| `sketchBlock` k=128 (256-word) | 13.4 µs / 153 MB/s | **10.8 µs / 190** | 1.24× |
| `pospopcnt` 252-word (CSA) | 851 ns / 2.37 GB/s | **702 ns / 2.87 GB/s** | 1.21× |

The ordering matches round 1's ISA analysis: `eqCount` (compare +
accumulate, no multiplies) is exactly what NEON is good at and
over-delivers its 2–3× prediction band's midpoint; `sketchBlock` is
capped by the synthesized 64-bit multiply; `pospopcnt` competes against
an M-series scalar core that is already excellent.

## Single-thread — pipelines (100 KB docs)

| Pipeline | throughput | time/op |
|---|---|---|
| minhash Words (w=3) | 120.5 MB/s | 850 µs |
| minhash Char (k-gram 8) | 19.3 MB/s | 5.31 ms |
| simhash SketchText | 332.9 MB/s | 308 µs |
| root `Sketcher` (fused) | 124.9 MB/s | 820 µs |

Cross-machine (same commit): Words 120.5 here vs 114.6 Comet Lake AVX2
vs 224.8 Tiger Lake AVX-512; SketchText 332.9 beats both (259.3 / 308).
The M2's scalar front-end plus the batch fold outruns Tiger Lake's
AVX-512 fold pipeline — per-ISA ratios, not absolutes, are the portable
facts, but the absolute ranking makes the point that arm64 needs no
apology on Words/SketchText. Char is the exception (19.3 vs 64.7 on
Tiger Lake): it is ~85% permutation loop, where NEON's missing 64-bit
lane multiply caps the kernel at 1.24× while AVX-512 runs 5.2× — the
round-1 asymmetry, permanent on this ISA generation.

## Saturated vs single-thread scaling

`*Parallel` benchmarks at default `-cpu=10` (6P+4E). Aggregate MB/s.

| Benchmark | 1 thread | 10 threads (sat.) | scaling |
|---|---|---|---|
| `sketchBlock` NEON (kernel) | 190 | **1287** | 6.8× |
| `pospopcnt` NEON (kernel) | 2871 | **22462** | 7.8× |
| minhash Words pipeline | 120 | **829** | 6.9× |
| simhash SketchText pipeline | 333 | **2487** | 7.5× |
| winnow `Text` scan | 109 | **650** | 5.9× |
| winnow `Index.Overlap` query | — | — | 3.3× |

6P+4E lands at ~6–8× — the efficiency cores contribute real throughput
(unlike SMT siblings sharing ports), and there is no turbo cliff:
saturated aggregate peaks at **~2.5 GB/s** SimHash sketching and
**~22 GB/s** raw positional popcount, both ahead of the Tiger Lake
laptop's saturated numbers.

## Winnow (fragment provenance)

| Operation | size | time/op | throughput |
|---|---|---|---|
| `Text` scan | 1 KB | 7.0 µs | 148 MB/s |
| `Text` scan | 100 KB | 937 µs | 109 MB/s |
| `Overlaps` (spans) | 2×100 KB, 4 KB shared | 5.43 ms | 37.7 MB/s |
| `Index.Add` | 100 KB | 4.47 ms | 22.9 MB/s |
| `Index.Overlap` (score) | 2 KB query / 100 docs | 863 µs | — |
| `Index.Matches` (spans) | 2 KB query / 100 docs | 258 µs | — |

Winnow's selection loop loves this core: single-thread `Text` runs 1.4×
Tiger Lake and 3.9× Comet Lake.
