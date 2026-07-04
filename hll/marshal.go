package hll

import "fmt"

// Wire format (frozen; versioned):
//
//	[version:1][algo:1][p:1][registers: 2^p bytes]
//
// The algorithm byte shares the module-wide registry in
// docs/design-notes.md (1 = MinHash k-perm, 2 = SimHash-64, 3 = this).
const (
	formatVersion = 1
	algoHLL       = 3
	headerSize    = 3
)

// MarshalBinary implements encoding.BinaryMarshaler. The error is always
// nil.
func (s *Sketch) MarshalBinary() ([]byte, error) {
	return s.AppendBinary(make([]byte, 0, headerSize+len(s.regs)))
}

// AppendBinary implements encoding.BinaryAppender. The error is always
// nil.
func (s *Sketch) AppendBinary(b []byte) ([]byte, error) {
	b = append(b, formatVersion, algoHLL, s.p)
	return append(b, s.regs...), nil
}

// UnmarshalBinary implements encoding.BinaryUnmarshaler, replacing the
// sketch's contents. It returns an error on an unknown version or
// algorithm, an out-of-range p, or a length that does not match p.
func (s *Sketch) UnmarshalBinary(data []byte) error {
	if len(data) < headerSize {
		return fmt.Errorf("hll: truncated sketch: %d bytes", len(data))
	}
	if data[0] != formatVersion {
		return fmt.Errorf("hll: unknown sketch format version %d", data[0])
	}
	if data[1] != algoHLL {
		return fmt.Errorf("hll: unknown sketch algorithm id %d", data[1])
	}
	p := data[2]
	if p < minP || p > maxP {
		return fmt.Errorf("hll: invalid precision %d", p)
	}
	if want := headerSize + 1<<p; len(data) != want {
		return fmt.Errorf("hll: sketch with p=%d must be %d bytes, got %d", p, want, len(data))
	}
	s.p = p
	s.regs = append(s.regs[:0], data[headerSize:]...)
	return nil
}
