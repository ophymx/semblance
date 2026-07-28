//go:build !amd64

package shingle

// foldVector has no vector implementation on this platform; the batched
// scalar fold handles every shingle.
func foldVector(sbuf, toks []uint64, c int) int { return 0 }
