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

// xxhash prime constants as 4-lane broadcast tables for the AVX2 kernel's
// memory operands (EVEX embedded broadcast is AVX-512-only). The *s tables
// hold prime>>32: VPMULUDQ reads only the low 32 bits of each lane, so
// they supply the high halves for the synthesized 64-bit multiply.
DATA charP1<>+0(SB)/8, $0x9E3779B185EBCA87
DATA charP1<>+8(SB)/8, $0x9E3779B185EBCA87
DATA charP1<>+16(SB)/8, $0x9E3779B185EBCA87
DATA charP1<>+24(SB)/8, $0x9E3779B185EBCA87
GLOBL charP1<>(SB), RODATA|NOPTR, $32
DATA charP1s<>+0(SB)/8, $0x9E3779B1
DATA charP1s<>+8(SB)/8, $0x9E3779B1
DATA charP1s<>+16(SB)/8, $0x9E3779B1
DATA charP1s<>+24(SB)/8, $0x9E3779B1
GLOBL charP1s<>(SB), RODATA|NOPTR, $32
DATA charP2<>+0(SB)/8, $0xC2B2AE3D27D4EB4F
DATA charP2<>+8(SB)/8, $0xC2B2AE3D27D4EB4F
DATA charP2<>+16(SB)/8, $0xC2B2AE3D27D4EB4F
DATA charP2<>+24(SB)/8, $0xC2B2AE3D27D4EB4F
GLOBL charP2<>(SB), RODATA|NOPTR, $32
DATA charP2s<>+0(SB)/8, $0xC2B2AE3D
DATA charP2s<>+8(SB)/8, $0xC2B2AE3D
DATA charP2s<>+16(SB)/8, $0xC2B2AE3D
DATA charP2s<>+24(SB)/8, $0xC2B2AE3D
GLOBL charP2s<>(SB), RODATA|NOPTR, $32
DATA charP3<>+0(SB)/8, $0x165667B19E3779F9
DATA charP3<>+8(SB)/8, $0x165667B19E3779F9
DATA charP3<>+16(SB)/8, $0x165667B19E3779F9
DATA charP3<>+24(SB)/8, $0x165667B19E3779F9
GLOBL charP3<>(SB), RODATA|NOPTR, $32
DATA charP3s<>+0(SB)/8, $0x165667B1
DATA charP3s<>+8(SB)/8, $0x165667B1
DATA charP3s<>+16(SB)/8, $0x165667B1
DATA charP3s<>+24(SB)/8, $0x165667B1
GLOBL charP3s<>(SB), RODATA|NOPTR, $32
DATA charP4<>+0(SB)/8, $0x85EBCA77C2B2AE63
DATA charP4<>+8(SB)/8, $0x85EBCA77C2B2AE63
DATA charP4<>+16(SB)/8, $0x85EBCA77C2B2AE63
DATA charP4<>+24(SB)/8, $0x85EBCA77C2B2AE63
GLOBL charP4<>(SB), RODATA|NOPTR, $32
DATA charInit<>+0(SB)/8, $0x27D4EB2F165667CD
DATA charInit<>+8(SB)/8, $0x27D4EB2F165667CD
DATA charInit<>+16(SB)/8, $0x27D4EB2F165667CD
DATA charInit<>+24(SB)/8, $0x27D4EB2F165667CD
GLOBL charInit<>(SB), RODATA|NOPTR, $32

// CHARMUL64 synthesizes the 64-bit lane multiply v *= prime that AVX2
// lacks: lo(p)*lo(v) + ((hi(p)*lo(v) + lo(p)*hi(v)) << 32), with the
// prime's halves supplied by the p/ps broadcast tables.
#define CHARMUL64(v, p, ps, t1, t2) \
	VPSRLQ   $32, v, t1   \
	VPMULUDQ p, t1, t1    \
	VPMULUDQ ps, v, t2    \
	VPADDQ   t2, t1, t1   \
	VPSLLQ   $32, t1, t1  \
	VPMULUDQ p, v, v      \
	VPADDQ   t1, v, v

// CHARROL rotates each 64-bit lane left by r.
#define CHARROL(r, v, t1) \
	VPSLLQ $r, v, t1        \
	VPSRLQ $(64-r), v, v    \
	VPOR   t1, v, v

// CHARXSHIFT is one avalanche step: v ^= v >> r.
#define CHARXSHIFT(r, v, t1) \
	VPSRLQ $r, v, t1  \
	VPXOR  t1, v, v

// func charHash8AVX2(dst []uint64, text string)
//
// The AVX2 twin of charHash8AVX512, same contract: dst[i] =
// XXH64(text[i:i+8]), len(dst) % 8 == 0 and > 0, len(text) >=
// len(dst)+8. One 16-byte broadcast feeds two YMM groups of four windows
// (the halves of the ZMM shuffle table); the two chains interleave to
// cover the synthesized-multiply latency. Everything AVX-512 gets for
// free is synthesized here: the 64-bit multiply from 3 vpmuludq
// (CHARMUL64) and the rotates from shift pairs (CHARROL).
TEXT ·charHash8AVX2(SB), NOSPLIT, $0-40
	MOVQ dst_base+0(FP), DI
	MOVQ dst_len+8(FP), CX
	MOVQ text_base+24(FP), SI
	SHRQ $3, CX

avx2loop:
	VBROADCASTI128 (SI), Y0
	VPSHUFB charShuf<>+0(SB), Y0, Y1  // windows 0..3
	VPSHUFB charShuf<>+32(SB), Y0, Y2 // windows 4..7

	CHARMUL64(Y1, charP2<>(SB), charP2s<>(SB), Y3, Y4) // v * prime2
	CHARMUL64(Y2, charP2<>(SB), charP2s<>(SB), Y5, Y6)
	CHARROL(31, Y1, Y3)
	CHARROL(31, Y2, Y5)
	CHARMUL64(Y1, charP1<>(SB), charP1s<>(SB), Y3, Y4) // k1 = rol31(...)*p1
	CHARMUL64(Y2, charP1<>(SB), charP1s<>(SB), Y5, Y6)
	VPXOR charInit<>(SB), Y1, Y1                       // h = (p5+8) ^ k1
	VPXOR charInit<>(SB), Y2, Y2
	CHARROL(27, Y1, Y3)
	CHARROL(27, Y2, Y5)
	CHARMUL64(Y1, charP1<>(SB), charP1s<>(SB), Y3, Y4)
	CHARMUL64(Y2, charP1<>(SB), charP1s<>(SB), Y5, Y6)
	VPADDQ charP4<>(SB), Y1, Y1                        // h = rol27(h)*p1 + p4
	VPADDQ charP4<>(SB), Y2, Y2

	CHARXSHIFT(33, Y1, Y3)                             // avalanche
	CHARXSHIFT(33, Y2, Y5)
	CHARMUL64(Y1, charP2<>(SB), charP2s<>(SB), Y3, Y4)
	CHARMUL64(Y2, charP2<>(SB), charP2s<>(SB), Y5, Y6)
	CHARXSHIFT(29, Y1, Y3)
	CHARXSHIFT(29, Y2, Y5)
	CHARMUL64(Y1, charP3<>(SB), charP3s<>(SB), Y3, Y4)
	CHARMUL64(Y2, charP3<>(SB), charP3s<>(SB), Y5, Y6)
	CHARXSHIFT(32, Y1, Y3)
	CHARXSHIFT(32, Y2, Y5)

	VMOVDQU Y1, (DI)
	VMOVDQU Y2, 32(DI)
	ADDQ $8, SI
	ADDQ $64, DI
	DECQ CX
	JNZ  avx2loop

	VZEROUPPER
	RET
