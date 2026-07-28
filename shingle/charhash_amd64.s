#include "textflag.h"

// Shuffle indices building eight overlapping 8-byte windows from a 16-byte
// block replicated to all four 128-bit lanes: lane L produces windows 2L
// (bytes 2L..2L+7) and 2L+1 (bytes 2L+1..2L+8). VPSHUFB indexes within
// each lane, and every needed byte index is <= 14.
DATA charShuf<>+0(SB)/8, $0x0706050403020100  // window 0: bytes 0..7
DATA charShuf<>+8(SB)/8, $0x0807060504030201  // window 1: bytes 1..8
DATA charShuf<>+16(SB)/8, $0x0908070605040302 // window 2
DATA charShuf<>+24(SB)/8, $0x0A09080706050403 // window 3
DATA charShuf<>+32(SB)/8, $0x0B0A090807060504 // window 4
DATA charShuf<>+40(SB)/8, $0x0C0B0A0908070605 // window 5
DATA charShuf<>+48(SB)/8, $0x0D0C0B0A09080706 // window 6
DATA charShuf<>+56(SB)/8, $0x0E0D0C0B0A090807 // window 7
GLOBL charShuf<>(SB), RODATA|NOPTR, $64

// func charHash8AVX512(dst []uint64, text string)
//
// dst[i] = XXH64(text[i:i+8], seed 0) — the lane-parallel form of the
// scalar per-window xxhash.Sum64String loop in Char, eight windows per
// iteration. Each iteration broadcasts 16 text bytes to all lanes,
// shuffles them into eight overlapping windows, and runs the exact
// xxhash 8-byte path in the 64-bit lanes:
//
//   h = (prime5 + 8) XOR (rol31(v*prime2)*prime1)
//   h = rol27(h)*prime1 + prime4
//   h ^= h>>33; h *= prime2; h ^= h>>29; h *= prime3; h ^= h>>32
//
// Bit-identical to xxhash.Sum64 by construction (exact integer ops).
// Requires len(dst) % 8 == 0, len(dst) > 0, and len(text) >= len(dst)+8
// (each iteration loads 16 bytes at the group's start offset).
TEXT ·charHash8AVX512(SB), NOSPLIT, $0-40
	MOVQ dst_base+0(FP), DI
	MOVQ dst_len+8(FP), CX
	MOVQ text_base+24(FP), SI
	SHRQ $3, CX

	VMOVDQU64 charShuf<>(SB), Z15
	MOVQ $0x9E3779B185EBCA87, AX // prime1
	VPBROADCASTQ AX, Z12
	MOVQ $0xC2B2AE3D27D4EB4F, AX // prime2
	VPBROADCASTQ AX, Z13
	MOVQ $0x165667B19E3779F9, AX // prime3
	VPBROADCASTQ AX, Z10
	MOVQ $0x85EBCA77C2B2AE63, AX // prime4
	VPBROADCASTQ AX, Z11
	MOVQ $0x27D4EB2F165667CD, AX // prime5 + 8 (the length term)
	VPBROADCASTQ AX, Z14

loop:
	VBROADCASTI32X4 (SI), Z8     // 16 text bytes in every lane
	VPSHUFB Z15, Z8, Z9          // eight overlapping windows, one per lane

	VPMULLQ Z13, Z9, Z9          // v * prime2
	VPROLQ  $31, Z9, Z9
	VPMULLQ Z12, Z9, Z9          // k1 = rol31(v*p2) * p1
	VPXORQ  Z14, Z9, Z9          // h = (p5+8) ^ k1
	VPROLQ  $27, Z9, Z9
	VPMULLQ Z12, Z9, Z9
	VPADDQ  Z11, Z9, Z9          // h = rol27(h)*p1 + p4

	VPSRLQ $33, Z9, Z8           // avalanche
	VPXORQ Z8, Z9, Z9
	VPMULLQ Z13, Z9, Z9          // * p2
	VPSRLQ $29, Z9, Z8
	VPXORQ Z8, Z9, Z9
	VPMULLQ Z10, Z9, Z9          // * p3
	VPSRLQ $32, Z9, Z8
	VPXORQ Z8, Z9, Z9

	VMOVDQU64 Z9, (DI)
	ADDQ $8, SI
	ADDQ $64, DI
	DECQ CX
	JNZ  loop

	VZEROUPPER
	RET
