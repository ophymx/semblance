package shingle_test

import (
	"slices"
	"testing"

	"github.com/ophymx/semblance/shingle"
)

// TestBytesVariantsMatch verifies every *Bytes variant produces the exact
// hash stream of its string counterpart.
func TestBytesVariantsMatch(t *testing.T) {
	inputs := []string{
		"",
		"the quick brown fox jumps over the lazy dog",
		"Deterministic, Heuristic, FAST sketching for Go 1.25!",
		"Ĝis la revido — ĝis!",
		"\xff\xfe invalid \xc3 utf8 \x80 bytes",
		"HÉLLO ÖST 123abc",
	}
	for _, text := range inputs {
		b := []byte(text)
		if got, want := slices.Collect(shingle.CharBytes(b, 4)), slices.Collect(shingle.Char(text, 4)); !slices.Equal(got, want) {
			t.Errorf("CharBytes(%q) diverges from Char", text)
		}
		if got, want := slices.Collect(shingle.CharRunesBytes(b, 3)), slices.Collect(shingle.CharRunes(text, 3)); !slices.Equal(got, want) {
			t.Errorf("CharRunesBytes(%q) diverges from CharRunes", text)
		}
		if got, want := slices.Collect(shingle.WordsBytes(b, 2)), slices.Collect(shingle.Words(text, 2)); !slices.Equal(got, want) {
			t.Errorf("WordsBytes(%q) diverges from Words", text)
		}
	}
}

func TestBytesVariantsPanics(t *testing.T) {
	for name, fn := range map[string]func(){
		"CharBytes":      func() { shingle.CharBytes([]byte("abc"), 0) },
		"CharRunesBytes": func() { shingle.CharRunesBytes([]byte("abc"), 0) },
		"WordsBytes":     func() { shingle.WordsBytes([]byte("abc"), 0) },
	} {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("expected panic, got none")
				}
			}()
			fn()
		})
	}
}

// TestBytesNoExtraAllocs pins that the zero-copy bridge adds no allocations
// over the string path.
func TestBytesNoExtraAllocs(t *testing.T) {
	text := "the quick brown fox jumps over the lazy dog"
	b := []byte(text)
	drain := func(seq func(func(uint64) bool)) {
		for range seq {
			_ = struct{}{}
		}
	}
	str := testing.AllocsPerRun(100, func() { drain(shingle.Words(text, 3)) })
	byt := testing.AllocsPerRun(100, func() { drain(shingle.WordsBytes(b, 3)) })
	if byt > str {
		t.Errorf("WordsBytes allocates more than Words: %v > %v", byt, str)
	}
}
