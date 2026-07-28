package shingle

import "github.com/ophymx/semblance/internal/hashutil"

// The w=3 shingle fold runs batched on every platform: token hashes
// accumulate in a buffer and are folded 256 shingles at a time, replacing
// the per-token-arrival ring fold. On amd64 the batch feeds the AVX-512/
// AVX2 kernels (docs/simd-analysis.md, round 8); elsewhere it feeds the
// batch scalar fold, which beats the ring fold on its own — a tight
// sequential loop with full ILP instead of modular window indexing per
// token (round 10).

// foldShingles3Scalar is the batch scalar fold: the oracle for the vector
// kernels, the tail path for non-multiple-of-lane counts, and the whole
// path on platforms without a kernel. Computes
// dst[i] = Mix(Mix(Mix(MixInit, tokens[i]), tokens[i+1]), tokens[i+2]);
// requires len(tokens) >= len(dst)+2.
func foldShingles3Scalar(dst, tokens []uint64) {
	for i := range dst {
		acc := hashutil.Mix(hashutil.MixInit, tokens[i])
		acc = hashutil.Mix(acc, tokens[i+1])
		dst[i] = hashutil.Mix(acc, tokens[i+2])
	}
}

// words3State is the shared batching state for the w=3 paths: raw token
// hashes accumulate in toks (256 shingles per drain plus the two carried
// tokens), folded shingles land in sbuf. One struct so the tokenHashes
// emit closure captures a single variable.
type words3State struct {
	toks [258]uint64
	sbuf [256]uint64
	nt   int // tokens buffered
}

// drain folds every complete shingle in st.toks into st.sbuf and returns
// the count. mid-scan drains are always full (256); the final drain
// routes the sub-lane-group tail (everything, on platforms without a
// vector kernel) through the scalar fold. The last two tokens are carried
// to the front for the next window.
func (st *words3State) drain() int {
	c := st.nt - 2
	if c <= 0 {
		return 0
	}
	v := foldVector(st.sbuf[:], st.toks[:], c)
	if v < c {
		foldShingles3Scalar(st.sbuf[v:c], st.toks[v:st.nt])
	}
	st.toks[0], st.toks[1] = st.toks[st.nt-2], st.toks[st.nt-1]
	st.nt = 2
	return c
}

// words3Seq is Words' batched w=3 path: yield per shingle, folds batched
// through drain.
func words3Seq(text string, yield func(uint64) bool) {
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
				return
			}
		}
	}
}

// words3Blocks is WordsBlocks' batched w=3 path, preserving its exact
// flush cadence (same shingle sequence, same block boundaries).
func words3Blocks(text string, block []uint64, flush func(hashes []uint64) bool) {
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
}
