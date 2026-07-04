package simhash

import (
	"encoding/binary"
	"fmt"
)

// Wire format (frozen; versioned):
//
//	[version:1][algo:1][fingerprint:8 LE]
//
// The algorithm byte shares the registry in docs/design-notes.md with the
// minhash signature format, so incompatible sketch families can never
// silently mix in storage.
const (
	formatVersion = 1
	algoSimHash64 = 2 // 64-bit SimHash
	marshaledSize = 1 + 1 + 8
)

// MarshalBinary implements encoding.BinaryMarshaler using the versioned
// 10-byte wire format. The error is always nil.
func (f Fingerprint) MarshalBinary() ([]byte, error) {
	return f.AppendBinary(make([]byte, 0, marshaledSize))
}

// AppendBinary implements encoding.BinaryAppender: it appends the marshaled
// fingerprint to b and returns the extended slice. The error is always nil.
func (f Fingerprint) AppendBinary(b []byte) ([]byte, error) {
	b = append(b, formatVersion, algoSimHash64)
	return binary.LittleEndian.AppendUint64(b, uint64(f)), nil
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler. It returns an
// error on an unknown version or algorithm, or if data is not exactly the
// marshaled size.
func (f *Fingerprint) UnmarshalBinary(data []byte) error {
	if len(data) != marshaledSize {
		return fmt.Errorf("simhash: marshaled fingerprint must be %d bytes, got %d", marshaledSize, len(data))
	}
	if data[0] != formatVersion {
		return fmt.Errorf("simhash: unknown fingerprint format version %d", data[0])
	}
	if data[1] != algoSimHash64 {
		return fmt.Errorf("simhash: unknown fingerprint algorithm id %d", data[1])
	}
	*f = Fingerprint(binary.LittleEndian.Uint64(data[2:]))
	return nil
}
