package shingle

// The AVX-512 kernel folds eight w=3 shingles per iteration: the three
// Mix steps run as native VPROLQ/VPXORQ/VPMULLQ on three shifted views of
// the token stream (3.3× over the scalar batch fold). The AVX2 twin pays
// the synthesized multiply and rotate and measures 1.46× — over the 1.4×
// go/no-go gate, so AVX2-only machines get part of the win. See
// docs/simd-analysis.md, round 8.

// foldShingles3AVX512 computes dst[i] = Mix(Mix(Mix(MixInit, tokens[i]),
// tokens[i+1]), tokens[i+2]) — the w=3 shingle fold, eight lanes per
// iteration. Requires len(dst) % 8 == 0, len(dst) > 0, and
// len(tokens) >= len(dst)+2.
//
//go:noescape
func foldShingles3AVX512(dst, tokens []uint64)

// foldShingles3AVX2 is the AVX2 twin (four lanes, synthesized multiply
// and rotate), same contract but len(dst) % 4 == 0 suffices.
//
//go:noescape
func foldShingles3AVX2(dst, tokens []uint64)

// foldVector folds the leading lane-aligned prefix of the c complete
// shingles in toks into sbuf and returns how many it handled; the caller
// routes the remainder through the scalar fold. The kernel calls stay
// direct in each branch — an indirect kernel func value would defeat
// escape analysis and heap-allocate the caller's batching state.
func foldVector(sbuf, toks []uint64, c int) int {
	if useAVX512 {
		if v := c &^ 7; v > 0 {
			foldShingles3AVX512(sbuf[:v], toks)
			return v
		}
	} else if useAVX2 {
		if v := c &^ 3; v > 0 {
			foldShingles3AVX2(sbuf[:v], toks)
			return v
		}
	}
	return 0
}
