package simhash

import "math/bits"

// Positional popcount: count, for each bit position 0..63, how many words
// in a block have that bit set. The weight-1 sketch path reduces to this:
// after n unit-weight features of which s[i] have bit i set, the counter
// update is counts[i] += 2*s[i] - n. See docs/simd-analysis.md (target 1).
//
// The kernels use a carry-save adder over bit-planes: plane p holds bit p
// of a per-position tally, so adding a word is a ripple-carry of AND/XOR
// over at most 8 planes. 8 planes tally up to 255 words per bank, hence
// the block cap.

// pospopcntBlock is the maximum block length: the 8-plane CSA capacity.
const pospopcntBlock = 255

// accumulate folds a block of unit-weight feature hashes into counts.
func accumulate(counts *[64]int, block []uint64) {
	var s [64]int32
	pospopcnt(&s, block)
	for i := range counts {
		counts[i] += 2*int(s[i]) - len(block)
	}
}

// pospopcntGeneric is the portable scalar CSA kernel: cnt[i] += number of
// words in block with bit i set. len(block) must be <= pospopcntBlock.
func pospopcntGeneric(cnt *[64]int32, block []uint64) {
	var pl [8]uint64
	for _, h := range block {
		// Ripple-carry h into the planes; terminates within 8 steps
		// because each position's tally fits in 8 bits at <= 255 adds.
		for p := 0; h != 0; p++ {
			pl[p], h = pl[p]^h, pl[p]&h
		}
	}
	for p, w := range pl {
		expandPlane(cnt, w, uint(p))
	}
}

// expandPlane adds plane word w with significance 2^p into cnt.
func expandPlane(cnt *[64]int32, w uint64, p uint) {
	for w != 0 {
		i := bits.TrailingZeros64(w)
		cnt[i] += 1 << p
		w &= w - 1
	}
}
