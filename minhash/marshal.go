package minhash

import (
	"encoding/binary"
	"fmt"
)

// Wire format (frozen; versioned):
//
//	[version:1][algo:1][k:2 LE][seed:8 LE][values: k × 8 bytes LE]
//
// The header makes stored signatures self-describing: a compatible
// MinHasher can be reconstructed from the bytes alone via New(k, seed).
// The algorithm byte keeps future incompatible families (e.g.
// one-permutation hashing) from silently mixing with these signatures.
// Algorithm ids are registered in docs/design-notes.md.
const (
	formatVersion = 1
	algoKPerm     = 1 // k independent multiply-add permutations
	headerSize    = 1 + 1 + 2 + 8
)

// MarshalSignature encodes sig together with this hasher's parameters in
// the versioned wire format, headerSize + 8*k bytes. Panics if
// len(sig) != K.
func (m *MinHasher) MarshalSignature(sig Signature) []byte {
	if len(sig) != len(m.a) {
		panic("minhash: signature length does not match k")
	}
	buf := make([]byte, 0, headerSize+8*len(sig))
	buf = append(buf, formatVersion, algoKPerm)
	buf = binary.LittleEndian.AppendUint16(buf, uint16(len(sig)))
	buf = binary.LittleEndian.AppendUint64(buf, m.seed)
	for _, v := range sig {
		buf = binary.LittleEndian.AppendUint64(buf, v)
	}
	return buf
}

// UnmarshalSignature decodes a signature marshaled by a MinHasher with the
// same parameters. It returns an error if the data is malformed (see
// [DecodeSignature]) or if the encoded k or seed does not match this
// hasher — the mismatch that in-memory comparison cannot detect.
func (m *MinHasher) UnmarshalSignature(data []byte) (Signature, error) {
	sig, seed, err := DecodeSignature(data)
	if err != nil {
		return nil, err
	}
	if len(sig) != len(m.a) {
		return nil, fmt.Errorf("minhash: signature k=%d does not match hasher k=%d", len(sig), len(m.a))
	}
	if seed != m.seed {
		return nil, fmt.Errorf("minhash: signature seed %#x does not match hasher seed %#x", seed, m.seed)
	}
	return sig, nil
}

// DecodeSignature decodes a marshaled signature without requiring a
// MinHasher, returning the signature and the seed it was sketched with
// (its k is len(sig)). Use New(len(sig), seed) to reconstruct a hasher
// producing comparable signatures. It returns an error on an unknown
// version or algorithm, truncated input, or trailing bytes.
func DecodeSignature(data []byte) (sig Signature, seed uint64, err error) {
	if len(data) < headerSize {
		return nil, 0, fmt.Errorf("minhash: truncated signature: %d bytes", len(data))
	}
	if data[0] != formatVersion {
		return nil, 0, fmt.Errorf("minhash: unknown signature format version %d", data[0])
	}
	if data[1] != algoKPerm {
		return nil, 0, fmt.Errorf("minhash: unknown signature algorithm id %d", data[1])
	}
	k := int(binary.LittleEndian.Uint16(data[2:4]))
	if k == 0 {
		return nil, 0, fmt.Errorf("minhash: invalid signature length 0")
	}
	seed = binary.LittleEndian.Uint64(data[4:12])
	if want := headerSize + 8*k; len(data) != want {
		return nil, 0, fmt.Errorf("minhash: signature with k=%d must be %d bytes, got %d", k, want, len(data))
	}
	sig = make(Signature, k)
	for i := range sig {
		sig[i] = binary.LittleEndian.Uint64(data[headerSize+8*i:])
	}
	return sig, seed, nil
}
