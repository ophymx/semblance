//go:build !amd64 && !arm64

package simhash

// pospopcnt dispatches to the portable kernel on platforms without a
// vector implementation.
func pospopcnt(cnt *[64]int32, block []uint64) {
	pospopcntGeneric(cnt, block)
}
