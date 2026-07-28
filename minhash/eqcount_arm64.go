package minhash

// eqCountNEON counts positions where a and b match, four lanes per
// iteration (VCMEQ masks accumulated as vector adds of the -1 lanes).
// NEON is baseline on arm64, so no build gate is needed. Requires
// len(a) == len(b), len(a) % 4 == 0, and len(a) > 0.
//
//go:noescape
func eqCountNEON(a, b []uint64) int

func eqCount(a, b []uint64) int {
	eq := 0
	if n := len(a) &^ 3; n >= 16 {
		eq = eqCountNEON(a[:n], b[:n])
		a, b = a[n:], b[n:]
	}
	return eq + eqCountGeneric(a, b)
}
