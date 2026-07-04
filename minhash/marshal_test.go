package minhash_test

import (
	"bytes"
	"encoding/hex"
	"slices"
	"strings"
	"testing"

	"github.com/ophymx/semblance/internal/hashutil"
	"github.com/ophymx/semblance/minhash"
	"github.com/ophymx/semblance/shingle"
)

func TestMarshalRoundTrip(t *testing.T) {
	tests := []struct {
		name string
		k    int
		seed uint64
		text string
	}{
		{"defaults", 128, 0, "the quick brown fox jumps over the lazy dog"},
		{"small k", 4, 42, "one two three four five"},
		{"empty text", 16, 7, ""},
		{"max seed", 8, ^uint64(0), "some words to sketch here"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := minhash.New(tt.k, tt.seed)
			sig := m.Sketch(shingle.Words(tt.text, 2))
			data := m.MarshalSignature(sig)

			if want := 12 + 8*tt.k; len(data) != want {
				t.Fatalf("marshaled length %d, want %d", len(data), want)
			}

			got, err := m.UnmarshalSignature(data)
			if err != nil {
				t.Fatalf("UnmarshalSignature: %v", err)
			}
			if !slices.Equal(got, sig) {
				t.Error("hasher round-trip signature mismatch")
			}

			dec, seed, err := minhash.DecodeSignature(data)
			if err != nil {
				t.Fatalf("DecodeSignature: %v", err)
			}
			if !slices.Equal(dec, sig) || seed != tt.seed {
				t.Errorf("DecodeSignature = (%v, %#x), want (sig, %#x)", dec, seed, tt.seed)
			}

			// A hasher reconstructed from decoded parameters produces
			// comparable signatures: the self-describing property.
			m2 := minhash.New(len(dec), seed)
			if !slices.Equal(m2.Sketch(shingle.Words(tt.text, 2)), sig) {
				t.Error("hasher reconstructed from decoded parameters disagrees")
			}
		})
	}
}

// TestMarshalGoldenBytes pins the wire format with hand-written expected
// bytes: version 01, algo 01, k=4 LE, seed=42 LE, then the four values LE.
// A failure here means the serialization layout changed — a major-version
// event.
func TestMarshalGoldenBytes(t *testing.T) {
	m := minhash.New(4, 42)
	sig := minhash.Signature{1, 2, 0xDEADBEEF, ^uint64(0)}
	want := "0101" + "0400" + "2a00000000000000" +
		"0100000000000000" + "0200000000000000" + "efbeadde00000000" + "ffffffffffffffff"
	if got := hex.EncodeToString(m.MarshalSignature(sig)); got != want {
		t.Errorf("wire bytes\n got %s\nwant %s", got, want)
	}
}

func TestMarshalPanicsOnLengthMismatch(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("MarshalSignature with wrong length did not panic")
		}
	}()
	minhash.New(4, 0).MarshalSignature(make(minhash.Signature, 3))
}

func TestDecodeErrors(t *testing.T) {
	m := minhash.New(4, 42)
	good := m.MarshalSignature(minhash.Signature{1, 2, 3, 4})

	corrupt := func(mutate func(b []byte) []byte) []byte {
		return mutate(bytes.Clone(good))
	}
	tests := []struct {
		name    string
		data    []byte
		wantErr string
	}{
		{"empty", nil, "truncated"},
		{"truncated header", good[:11], "truncated"},
		{"truncated values", good[:len(good)-1], "must be"},
		{"trailing bytes", append(bytes.Clone(good), 0), "must be"},
		{"unknown version", corrupt(func(b []byte) []byte { b[0] = 2; return b }), "version"},
		{"unknown algo", corrupt(func(b []byte) []byte { b[1] = 9; return b }), "algorithm"},
		{"zero k", corrupt(func(b []byte) []byte { b[2], b[3] = 0, 0; return b[:12] }), "length 0"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, _, err := minhash.DecodeSignature(tt.data); err == nil {
				t.Error("DecodeSignature succeeded on malformed input")
			} else if !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("error %q does not mention %q", err, tt.wantErr)
			}
		})
	}
}

func TestUnmarshalParameterMismatch(t *testing.T) {
	sig := minhash.Signature{1, 2, 3, 4}
	data := minhash.New(4, 42).MarshalSignature(sig)

	if _, err := minhash.New(4, 43).UnmarshalSignature(data); err == nil || !strings.Contains(err.Error(), "seed") {
		t.Errorf("seed mismatch not rejected: %v", err)
	}
	if _, err := minhash.New(8, 42).UnmarshalSignature(data); err == nil || !strings.Contains(err.Error(), "k=") {
		t.Errorf("k mismatch not rejected: %v", err)
	}
	if _, err := minhash.New(4, 42).UnmarshalSignature(data); err != nil {
		t.Errorf("matching hasher rejected: %v", err)
	}
}

func TestNewPanicsOnHugeK(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Error("New(65536, 0) did not panic")
		}
	}()
	minhash.New(65536, 0)
}

func FuzzDecodeSignature(f *testing.F) {
	m := minhash.New(4, 42)
	f.Add(m.MarshalSignature(minhash.Signature{1, 2, 3, 4}))
	f.Add([]byte{1, 1, 0, 0})
	f.Add([]byte(nil))
	f.Fuzz(func(t *testing.T, data []byte) {
		sig, seed, err := minhash.DecodeSignature(data)
		if err != nil {
			return
		}
		// Anything that decodes must re-marshal to the identical bytes.
		rt := minhash.New(len(sig), seed).MarshalSignature(sig)
		if !bytes.Equal(rt, data) {
			t.Fatalf("re-marshal mismatch:\n got %x\nwant %x", rt, data)
		}
	})
}

func TestUnionAfterDecode(t *testing.T) {
	// Signatures survive a marshal round-trip as working signatures, not
	// just equal values: Union and Jaccard still behave.
	rng := hashutil.SplitMix64(9)
	elems := make([]uint64, 100)
	for i := range elems {
		elems[i] = rng.Next()
	}
	m := minhash.New(16, 5)
	a := m.Sketch(slices.Values(elems[:60]))
	b := m.Sketch(slices.Values(elems[40:]))

	a2, err := m.UnmarshalSignature(m.MarshalSignature(a))
	if err != nil {
		t.Fatal(err)
	}
	u := make(minhash.Signature, 16)
	minhash.Union(u, a2, b)
	if got := m.Sketch(slices.Values(elems)); !slices.Equal(u, got) {
		t.Error("Union of decoded signature diverges from direct sketch")
	}
}
