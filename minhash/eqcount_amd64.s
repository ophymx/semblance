#include "textflag.h"

// func eqCountAVX2(a, b []uint64) int
//
// vpcmpeqq produces -1 in equal lanes; subtracting the mask from a vector
// counter adds 1 per equal lane. The four lane counters are summed at the
// end. Requires len(a) % 4 == 0 and len(a) > 0.
TEXT ·eqCountAVX2(SB), NOSPLIT, $0-56
	MOVQ a_base+0(FP), SI
	MOVQ a_len+8(FP), CX
	MOVQ b_base+24(FP), DI
	SHRQ $2, CX
	TESTQ CX, CX                 // defensive: no lanes -> count 0
	JZ    eqavx2zero

	VPXOR Y2, Y2, Y2

loop:
	VMOVDQU  (SI), Y0
	VMOVDQU  (DI), Y1
	VPCMPEQQ Y1, Y0, Y0
	VPSUBQ   Y0, Y2, Y2
	ADDQ     $32, SI
	ADDQ     $32, DI
	DECQ     CX
	JNZ      loop

	VEXTRACTI128 $1, Y2, X1
	VPADDQ       X1, X2, X2
	VPSHUFD      $0x4E, X2, X1
	VPADDQ       X1, X2, X2
	VMOVQ        X2, AX
	MOVQ         AX, ret+48(FP)
	VZEROUPPER
	RET

eqavx2zero:
	MOVQ $0, ret+48(FP)
	RET

// func eqCountAVX512(a, b []uint64) int
//
// The AVX-512 twin of eqCountAVX2: eight lanes per iteration. VPCMPEQQ
// writes an 8-bit mask; a merge-masked VPADDQ adds one to the vector
// counter in exactly the equal lanes (no -1-mask subtraction needed). The
// eight lane counters are summed at the end. Requires len(a) % 8 == 0 and
// len(a) > 0.
TEXT ·eqCountAVX512(SB), NOSPLIT, $0-56
	MOVQ a_base+0(FP), SI
	MOVQ a_len+8(FP), CX
	MOVQ b_base+24(FP), DI
	SHRQ $3, CX
	TESTQ CX, CX                 // defensive: no lanes -> count 0
	JZ    eqavx512zero

	VPXORQ       Z2, Z2, Z2      // lane counters
	MOVQ         $1, AX
	VPBROADCASTQ AX, Z1          // ones

loop512:
	VMOVDQU64 (SI), Z0
	VPCMPEQQ  (DI), Z0, K1       // K1 = equal lanes
	VPADDQ    Z1, Z2, K1, Z2     // counters += 1 where equal
	ADDQ      $64, SI
	ADDQ      $64, DI
	DECQ      CX
	JNZ       loop512

	VEXTRACTI64X4 $1, Z2, Y1     // fold high 256 into low
	VPADDQ        Y1, Y2, Y2
	VEXTRACTI128  $1, Y2, X1
	VPADDQ        X1, X2, X2
	VPSHUFD       $0x4E, X2, X1
	VPADDQ        X1, X2, X2
	VMOVQ         X2, AX
	MOVQ          AX, ret+48(FP)
	VZEROUPPER
	RET

eqavx512zero:
	MOVQ $0, ret+48(FP)
	RET
