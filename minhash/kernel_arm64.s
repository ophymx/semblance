#include "textflag.h"

// func sketchBlockNEON(dst, a, b, block []uint64)
//
// For each group of four permutations (two 2-lane vector pairs):
//   A0lo/A0hi, A1lo/A1hi = the 32-bit halves of a[i:i+4], narrowed to .2S
//   B0, B1 = b[i:i+4]; D*a = D*b = dst[i:i+4] (running minima)
//   for each pair of elements x0, x1:
//     cross = umull(Alo, xhi) + umlal(Ahi, xlo)   (full 64-bit lanes)
//     v     = ((cross << 32) + umull(Alo, xlo)) + B
//     D     = bit(D, v, cmhi(D, v))               (unsigned 64-bit min)
//   dst[i:i+4] = min(D*a, D*b)
//
// Two accumulator sets (even/odd elements) halve the compare/insert
// dependency chain, mirroring the AVX2 kernel. Requires len(dst) % 4 == 0
// and len(block) % 2 == 0, both nonzero.
//
// XTN, UMULL, UMLAL, and CMHI are not in the Go assembler's arm64
// vocabulary; they are emitted as WORD directives with the intended
// instruction alongside. The kernel oracle test (TestSketchBlock, run on
// arm64 hardware) verifies the encodings.
TEXT ·sketchBlockNEON(SB), NOSPLIT, $0-96
	MOVD dst_base+0(FP), R0
	MOVD dst_len+8(FP), R1
	MOVD a_base+24(FP), R2
	MOVD b_base+48(FP), R3
	MOVD block_base+72(FP), R4
	MOVD block_len+80(FP), R5
	LSR  $2, R1                  // permutation groups

permloop:
	VLD1 (R2), [V0.D2, V1.D2]    // A0, A1
	VUSHR $32, V0.D2, V2.D2
	VUSHR $32, V1.D2, V3.D2
	WORD $0x0EA12804             // XTN V4.2S, V0.2D   (A0lo)
	WORD $0x0EA12845             // XTN V5.2S, V2.2D   (A0hi)
	WORD $0x0EA12826             // XTN V6.2S, V1.2D   (A1lo)
	WORD $0x0EA12867             // XTN V7.2S, V3.2D   (A1hi)
	VLD1 (R3), [V16.D2, V17.D2]  // B0, B1
	VLD1 (R0), [V8.D2, V9.D2]    // D0a, D1a
	VMOV V8.B16, V10.B16         // D0b
	VMOV V9.B16, V11.B16         // D1b

	MOVD R4, R6                  // block cursor
	MOVD R5, R7                  // elements remaining

elemloop:
	// even element x0 -> accumulator set a
	MOVD.P 8(R6), R8
	LSR    $32, R8, R9
	VDUP   R8, V12.S2            // x0 lo
	VDUP   R9, V13.S2            // x0 hi

	WORD $0x2EADC08E             // UMULL V14.2D, V4.2S, V13.2S
	WORD $0x2EAC80AE             // UMLAL V14.2D, V5.2S, V12.2S
	VSHL $32, V14.D2, V14.D2
	WORD $0x2EACC08F             // UMULL V15.2D, V4.2S, V12.2S
	VADD V15.D2, V14.D2, V14.D2
	VADD V16.D2, V14.D2, V14.D2
	WORD $0x6EEE350F             // CMHI V15.2D, V8.2D, V14.2D
	VBIT V15.B16, V14.B16, V8.B16

	WORD $0x2EADC0CE             // UMULL V14.2D, V6.2S, V13.2S
	WORD $0x2EAC80EE             // UMLAL V14.2D, V7.2S, V12.2S
	VSHL $32, V14.D2, V14.D2
	WORD $0x2EACC0CF             // UMULL V15.2D, V6.2S, V12.2S
	VADD V15.D2, V14.D2, V14.D2
	VADD V17.D2, V14.D2, V14.D2
	WORD $0x6EEE352F             // CMHI V15.2D, V9.2D, V14.2D
	VBIT V15.B16, V14.B16, V9.B16

	// odd element x1 -> accumulator set b
	MOVD.P 8(R6), R8
	LSR    $32, R8, R9
	VDUP   R8, V12.S2
	VDUP   R9, V13.S2

	WORD $0x2EADC08E             // UMULL V14.2D, V4.2S, V13.2S
	WORD $0x2EAC80AE             // UMLAL V14.2D, V5.2S, V12.2S
	VSHL $32, V14.D2, V14.D2
	WORD $0x2EACC08F             // UMULL V15.2D, V4.2S, V12.2S
	VADD V15.D2, V14.D2, V14.D2
	VADD V16.D2, V14.D2, V14.D2
	WORD $0x6EEE354F             // CMHI V15.2D, V10.2D, V14.2D
	VBIT V15.B16, V14.B16, V10.B16

	WORD $0x2EADC0CE             // UMULL V14.2D, V6.2S, V13.2S
	WORD $0x2EAC80EE             // UMLAL V14.2D, V7.2S, V12.2S
	VSHL $32, V14.D2, V14.D2
	WORD $0x2EACC0CF             // UMULL V15.2D, V6.2S, V12.2S
	VADD V15.D2, V14.D2, V14.D2
	VADD V17.D2, V14.D2, V14.D2
	WORD $0x6EEE356F             // CMHI V15.2D, V11.2D, V14.2D
	VBIT V15.B16, V14.B16, V11.B16

	SUBS $2, R7
	BNE  elemloop

	// merge accumulator sets: D*a = min(D*a, D*b)
	WORD $0x6EEA350E             // CMHI V14.2D, V8.2D, V10.2D
	VBIT V14.B16, V10.B16, V8.B16
	WORD $0x6EEB352E             // CMHI V14.2D, V9.2D, V11.2D
	VBIT V14.B16, V11.B16, V9.B16
	VST1.P [V8.D2, V9.D2], 32(R0)

	ADD  $32, R2
	ADD  $32, R3
	SUBS $1, R1
	BNE  permloop
	RET
