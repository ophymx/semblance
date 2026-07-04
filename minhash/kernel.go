package minhash

// sketchBlockSize is the number of element hashes buffered before invoking
// the block kernel. 2 KB of stack; large enough to amortize per-block
// overhead, small enough to stay cache- and stack-friendly.
const sketchBlockSize = 256

// sketchBlock folds a block of element hashes into dst:
// dst[i] = min(dst[i], a[i]*x + b[i]) over all x in block, for each
// permutation i. Element-major order measures fastest in scalar Go
// (permutation-major variants lost 10-40% — see docs/simd-analysis.md);
// an assembly kernel would replace this whole function and choose its own
// access pattern. Results are order-independent because min is commutative
// and associative.
func sketchBlock(dst, a, b []uint64, block []uint64) {
	a = a[:len(dst)]
	b = b[:len(dst)]
	for _, x := range block {
		for i := range dst {
			if v := a[i]*x + b[i]; v < dst[i] {
				dst[i] = v
			}
		}
	}
}
