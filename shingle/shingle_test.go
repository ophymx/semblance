package shingle_test

import (
	"slices"
	"testing"

	"github.com/cespare/xxhash/v2"

	"github.com/ophymx/semblance/internal/hashutil"
	"github.com/ophymx/semblance/shingle"
)

// hashWindows is the test oracle for char shingles: xxhash of each expected
// window substring.
func hashWindows(windows ...string) []uint64 {
	out := make([]uint64, len(windows))
	for i, w := range windows {
		out[i] = xxhash.Sum64String(w)
	}
	return out
}

// hashShingles is the test oracle for word shingles: xxhash each token,
// fold each w-window with the frozen mixing function.
func hashShingles(w int, tokens ...string) []uint64 {
	var out []uint64
	for i := 0; i+w <= len(tokens); i++ {
		acc := hashutil.MixInit
		for _, tok := range tokens[i : i+w] {
			acc = hashutil.Mix(acc, xxhash.Sum64String(tok))
		}
		out = append(out, acc)
	}
	return out
}

func TestChar(t *testing.T) {
	tests := []struct {
		name string
		text string
		k    int
		want []uint64
	}{
		{"basic", "abcd", 2, hashWindows("ab", "bc", "cd")},
		{"exact length", "abc", 3, hashWindows("abc")},
		{"shorter than k", "ab", 3, nil},
		{"empty", "", 1, nil},
		{"k=1", "abc", 1, hashWindows("a", "b", "c")},
		{"bytes not runes", "héllo", 3, hashWindows("h\xc3\xa9", "\xc3\xa9l", "\xa9ll", "llo")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := slices.Collect(shingle.Char(tt.text, tt.k))
			if !slices.Equal(got, tt.want) {
				t.Errorf("Char(%q, %d) = %v, want %v", tt.text, tt.k, got, tt.want)
			}
		})
	}
}

func TestCharRunes(t *testing.T) {
	tests := []struct {
		name string
		text string
		k    int
		want []uint64
	}{
		{"ascii", "abcd", 2, hashWindows("ab", "bc", "cd")},
		{"multibyte", "héllo", 2, hashWindows("hé", "él", "ll", "lo")},
		{"shorter than k", "hé", 3, nil},
		{"invalid utf8", "\xffab", 2, hashWindows("\xffa", "ab")},
		{"empty", "", 2, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := slices.Collect(shingle.CharRunes(tt.text, tt.k))
			if !slices.Equal(got, tt.want) {
				t.Errorf("CharRunes(%q, %d) = %v, want %v", tt.text, tt.k, got, tt.want)
			}
		})
	}
}

func TestWords(t *testing.T) {
	tests := []struct {
		name string
		text string
		w    int
		want []uint64
	}{
		{"single words", "Hello, World!", 1, hashShingles(1, "hello", "world")},
		{"bigram", "Hello, World!", 2, hashShingles(2, "hello", "world")},
		{"trigrams", "one two three four", 3, hashShingles(3, "one", "two", "three", "four")},
		{"fewer than w", "one two", 3, nil},
		{"no tokens", "!!! ... ---", 2, nil},
		{"empty", "", 1, nil},
		{"digits in tokens", "abc123 4def", 2, hashShingles(2, "abc123", "4def")},
		{"unicode letters", "Ĝis la revido", 2, hashShingles(2, "ĝis", "la", "revido")},
		{"punctuation splits", "don't stop", 2, hashShingles(2, "don", "t", "stop")},
		{"underscore splits", "foo_bar", 1, hashShingles(1, "foo", "bar")},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := slices.Collect(shingle.Words(tt.text, tt.w))
			if !slices.Equal(got, tt.want) {
				t.Errorf("Words(%q, %d) = %v, want %v", tt.text, tt.w, got, tt.want)
			}
		})
	}
}

func TestWordsCaseInsensitive(t *testing.T) {
	tests := []struct{ a, b string }{
		{"Hello World", "hello world"},
		{"HÉLLO ÖST", "héllo öst"},
	}
	for _, tt := range tests {
		a := slices.Collect(shingle.Words(tt.a, 2))
		b := slices.Collect(shingle.Words(tt.b, 2))
		if !slices.Equal(a, b) {
			t.Errorf("Words(%q) != Words(%q)", tt.a, tt.b)
		}
	}
}

func TestWordsOrderSensitive(t *testing.T) {
	abc := slices.Collect(shingle.Words("aa bb cc", 3))
	cba := slices.Collect(shingle.Words("cc bb aa", 3))
	if slices.Equal(abc, cba) {
		t.Fatal("word shingle of reversed token order collided")
	}
}

func TestEarlyStop(t *testing.T) {
	// Breaking out of iteration must not panic or misbehave.
	for range shingle.Char("abcdef", 2) {
		break
	}
	for range shingle.CharRunes("abcdef", 2) {
		break
	}
	got := 0
	for range shingle.Words("one two three four five", 2) {
		got++
		if got == 2 {
			break
		}
	}
	if got != 2 {
		t.Fatalf("early stop consumed %d shingles, want 2", got)
	}
}

func TestPanics(t *testing.T) {
	tests := []struct {
		name string
		fn   func()
	}{
		{"Char k=0", func() { shingle.Char("abc", 0) }},
		{"Char k=-1", func() { shingle.Char("abc", -1) }},
		{"CharRunes k=0", func() { shingle.CharRunes("abc", 0) }},
		{"Words w=0", func() { shingle.Words("abc", 0) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("expected panic, got none")
				}
			}()
			tt.fn()
		})
	}
}

func FuzzChar(f *testing.F) {
	f.Add("hello world", 4)
	f.Add("", 1)
	f.Add("héllo wörld", 3)
	f.Add("\xff\xfe\xfd", 2)
	f.Fuzz(func(t *testing.T, text string, k int) {
		if k <= 0 || k > 64 {
			t.Skip()
		}
		a := slices.Collect(shingle.Char(text, k))
		b := slices.Collect(shingle.Char(text, k))
		if !slices.Equal(a, b) {
			t.Fatal("Char is nondeterministic")
		}
		a = slices.Collect(shingle.CharRunes(text, k))
		b = slices.Collect(shingle.CharRunes(text, k))
		if !slices.Equal(a, b) {
			t.Fatal("CharRunes is nondeterministic")
		}
	})
}

func FuzzWords(f *testing.F) {
	f.Add("hello world how are you", 3)
	f.Add("", 1)
	f.Add("Ĝis la revido — ĝis!", 2)
	f.Add("\xff\xfe a\xffb", 1)
	f.Fuzz(func(t *testing.T, text string, w int) {
		if w <= 0 || w > 64 {
			t.Skip()
		}
		a := slices.Collect(shingle.Words(text, w))
		b := slices.Collect(shingle.Words(text, w))
		if !slices.Equal(a, b) {
			t.Fatal("Words is nondeterministic")
		}
	})
}

// FuzzBytesEquivalence cross-checks every *Bytes variant against its
// string counterpart on arbitrary inputs.
func FuzzBytesEquivalence(f *testing.F) {
	f.Add("hello world", 3)
	f.Add("\xff\xfe a\xffb", 2)
	f.Fuzz(func(t *testing.T, text string, n int) {
		if n <= 0 || n > 64 {
			t.Skip()
		}
		b := []byte(text)
		if !slices.Equal(slices.Collect(shingle.CharBytes(b, n)), slices.Collect(shingle.Char(text, n))) {
			t.Fatal("CharBytes diverges from Char")
		}
		if !slices.Equal(slices.Collect(shingle.CharRunesBytes(b, n)), slices.Collect(shingle.CharRunes(text, n))) {
			t.Fatal("CharRunesBytes diverges from CharRunes")
		}
		if !slices.Equal(slices.Collect(shingle.WordsBytes(b, n)), slices.Collect(shingle.Words(text, n))) {
			t.Fatal("WordsBytes diverges from Words")
		}
	})
}

// FuzzWordsBlocks cross-checks the fused block path against Words on
// arbitrary inputs and block sizes.
func FuzzWordsBlocks(f *testing.F) {
	f.Add("hello world how are you", 3, 4)
	f.Add("\xff\xfe a\xffb", 1, 1)
	f.Fuzz(func(t *testing.T, text string, w int, blockSize int) {
		if w <= 0 || w > 64 || blockSize <= 0 || blockSize > 1024 {
			t.Skip()
		}
		var got []uint64
		shingle.WordsBlocks(text, w, make([]uint64, blockSize), func(h []uint64) bool {
			got = append(got, h...)
			return true
		})
		if !slices.Equal(got, slices.Collect(shingle.Words(text, w))) {
			t.Fatal("WordsBlocks diverges from Words")
		}
	})
}
