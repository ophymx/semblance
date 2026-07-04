//go:build !amd64.v3

package minhash

// eqCount dispatches to the portable kernel on platforms without a vector
// implementation.
func eqCount(a, b []uint64) int {
	return eqCountGeneric(a, b)
}
