package simhash_test

import (
	"encoding"
	"encoding/hex"
	"strings"
	"testing"

	"github.com/ophymx/semblance/simhash"
)

var (
	_ encoding.BinaryMarshaler   = simhash.Fingerprint(0)
	_ encoding.BinaryAppender    = simhash.Fingerprint(0)
	_ encoding.BinaryUnmarshaler = (*simhash.Fingerprint)(nil)
)

func TestFingerprintRoundTrip(t *testing.T) {
	for _, fp := range []simhash.Fingerprint{0, 1, 0xDEADBEEFCAFEF00D, ^simhash.Fingerprint(0)} {
		data, err := fp.MarshalBinary()
		if err != nil {
			t.Fatalf("MarshalBinary(%#x): %v", fp, err)
		}
		var got simhash.Fingerprint
		if err := got.UnmarshalBinary(data); err != nil {
			t.Fatalf("UnmarshalBinary(%#x): %v", fp, err)
		}
		if got != fp {
			t.Errorf("round trip = %#x, want %#x", got, fp)
		}
	}
}

func TestFingerprintAppendBinary(t *testing.T) {
	prefix := []byte("prefix")
	data, err := simhash.Fingerprint(7).AppendBinary(prefix)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "prefix") || len(data) != len(prefix)+10 {
		t.Errorf("AppendBinary = %x, want prefix + 10 bytes", data)
	}
}

// TestFingerprintGoldenBytes pins the wire format with hand-written bytes:
// version 01, algo 02, then the value LE. A failure means the layout
// changed — a major-version event.
func TestFingerprintGoldenBytes(t *testing.T) {
	data, _ := simhash.Fingerprint(0xDEADBEEF).MarshalBinary()
	if got, want := hex.EncodeToString(data), "0102"+"efbeadde00000000"; got != want {
		t.Errorf("wire bytes = %s, want %s", got, want)
	}
}

func TestFingerprintUnmarshalErrors(t *testing.T) {
	good, _ := simhash.Fingerprint(1).MarshalBinary()
	tests := []struct {
		name    string
		data    []byte
		wantErr string
	}{
		{"empty", nil, "10 bytes"},
		{"short", good[:9], "10 bytes"},
		{"long", append(append([]byte{}, good...), 0), "10 bytes"},
		{"bad version", []byte{9, 2, 0, 0, 0, 0, 0, 0, 0, 0}, "version"},
		{"bad algo", []byte{1, 1, 0, 0, 0, 0, 0, 0, 0, 0}, "algorithm"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var fp simhash.Fingerprint
			if err := fp.UnmarshalBinary(tt.data); err == nil {
				t.Error("UnmarshalBinary succeeded on malformed input")
			} else if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not mention %q", err, tt.wantErr)
			}
		})
	}
}
