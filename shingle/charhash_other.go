//go:build !amd64

package shingle

// charHash8Seq has no vector implementation on this platform; Char's
// scalar loop handles every window.
func charHash8Seq(string, func(uint64) bool) (int, bool) { return 0, false }
