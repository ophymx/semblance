package cpuinfo

// HasAVX2 reports whether both the CPU and the operating system support
// AVX2 (the OS must save/restore YMM state, checked via OSXSAVE+XGETBV).
// Detected once at startup.
var HasAVX2 = detectAVX2()

// HasAVX512 reports whether both the CPU and the operating system support
// the AVX-512 subset the kernels use: AVX512F (foundation: ZMM, masks,
// vpminuq, vpaddq), AVX512DQ (vpmullq, the native 64-bit lane multiply),
// and AVX512BW (vpshufb on ZMM). Every CPU shipping F+DQ (Skylake-X
// onward) also has BW, so requiring it excludes no real hardware. The OS
// must save/restore the extended state — opmask, ZMM_Hi256, and Hi16_ZMM —
// checked via XGETBV. Detected once at startup.
var HasAVX512 = detectAVX512()

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

func detectAVX512() bool {
	maxID, _, _, _ := cpuid(0, 0)
	if maxID < 7 {
		return false
	}
	_, _, ecx1, _ := cpuid(1, 0)
	const osxsave = 1 << 27
	if ecx1&osxsave == 0 {
		return false
	}
	// XCR0 must have the AVX bits (1,2) plus the AVX-512 bits: 5 (opmask),
	// 6 (ZMM_Hi256), and 7 (Hi16_ZMM) — mask 0xE6 — so the OS preserves the
	// full ZMM/mask state across context switches.
	if eax, _ := xgetbv(); eax&0xe6 != 0xe6 {
		return false
	}
	_, ebx7, _, _ := cpuid(7, 0)
	const (
		avx512f  = 1 << 16
		avx512dq = 1 << 17
		avx512bw = 1 << 30
	)
	return ebx7&avx512f != 0 && ebx7&avx512dq != 0 && ebx7&avx512bw != 0
}

func cpuid(eaxIn, ecxIn uint32) (eax, ebx, ecx, edx uint32)

func xgetbv() (eax, edx uint32)
