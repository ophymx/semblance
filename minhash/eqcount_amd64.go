package minhash

// eqCountAVX2 counts positions where a and b match, four lanes per
// iteration (vpcmpeqq accumulated as vector subtraction of the -1 masks).
// Requires len(a) == len(b), len(a) % 4 == 0, and len(a) > 0.
//
//go:noescape
func eqCountAVX2(a, b []uint64) int

// eqCountAVX512 is the wider twin: eight lanes per iteration, VPCMPEQQ to a
// mask plus a merge-masked VPADDQ into the lane counters. Requires
// len(a) == len(b), len(a) % 8 == 0, and len(a) > 0.
//
//go:noescape
func eqCountAVX512(a, b []uint64) int

func eqCount(a, b []uint64) int {
	eq := 0
	switch {
	case useAVX512 && len(a)&^7 >= 16:
		n := len(a) &^ 7
		eq = eqCountAVX512(a[:n], b[:n])
		a, b = a[n:], b[n:]
	case useAVX2 && len(a)&^3 >= 16:
		n := len(a) &^ 3
		eq = eqCountAVX2(a[:n], b[:n])
		a, b = a[n:], b[n:]
	}
	return eq + eqCountGeneric(a, b)
}
