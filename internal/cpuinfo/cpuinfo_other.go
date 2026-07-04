//go:build !amd64

package cpuinfo

// HasAVX2 is false on non-amd64 platforms.
const HasAVX2 = false
