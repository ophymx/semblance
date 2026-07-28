package shingle

import "github.com/ophymx/semblance/internal/cpuinfo"

// The AVX-512 kernel hashes eight overlapping 8-byte windows per iteration
// (one 16-byte broadcast + VPSHUFB builds the windows; VPMULLQ/VPROLQ run
// the exact xxhash 8-byte path in the lanes). Dispatched at runtime via
// cpuinfo (a var so tests can exercise both paths).

var useAVX512 = cpuinfo.HasAVX512

// charHash8AVX512 computes dst[i] = XXH64(text[i:i+8]) for i in
// [0, len(dst)). Requires len(dst) % 8 == 0, len(dst) > 0, and
// len(text) >= len(dst)+8.
//
//go:noescape
func charHash8AVX512(dst []uint64, text string)

// charHash8Seq yields the hashes of text's 8-byte windows from offset 0
// using the vector kernel, in 256-hash stack blocks. It returns the first
// window index it did not yield — the scalar loop in Char continues from
// there (the kernel needs a spare byte past each 8-window group, so the
// last few windows always fall to scalar) — and whether yield stopped the
// iteration.
func charHash8Seq(text string, yield func(uint64) bool) (int, bool) {
	if !useAVX512 || len(text) < 16 {
		return 0, false
	}
	n := (len(text) - 8) &^ 7
	var buf [256]uint64
	i := 0
	for i < n {
		c := min(len(buf), n-i)
		charHash8AVX512(buf[:c], text[i:])
		for _, h := range buf[:c] {
			if !yield(h) {
				return 0, true
			}
		}
		i += c
	}
	return n, false
}
