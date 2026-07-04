package shingle_test

import (
	"slices"
	"testing"

	"github.com/ophymx/semblance/internal/hashutil"
	"github.com/ophymx/semblance/shingle"
)

// scanChunked feeds text to a WordScanner in the given chunk sizes
// (cycling) and returns the emitted hashes.
func scanChunked(text string, w, blockSize int, chunks []int) []uint64 {
	var out []uint64
	sc := shingle.NewWordScanner(w, make([]uint64, blockSize), func(h []uint64) bool {
		out = append(out, h...)
		return true
	})
	for i, c := 0, 0; i < len(text); c++ {
		n := min(chunks[c%len(chunks)], len(text)-i)
		if n < 1 {
			n = 1
		}
		// Alternate Write and WriteString to cover both entry points.
		if c%2 == 0 {
			sc.WriteString(text[i : i+n])
		} else {
			sc.Write([]byte(text[i : i+n]))
		}
		i += n
	}
	sc.Finish()
	return out
}

func TestWordScannerMatchesWords(t *testing.T) {
	texts := []string{
		"",
		"one",
		"the quick brown fox jumps over the lazy dog and keeps on running",
		"HÉLLO wörld — mixed «script» tokens 123abc naïve façade",
		"\xff\xfe bad \xc3 utf8 \x80 bytes interleaved with words",
		"nospacesatallinthisverylongtokenthatspansmanychunks",
		"Ĝis— la—revido",
	}
	chunkings := [][]int{{1}, {2}, {3}, {7}, {64}, {1 << 20}, {1, 5, 2}}
	for _, text := range texts {
		for _, w := range []int{1, 2, 3} {
			want := slices.Collect(shingle.Words(text, w))
			for _, chunks := range chunkings {
				if got := scanChunked(text, w, 4, chunks); !slices.Equal(got, want) {
					t.Errorf("WordScanner(%q, w=%d, chunks=%v) diverges from Words", text, w, chunks)
				}
			}
		}
	}
}

func TestWordScannerReuse(t *testing.T) {
	var out []uint64
	sc := shingle.NewWordScanner(2, make([]uint64, 8), func(h []uint64) bool {
		out = append(out, h...)
		return true
	})
	for range 3 {
		out = out[:0]
		sc.WriteString("the quick brown fox")
		sc.Finish()
		if want := slices.Collect(shingle.Words("the quick brown fox", 2)); !slices.Equal(out, want) {
			t.Fatal("reused scanner diverges from Words")
		}
		sc.Reset()
	}
}

func TestWordScannerEarlyStop(t *testing.T) {
	calls := 0
	sc := shingle.NewWordScanner(1, make([]uint64, 2), func(h []uint64) bool {
		calls++
		return false
	})
	sc.WriteString("one two three four five six")
	sc.WriteString("seven eight nine ten")
	sc.Finish()
	if calls != 1 {
		t.Fatalf("flush called %d times after returning false, want 1", calls)
	}
}

func TestWordScannerLifecyclePanics(t *testing.T) {
	discard := func([]uint64) bool { return true }
	newScanner := func() *shingle.WordScanner {
		return shingle.NewWordScanner(1, make([]uint64, 2), discard)
	}
	tests := map[string]func(){
		"write after finish": func() { sc := newScanner(); sc.Finish(); sc.WriteString("x") },
		"double finish":      func() { sc := newScanner(); sc.Finish(); sc.Finish() },
		"nil flush":          func() { shingle.NewWordScanner(1, make([]uint64, 2), nil) },
		"w=0":                func() { shingle.NewWordScanner(0, make([]uint64, 2), discard) },
		"empty block":        func() { shingle.NewWordScanner(1, nil, discard) },
	}
	for name, fn := range tests {
		t.Run(name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("expected panic, got none")
				}
			}()
			fn()
		})
	}

	// Reset after Finish permits writing again.
	sc := newScanner()
	sc.Finish()
	sc.Reset()
	sc.WriteString("fine again")
	sc.Finish()
}

// FuzzWordScanner cross-checks arbitrary chunkings against Words: the
// chunk sizes are derived from a fuzzed seed, so splits land inside
// tokens and multi-byte runes.
func FuzzWordScanner(f *testing.F) {
	f.Add("the quick brown fox — jumps über the lazy dog", 3, uint64(42))
	f.Add("\xff\xfe a\xffb", 1, uint64(7))
	f.Fuzz(func(t *testing.T, text string, w int, seed uint64) {
		if w <= 0 || w > 64 {
			t.Skip()
		}
		rng := hashutil.SplitMix64(seed)
		var got []uint64
		sc := shingle.NewWordScanner(w, make([]uint64, 3), func(h []uint64) bool {
			got = append(got, h...)
			return true
		})
		for i := 0; i < len(text); {
			n := int(rng.Next()%7) + 1
			n = min(n, len(text)-i)
			sc.WriteString(text[i : i+n])
			i += n
		}
		sc.Finish()
		if !slices.Equal(got, slices.Collect(shingle.Words(text, w))) {
			t.Fatal("chunked WordScanner diverges from Words")
		}
	})
}
