//go:build amd64.v3

package minhash

// eqCountAVX2 counts positions where a and b match, four lanes per
// iteration (vpcmpeqq accumulated as vector subtraction of the -1 masks).
// Requires len(a) == len(b), len(a) % 4 == 0, and len(a) > 0.
//
//go:noescape
func eqCountAVX2(a, b []uint64) int

func eqCount(a, b []uint64) int {
	n := len(a) &^ 3
	eq := 0
	if n >= 16 {
		eq = eqCountAVX2(a[:n], b[:n])
		a, b = a[n:], b[n:]
	}
	return eq + eqCountGeneric(a, b)
}
