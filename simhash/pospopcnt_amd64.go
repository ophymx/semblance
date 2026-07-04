package simhash

import "github.com/ophymx/semblance/internal/cpuinfo"

// The AVX2 kernel runs four independent CSA banks in the 64-bit lanes of
// the YMM planes; the Go wrapper merges the banks. Dispatched at runtime
// via cpuinfo (a var so tests can exercise both paths).

var useAVX2 = cpuinfo.HasAVX2

// csaAVX2 ripple-carries each 4-word group of block into 8 YMM bit-planes
// and writes them to planes. Requires len(block) % 4 == 0 and
// len(block)/4 <= 255.
//
//go:noescape
func csaAVX2(planes *[8][4]uint64, block []uint64)

func pospopcnt(cnt *[64]int32, block []uint64) {
	if n := len(block) &^ 3; useAVX2 && n >= 16 {
		var planes [8][4]uint64
		csaAVX2(&planes, block[:n])
		for p := range planes {
			for _, w := range planes[p] {
				expandPlane(cnt, w, uint(p))
			}
		}
		block = block[n:]
	}
	pospopcntGeneric(cnt, block)
}
