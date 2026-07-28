package minhash

import "github.com/ophymx/semblance/internal/cpuinfo"

// The AVX2 kernel processes four permutations per YMM register,
// permutation-block-major so the a/b parameters and the running minima
// live in registers across the whole element block. AVX2 lacks both a
// 64-bit lane multiply and an unsigned 64-bit min; the kernel synthesizes
// the multiply from three vpmuludq and the min from a sign-bias plus
// signed compare. Dispatched at runtime via cpuinfo (a var so tests can
// exercise both paths).

var (
	useAVX2   = cpuinfo.HasAVX2
	useAVX512 = cpuinfo.HasAVX512
)

// sketchBlockAVX2 requires len(dst) == len(a) == len(b), len(dst) % 4 == 0,
// len(block) % 2 == 0, and len(block) > 0.
//
//go:noescape
func sketchBlockAVX2(dst, a, b, block []uint64)

// sketchBlockAVX512 is the wider twin: native VPMULLQ/VPMINUQ, eight
// permutations per ZMM. Requires len(dst) == len(a) == len(b),
// len(dst) % 8 == 0, len(block) % 2 == 0, and len(block) > 0.
//
//go:noescape
func sketchBlockAVX512(dst, a, b, block []uint64)

func sketchBlock(dst, a, b []uint64, block []uint64) {
	n := len(block) &^ 1
	switch {
	case n >= 8 && useAVX512 && len(dst)%8 == 0:
		sketchBlockAVX512(dst, a, b, block[:n])
		block = block[n:]
	case n >= 8 && useAVX2 && len(dst)%4 == 0:
		sketchBlockAVX2(dst, a, b, block[:n])
		block = block[n:]
	}
	sketchBlockGeneric(dst, a, b, block)
}
