//go:build !amd64 && !arm64

package minhash

// sketchBlock dispatches to the portable kernel on platforms without a
// vector implementation.
func sketchBlock(dst, a, b []uint64, block []uint64) {
	sketchBlockGeneric(dst, a, b, block)
}
