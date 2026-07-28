#include "textflag.h"

// func foldShingles3AVX512(dst, tokens []uint64)
//
// dst[i] = Mix(Mix(Mix(MixInit, tokens[i]), tokens[i+1]), tokens[i+2]) —
// the w=3 shingle fold, eight shingles per iteration. Each Mix step is
// rol31 + xor + multiply, all native ZMM ops; the chains of adjacent
// shingles are independent, so lanes are just shifted views of the token
// stream (three overlapping 64-byte loads). The first step's
// rol31(MixInit) is folded into a broadcast constant computed at setup.
// Requires len(dst) % 8 == 0, len(dst) > 0, len(tokens) >= len(dst)+2.
TEXT ·foldShingles3AVX512(SB), NOSPLIT, $0-48
	MOVQ dst_base+0(FP), DI
	MOVQ dst_len+8(FP), CX
	MOVQ tokens_base+24(FP), SI
	SHRQ $3, CX

	MOVQ $0x9E3779B97F4A7C15, AX // the Mix multiplier
	VPBROADCASTQ AX, Z15
	MOVQ $0xC2B2AE3D27D4EB4F, AX // MixInit
	ROLQ $31, AX                 // rol31(MixInit), step 1 pre-rotated
	VPBROADCASTQ AX, Z14

loop:
	VMOVDQU64 (SI), Z0           // tokens[i..i+7]
	VMOVDQU64 8(SI), Z1          // tokens[i+1..i+8]
	VMOVDQU64 16(SI), Z2         // tokens[i+2..i+9]

	VPXORQ  Z14, Z0, Z0          // acc = rol31(init) ^ t0
	VPMULLQ Z15, Z0, Z0          // acc *= K
	VPROLQ  $31, Z0, Z0
	VPXORQ  Z1, Z0, Z0
	VPMULLQ Z15, Z0, Z0
	VPROLQ  $31, Z0, Z0
	VPXORQ  Z2, Z0, Z0
	VPMULLQ Z15, Z0, Z0

	VMOVDQU64 Z0, (DI)
	ADDQ $64, SI
	ADDQ $64, DI
	DECQ CX
	JNZ  loop

	VZEROUPPER
	RET

// Broadcast tables for the AVX2 twin: the Mix multiplier K, its high half
// for the synthesized multiply, and rol31(MixInit).
DATA foldK<>+0(SB)/8, $0x9E3779B97F4A7C15
DATA foldK<>+8(SB)/8, $0x9E3779B97F4A7C15
DATA foldK<>+16(SB)/8, $0x9E3779B97F4A7C15
DATA foldK<>+24(SB)/8, $0x9E3779B97F4A7C15
GLOBL foldK<>(SB), RODATA|NOPTR, $32
DATA foldKs<>+0(SB)/8, $0x9E3779B9
DATA foldKs<>+8(SB)/8, $0x9E3779B9
DATA foldKs<>+16(SB)/8, $0x9E3779B9
DATA foldKs<>+24(SB)/8, $0x9E3779B9
GLOBL foldKs<>(SB), RODATA|NOPTR, $32
DATA foldInit<>+0(SB)/8, $0x93EA75A7E159571E // rol31(MixInit)
DATA foldInit<>+8(SB)/8, $0x93EA75A7E159571E
DATA foldInit<>+16(SB)/8, $0x93EA75A7E159571E
DATA foldInit<>+24(SB)/8, $0x93EA75A7E159571E
GLOBL foldInit<>(SB), RODATA|NOPTR, $32

// FOLDMUL64 synthesizes v *= K (AVX2 has no 64-bit lane multiply).
#define FOLDMUL64(v, t1, t2) \
	VPSRLQ   $32, v, t1           \
	VPMULUDQ foldK<>(SB), t1, t1  \
	VPMULUDQ foldKs<>(SB), v, t2  \
	VPADDQ   t2, t1, t1           \
	VPSLLQ   $32, t1, t1          \
	VPMULUDQ foldK<>(SB), v, v    \
	VPADDQ   t1, v, v

// FOLDROL31 rotates each 64-bit lane left by 31.
#define FOLDROL31(v, t1) \
	VPSLLQ $31, v, t1  \
	VPSRLQ $33, v, v   \
	VPOR   t1, v, v

// func foldShingles3AVX2(dst, tokens []uint64)
//
// The AVX2 twin of foldShingles3AVX512: four lanes, the multiply
// synthesized from 3 vpmuludq and the rotate from a shift pair, prime
// constants as broadcast-table memory operands. Measured 1.46x over the
// scalar batch fold — kept under the project's 1.4x gate so AVX2-only
// machines get part of the fold win. Requires len(dst) % 4 == 0,
// len(dst) > 0, len(tokens) >= len(dst)+2.
TEXT ·foldShingles3AVX2(SB), NOSPLIT, $0-48
	MOVQ dst_base+0(FP), DI
	MOVQ dst_len+8(FP), CX
	MOVQ tokens_base+24(FP), SI
	SHRQ $2, CX

avx2loop:
	VMOVDQU (SI), Y0             // tokens[i..i+3]
	VMOVDQU 8(SI), Y1
	VMOVDQU 16(SI), Y2

	VPXOR foldInit<>(SB), Y0, Y0
	FOLDMUL64(Y0, Y3, Y4)
	FOLDROL31(Y0, Y3)
	VPXOR Y1, Y0, Y0
	FOLDMUL64(Y0, Y3, Y4)
	FOLDROL31(Y0, Y3)
	VPXOR Y2, Y0, Y0
	FOLDMUL64(Y0, Y3, Y4)

	VMOVDQU Y0, (DI)
	ADDQ $32, SI
	ADDQ $32, DI
	DECQ CX
	JNZ  avx2loop

	VZEROUPPER
	RET
