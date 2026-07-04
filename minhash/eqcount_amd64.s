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
