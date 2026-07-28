package shingle

import "github.com/ophymx/semblance/internal/hashutil"

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

// foldShingles3Scalar is the batch scalar fold: the oracle for the kernel
// and the tail path for non-multiple-of-8 counts.
func foldShingles3Scalar(dst, tokens []uint64) {
	for i := range dst {
		acc := hashutil.Mix(hashutil.MixInit, tokens[i])
		acc = hashutil.Mix(acc, tokens[i+1])
		dst[i] = hashutil.Mix(acc, tokens[i+2])
	}
}

// words3State is the shared batching state for the vector w=3 paths: raw
// token hashes accumulate in toks (256 shingles per drain plus the two
// carried tokens), folded shingles land in sbuf. One struct so the
// tokenHashes emit closure captures a single variable.
type words3State struct {
	toks [258]uint64
	sbuf [256]uint64
	nt   int // tokens buffered
}

// drain folds every complete shingle in st.toks into st.sbuf and returns
// the count. mid-scan drains are always full (256, kernel-only); the
// final drain routes the sub-lane-group tail through the scalar fold.
// The last two tokens are carried to the front for the next window. The
// kernel calls stay direct in each branch — an indirect kernel func value
// would defeat escape analysis and heap-allocate the state.
func (st *words3State) drain() int {
	c := st.nt - 2
	if c <= 0 {
		return 0
	}
	v := 0
	if useAVX512 {
		if v = c &^ 7; v > 0 {
			foldShingles3AVX512(st.sbuf[:v], st.toks[:])
		}
	} else if useAVX2 {
		if v = c &^ 3; v > 0 {
			foldShingles3AVX2(st.sbuf[:v], st.toks[:])
		}
	}
	if v < c {
		foldShingles3Scalar(st.sbuf[v:c], st.toks[v:st.nt])
	}
	st.toks[0], st.toks[1] = st.toks[st.nt-2], st.toks[st.nt-1]
	st.nt = 2
	return c
}

// words3Seq is Words' w=3 vector path: yield per shingle, batched through
// the fold kernel. Returns false (untouched input) when the kernel is
// unavailable.
func words3Seq(text string, yield func(uint64) bool) bool {
	if !useAVX512 && !useAVX2 {
		return false
	}
	var st words3State
	ok := true
	tokenHashes(text, func(th uint64) bool {
		st.toks[st.nt] = th
		st.nt++
		if st.nt < len(st.toks) {
			return true
		}
		for _, h := range st.sbuf[:st.drain()] {
			if !yield(h) {
				ok = false
				return false
			}
		}
		return true
	})
	if ok {
		for _, h := range st.sbuf[:st.drain()] {
			if !yield(h) {
				return true
			}
		}
	}
	return true
}

// words3Blocks is WordsBlocks' w=3 vector path, preserving its exact
// flush cadence (same shingle sequence, same block boundaries). Returns
// false (untouched input) when the kernel is unavailable.
func words3Blocks(text string, block []uint64, flush func(hashes []uint64) bool) bool {
	if !useAVX512 && !useAVX2 {
		return false
	}
	var st words3State
	nb := 0
	ok := true
	deliver := func(sh []uint64) bool {
		for len(sh) > 0 {
			n := copy(block[nb:], sh)
			nb += n
			sh = sh[n:]
			if nb == len(block) {
				ok = flush(block)
				nb = 0
				if !ok {
					return false
				}
			}
		}
		return true
	}
	tokenHashes(text, func(th uint64) bool {
		st.toks[st.nt] = th
		st.nt++
		if st.nt < len(st.toks) {
			return true
		}
		return deliver(st.sbuf[:st.drain()])
	})
	if ok && deliver(st.sbuf[:st.drain()]) && nb > 0 {
		flush(block[:nb])
	}
	return true
}
