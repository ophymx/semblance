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

// collectBlocks drains WordsBlocks through a given block size.
func collectBlocks(text string, w, blockSize int) []uint64 {
	var out []uint64
	block := make([]uint64, blockSize)
	shingle.WordsBlocks(text, w, block, func(hashes []uint64) bool {
		out = append(out, hashes...)
		return true
	})
	return out
}

// TestWordsBlocksMatchesWords verifies the fused path emits exactly the
// Words hash sequence for every block size, including sizes that force
// mid-scan flushes.
func TestWordsBlocksMatchesWords(t *testing.T) {
	texts := []string{
		"",
		"one",
		"the quick brown fox jumps over the lazy dog and keeps going",
		"HÉLLO wörld — mixed «script» tokens 123abc \xff\xfe",
	}
	for _, text := range texts {
		for _, w := range []int{1, 2, 3} {
			want := slices.Collect(shingle.Words(text, w))
			for _, bs := range []int{1, 2, 3, 7, 255, 256} {
				if got := collectBlocks(text, w, bs); !slices.Equal(got, want) {
					t.Errorf("WordsBlocks(%q, w=%d, block=%d) diverges from Words", text, w, bs)
				}
			}
		}
	}
	// Bytes variant.
	text := "the quick brown fox jumps over the lazy dog"
	var got []uint64
	block := make([]uint64, 4)
	shingle.WordsBlocksBytes([]byte(text), 2, block, func(h []uint64) bool {
		got = append(got, h...)
		return true
	})
	if !slices.Equal(got, slices.Collect(shingle.Words(text, 2))) {
		t.Error("WordsBlocksBytes diverges from Words")
	}
}

func TestWordsBlocksEarlyStop(t *testing.T) {
	calls := 0
	block := make([]uint64, 2)
	shingle.WordsBlocks("one two three four five six seven eight", 1, block, func(h []uint64) bool {
		calls++
		return false
	})
	if calls != 1 {
		t.Fatalf("flush called %d times after returning false, want 1", calls)
	}
}

func TestWordsBlocksPanics(t *testing.T) {
	for name, fn := range map[string]func(){
		"w=0":         func() { shingle.WordsBlocks("abc", 0, make([]uint64, 4), nil) },
		"empty block": func() { shingle.WordsBlocks("abc", 1, nil, nil) },
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
