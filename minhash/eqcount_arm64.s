#include "textflag.h"

// func eqCountNEON(a, b []uint64) int
//
// VCMEQ produces -1 in equal lanes; adding the masks into two vector
// accumulators counts matches as negated sums, folded and negated at the
// end. Four lanes per iteration (two 2-lane vectors), mirroring the AVX2
// kernel's shape. Requires len(a) % 4 == 0 and len(a) > 0.
TEXT ·eqCountNEON(SB), NOSPLIT, $0-56
	MOVD a_base+0(FP), R0
	MOVD a_len+8(FP), R1
	MOVD b_base+24(FP), R2
	LSR  $2, R1                  // 4-lane groups
	CBZ  R1, eqzero              // defensive: no lanes -> count 0

	VEOR V4.B16, V4.B16, V4.B16  // mask accumulators (-1 per match)
	VEOR V5.B16, V5.B16, V5.B16

eqloop:
	VLD1.P 32(R0), [V0.D2, V1.D2]
	VLD1.P 32(R2), [V2.D2, V3.D2]
	VCMEQ  V0.D2, V2.D2, V6.D2
	VCMEQ  V1.D2, V3.D2, V7.D2
	VADD   V6.D2, V4.D2, V4.D2
	VADD   V7.D2, V5.D2, V5.D2
	SUBS   $1, R1
	BNE    eqloop

	VADD V5.D2, V4.D2, V4.D2
	VMOV V4.D[0], R3
	VMOV V4.D[1], R4
	ADD  R4, R3, R3
	NEG  R3, R3
	MOVD R3, ret+48(FP)
	RET

eqzero:
	MOVD $0, ret+48(FP)
	RET
