# Benchmarks: Tiger Lake (i7-11850H, AVX-512)

Third reference machine, to compare against the AVX2-only dev box and the
M1 Pro in `simd-analysis.md` (rounds 2–3). This is the first machine with
AVX-512, so it exercises the round-6/7/8 kernels (`sketchBlockAVX512`,
`eqCountAVX512`, `charHash8AVX512`, `foldShingles3AVX512`) that the other
two cannot.

## Machine

| | |
|---|---|
| CPU | 11th Gen Intel Core i7-11850H (Tiger Lake-H) |
| Cores / threads | 8 physical / 16 logical (SMT) |
| Clocks | 2.5 GHz base, 4.8 GHz turbo, 0.8 GHz min |
| ISA | AVX2 + AVX-512 F/DQ/BW/VL, VPOPCNTDQ, GFNI, VAES |
| Toolchain | go1.26.5 linux/amd64 |
| Kernel | Linux 6.12 |

**Caveat (mobile thermal/turbo).** This is a laptop part: single-core turbo
(4.8 GHz) is far above all-core sustained clocks, and the governor was
observed scaling around 60–65% at rest. Saturated per-core throughput
therefore drops well below single-thread × core-count; size fleets from the
saturated numbers, per round 3's standing lesson. Numbers are `-count=5`
(kernels) / `-count=3` (pipelines); representative run shown.

## Single-thread — per-ISA kernels

Direct kernel benchmarks (one block/op), forcing each dispatch path.
AVX-512 is unique to this machine; the AVX2 and scalar columns are
directly comparable to the AVX2 dev box.

| Kernel (size) | scalar | AVX2 | AVX-512 | 512 vs scalar | 512 vs AVX2 |
|---|---|---|---|---|---|
| `charHash8` window hash (4 KB) | 13.4 µs / 307 MB/s | 5.65 µs / 725 | **2.49 µs / 1646** | 5.4× | 2.3× |
| `foldShingles3` w=3 fold (4096) | 4.67 µs / 7.0 GB/s | 3.24 µs / 10.1 | **1.43 µs / 23.0 GB/s** | 3.3× | 2.3× |
| `sketchBlock` k=128 (256-word) | 16.1 µs / 127 MB/s | 8.38 µs / 244 | **3.09 µs / 664** | 5.2× | 2.7× |
| `eqCount` k=128 (Jaccard core) | 61.9 ns / 16.5 GB/s | 16.5 ns / 62 | **12.2 ns / 84 GB/s** | 5.1× | 1.4× |
| `pospopcnt` 252-word (CSA) | 1.32 µs / 1.52 GB/s | **0.74 µs / 2.71 GB/s** ¹ | — ² | — | 1.8× (AVX2 vs scalar) |

¹ `pospopcnt` ships AVX2 as the widest path; the dispatched benchmark
selects it. ² An AVX-512 CSA was measured 1.4× faster raw but is
expand-bound end-to-end and not shipped (round 6). `naive` per-bit loop for
reference: 33.5 µs / 60 MB/s.

## Single-thread — pipelines (100 KB docs)

End-to-end sketch pipelines under each dispatch path.

| Pipeline | scalar | AVX2 | AVX-512 | 512 vs AVX2 |
|---|---|---|---|---|
| minhash Char (k-gram 8) | 14.6 MB/s (7.0 ms) | 26.8 (3.82 ms) | **64.7 (1.58 ms)** | 2.4× |
| minhash Words (w=3) | 85.4 MB/s (1.20 ms) | 136.6 (0.749 ms) | **224.8 (0.455 ms)** | 1.6× |
| simhash SketchText (dispatched) | — | — | **308 MB/s (0.33 ms)** ³ | — |
| Jaccard k=128 (full, dispatched) | — | — | **13.5 ns** | — |

³ SketchText's shipped path already mixes ISAs (AVX-512 fold + AVX2
pospopcnt + scalar tokenizer); 308 MB/s is the dispatched number, the one
to compare cross-machine.

## Saturated vs single-thread scaling

`*Parallel` benchmarks (shared read-only input, per-goroutine output) at
`-cpu=1` and `-cpu=16` (same harness both ways, so the scaling factor is
apples-to-apples). Kernel rows are one 256/252-word block per op; pipeline
rows are 100 KB docs. Aggregate MB/s.

| Benchmark | 1 thread | 16 threads (sat.) | scaling |
|---|---|---|---|
| `sketchBlock` AVX-512 (kernel) | 670 | **4154** | 6.2× |
| `sketchBlock` scalar (kernel) | 128 | 830 | 6.5× |
| `pospopcnt` AVX2 (kernel) | 2704 | **19484** | 7.2× |
| `pospopcnt` scalar (kernel) | 2005 | 14244 | 7.1× |
| minhash Words pipeline | 231 | **1578** | 6.8× |
| simhash SketchText pipeline | 312 | **1781** | 5.7× |

**Reading the scaling.** 8 physical cores + SMT on 16 logical threads land
at ~6–7× aggregate, not 16×: SMT siblings share execution ports (and, on
this mobile part, all-core turbo is far below single-core), so single-thread
× 16 over-predicts by roughly 2–2.5×. Vector and scalar scale about
equally (6.2× vs 6.5× on sketchBlock; 7.2× vs 7.1× on pospopcnt), so the
single-thread SIMD advantage survives saturation: the AVX2 pospopcnt edge
over scalar roughly holds (1.35× at 1 thread → 1.37× saturated) and
sketchBlock AVX-512's edge over scalar holds near 5× (5.2× → 5.0×).
Saturated aggregate peaks: **~1.8 GB/s** SimHash
sketching, **~1.6 GB/s** the full MinHash words pipeline, **~19 GB/s** raw
positional-popcount.

Minor note: the 1-thread column here is the `-cpu=1` parallel run; a few
rows differ slightly from the single-thread kernel section above (e.g.
scalar `pospopcnt` reads higher) because per-core turbo state shifts between
runs on this mobile part. Trust the scaling factor (same-harness ratio),
not cross-section absolute equality.

## Cross-machine comparison

The AVX2-only dev box and M1 Pro numbers live in `simd-analysis.md`
round 3. Two things to carry over when comparing:

- **This is the only AVX-512 machine**, so the round-6/7/8 kernels
  (`sketchBlock`/`eqCount`/`charHash8`/`foldShingles3` AVX-512) have no
  counterpart on the other two — compare their AVX2 columns, and treat the
  AVX-512 column as this machine's ceiling.
- **Absolute throughput is not comparable across silicon generations** —
  this Tiger Lake part is materially faster per-core than the older dev box
  in round 3 even on the same AVX2 path, so a slowdown/speedup claim only
  means something *within one machine*. The portable facts are the per-ISA
  *ratios* (AVX-512 ≈ 2.3–2.7× AVX2 on the 64-bit-arithmetic kernels here)
  and the scaling factors above.
