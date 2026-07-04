//go:build amd64.v3

package minhash

// The AVX2 kernel processes four permutations per YMM register,
// permutation-block-major so the a/b parameters and the running minima
// live in registers across the whole element block. AVX2 lacks both a
// 64-bit lane multiply and an unsigned 64-bit min; the kernel synthesizes
// the multiply from three vpmuludq and the min from a sign-bias plus
// signed compare. Gated on GOAMD64=v3 like the simhash kernel.

// sketchBlockAVX2 requires len(dst) == len(a) == len(b), len(dst) % 4 == 0,
// len(block) % 2 == 0, and len(block) > 0.
//
//go:noescape
func sketchBlockAVX2(dst, a, b, block []uint64)

func sketchBlock(dst, a, b []uint64, block []uint64) {
	if n := len(block) &^ 1; len(dst)%4 == 0 && n >= 8 {
		sketchBlockAVX2(dst, a, b, block[:n])
		block = block[n:]
	}
	sketchBlockGeneric(dst, a, b, block)
}
