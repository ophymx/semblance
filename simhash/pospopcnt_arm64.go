package simhash

// The NEON kernel runs four independent CSA banks: two in the 64-bit lanes
// of V0..V7 and two in V16..V23, giving the wide M-series/Neoverse SIMD
// units two independent ripple chains per iteration. NEON is baseline on
// arm64, so no build gate or dispatch is needed.

// csaNEON ripple-carries each 4-word group of block into the bit-plane
// banks and writes them to planes (plane p: bank words at planes[p]).
// Requires len(block) % 4 == 0 and len(block)/4 <= 255.
//
//go:noescape
func csaNEON(planes *[8][4]uint64, block []uint64)

func pospopcnt(cnt *[64]int32, block []uint64) {
	if n := len(block) &^ 3; n >= 16 {
		var planes [8][4]uint64
		csaNEON(&planes, block[:n])
		for p := range planes {
			for _, w := range planes[p] {
				expandPlane(cnt, w, uint(p))
			}
		}
		block = block[n:]
	}
	pospopcntGeneric(cnt, block)
}
