//go:build amd64.v3

#include "textflag.h"

// func csaAVX2(planes *[8][4]uint64, block []uint64)
//
// Four independent carry-save-adder banks in the 64-bit lanes of Y0..Y7
// (plane p in Yp). Each iteration loads four words and ripple-carries them
// through the planes: t = plane AND c; plane = plane XOR c; c = t. The
// final carry out of plane 7 is dropped; callers guarantee at most 255
// words per lane so it is always zero.
TEXT ·csaAVX2(SB), NOSPLIT, $0-32
	MOVQ planes+0(FP), DI
	MOVQ block_base+8(FP), SI
	MOVQ block_len+16(FP), CX
	SHRQ $2, CX

	VPXOR Y0, Y0, Y0
	VPXOR Y1, Y1, Y1
	VPXOR Y2, Y2, Y2
	VPXOR Y3, Y3, Y3
	VPXOR Y4, Y4, Y4
	VPXOR Y5, Y5, Y5
	VPXOR Y6, Y6, Y6
	VPXOR Y7, Y7, Y7

	TESTQ CX, CX
	JZ    done

loop:
	VMOVDQU (SI), Y8

	VPAND Y0, Y8, Y9
	VPXOR Y8, Y0, Y0
	VPAND Y1, Y9, Y8
	VPXOR Y9, Y1, Y1
	VPAND Y2, Y8, Y9
	VPXOR Y8, Y2, Y2
	VPAND Y3, Y9, Y8
	VPXOR Y9, Y3, Y3
	VPAND Y4, Y8, Y9
	VPXOR Y8, Y4, Y4
	VPAND Y5, Y9, Y8
	VPXOR Y9, Y5, Y5
	VPAND Y6, Y8, Y9
	VPXOR Y8, Y6, Y6
	VPXOR Y9, Y7, Y7

	ADDQ $32, SI
	DECQ CX
	JNZ  loop

done:
	VMOVDQU Y0, (DI)
	VMOVDQU Y1, 32(DI)
	VMOVDQU Y2, 64(DI)
	VMOVDQU Y3, 96(DI)
	VMOVDQU Y4, 128(DI)
	VMOVDQU Y5, 160(DI)
	VMOVDQU Y6, 192(DI)
	VMOVDQU Y7, 224(DI)
	VZEROUPPER
	RET
