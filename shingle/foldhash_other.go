//go:build !amd64

package shingle

// words3Seq has no vector implementation on this platform; Words' scalar
// fold handles every shingle.
func words3Seq(string, func(uint64) bool) bool { return false }

// words3Blocks has no vector implementation on this platform.
func words3Blocks(string, []uint64, func(hashes []uint64) bool) bool { return false }
