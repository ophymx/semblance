package cpuinfo

// HasAVX2 reports whether both the CPU and the operating system support
// AVX2 (the OS must save/restore YMM state, checked via OSXSAVE+XGETBV).
// Detected once at startup.
var HasAVX2 = detectAVX2()

func detectAVX2() bool {
	maxID, _, _, _ := cpuid(0, 0)
	if maxID < 7 {
		return false
	}
	_, _, ecx1, _ := cpuid(1, 0)
	const (
		osxsave = 1 << 27
		avx     = 1 << 28
	)
	if ecx1&osxsave == 0 || ecx1&avx == 0 {
		return false
	}
	// XCR0 bits 1 (SSE/XMM) and 2 (AVX/YMM) must both be OS-enabled.
	if eax, _ := xgetbv(); eax&0x6 != 0x6 {
		return false
	}
	_, ebx7, _, _ := cpuid(7, 0)
	return ebx7&(1<<5) != 0 // AVX2
}

func cpuid(eaxIn, ecxIn uint32) (eax, ebx, ecx, edx uint32)

func xgetbv() (eax, edx uint32)
