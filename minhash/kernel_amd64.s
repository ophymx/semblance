//go:build amd64.v3

#include "textflag.h"

// func sketchBlockAVX2(dst, a, b, block []uint64)
//
// For each group of four permutations (one YMM register wide):
//   A  = a[i:i+4], As = A >> 32, B = b[i:i+4]
//   D0 = D1 = dst[i:i+4] XOR sign      (sign-biased running minima)
//   for each pair of elements x0, x1 in block:
//     v = lo64(A*x) + B, computed as
//         vpmuludq(A,X) + ((vpmuludq(As,X) + vpmuludq(A,Xs)) << 32)
//     Dj = min(Dj, v XOR sign)         (vpcmpgtq + vpblendvb; signed
//                                       compare on biased values is
//                                       unsigned compare on originals)
//   dst[i:i+4] = min(D0, D1) XOR sign
//
// Two accumulators (D0 for even elements, D1 for odd) halve the
// compare/blend dependency chain. Requires len(dst) % 4 == 0 and
// len(block) % 2 == 0, both nonzero.
TEXT ·sketchBlockAVX2(SB), NOSPLIT, $0-96
	MOVQ dst_base+0(FP), DI
	MOVQ dst_len+8(FP), DX
	MOVQ a_base+24(FP), R8
	MOVQ b_base+48(FP), R9
	MOVQ block_base+72(FP), R10
	MOVQ block_len+80(FP), CX
	SHRQ $2, DX                  // permutation groups

	MOVQ         $0x8000000000000000, AX
	VMOVQ        AX, X12
	VPBROADCASTQ X12, Y12        // sign bias

permloop:
	VMOVDQU (R8), Y0             // A
	VPSRLQ  $32, Y0, Y1          // As
	VMOVDQU (R9), Y2             // B
	VMOVDQU (DI), Y3
	VPXOR   Y12, Y3, Y3          // D0 (biased)
	VMOVDQA Y3, Y4               // D1

	MOVQ R10, SI
	MOVQ CX, BX

elemloop:
	// even element -> D0
	VPBROADCASTQ (SI), Y8
	VPSRLQ       $32, Y8, Y9
	VPMULUDQ     Y8, Y0, Y10
	VPMULUDQ     Y8, Y1, Y11
	VPMULUDQ     Y9, Y0, Y9
	VPADDQ       Y11, Y9, Y9
	VPSLLQ       $32, Y9, Y9
	VPADDQ       Y10, Y9, Y9
	VPADDQ       Y2, Y9, Y9
	VPXOR        Y12, Y9, Y9
	VPCMPGTQ     Y9, Y3, Y10
	VPBLENDVB    Y10, Y9, Y3, Y3

	// odd element -> D1
	VPBROADCASTQ 8(SI), Y8
	VPSRLQ       $32, Y8, Y11
	VPMULUDQ     Y8, Y0, Y10
	VPMULUDQ     Y8, Y1, Y14
	VPMULUDQ     Y11, Y0, Y11
	VPADDQ       Y14, Y11, Y11
	VPSLLQ       $32, Y11, Y11
	VPADDQ       Y10, Y11, Y11
	VPADDQ       Y2, Y11, Y11
	VPXOR        Y12, Y11, Y11
	VPCMPGTQ     Y11, Y4, Y10
	VPBLENDVB    Y10, Y11, Y4, Y4

	ADDQ $16, SI
	SUBQ $2, BX
	JNZ  elemloop

	VPCMPGTQ  Y4, Y3, Y10
	VPBLENDVB Y10, Y4, Y3, Y3    // min(D0, D1)
	VPXOR     Y12, Y3, Y3        // unbias
	VMOVDQU   Y3, (DI)

	ADDQ $32, DI
	ADDQ $32, R8
	ADDQ $32, R9
	DECQ DX
	JNZ  permloop

	VZEROUPPER
	RET
