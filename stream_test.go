package semblance_test

import (
	"io"
	"slices"
	"strings"
	"testing"

	semblance "github.com/ophymx/semblance"
)

func TestStreamMatchesSketch(t *testing.T) {
	sk := semblance.NewSketcher(semblance.Defaults())
	text := "the quick brown fox jumps over the lazy dog — naïve façade " +
		strings.Repeat("and keeps running through the quiet field ", 50)
	want := sk.Sketch(text)

	for _, chunk := range []int{1, 3, 17, 1 << 20} {
		st := sk.NewStream()
		for i := 0; i < len(text); i += chunk {
			st.WriteString(text[i:min(i+chunk, len(text))])
		}
		if !slices.Equal(st.Signature(), want) {
			t.Errorf("chunk=%d: Stream signature diverges from Sketch", chunk)
		}
	}

	// io.Copy path (exercises Write via io.Reader plumbing).
	st := sk.NewStream()
	if _, err := io.Copy(st, strings.NewReader(text)); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(st.Signature(), want) {
		t.Error("io.Copy stream diverges from Sketch")
	}
}

func TestStreamLifecycle(t *testing.T) {
	sk := semblance.NewSketcher(semblance.Defaults())
	st := sk.NewStream()
	st.WriteString("the quick brown fox jumps over the lazy dog")

	// Signature is idempotent and returns copies.
	a := st.Signature()
	b := st.Signature()
	if !slices.Equal(a, b) {
		t.Fatal("repeated Signature calls disagree")
	}
	a[0] = 42
	if st.Signature()[0] == 42 {
		t.Fatal("Signature returned a live reference, not a copy")
	}

	// Writing after finalization panics; Reset recovers.
	func() {
		defer func() {
			if recover() == nil {
				t.Error("Write after Signature did not panic")
			}
		}()
		st.WriteString("more text")
	}()
	st.Reset()
	st.WriteString("an entirely different document about hashing")
	if slices.Equal(st.Signature(), b) {
		t.Error("Reset did not clear the previous document")
	}

	// Reuse produces identical signatures for identical documents.
	st.Reset()
	st.WriteString("the quick brown fox jumps over the lazy dog")
	if !slices.Equal(st.Signature(), b) {
		t.Error("reused Stream diverges from its first run")
	}
}

func TestStreamEmpty(t *testing.T) {
	sk := semblance.NewSketcher(semblance.Defaults())
	sig := sk.NewStream().Signature()
	if !slices.Equal(sig, sk.Sketch("")) {
		t.Error("empty Stream diverges from Sketch of empty text")
	}
}

// TestStreamWriteAllocs pins that steady-state writes (chunks ending at a
// token boundary) allocate nothing.
func TestStreamWriteAllocs(t *testing.T) {
	sk := semblance.NewSketcher(semblance.Defaults())
	st := sk.NewStream()
	chunk := []byte("the quick brown fox jumps over the lazy dog ")
	if allocs := testing.AllocsPerRun(100, func() { st.Write(chunk) }); allocs != 0 {
		t.Errorf("steady-state Write: %v allocs/op, want 0", allocs)
	}
}
