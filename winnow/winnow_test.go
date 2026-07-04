package winnow_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/cespare/xxhash/v2"

	"github.com/ophymx/semblance/internal/hashutil"
	"github.com/ophymx/semblance/winnow"
)

// naiveWinnow is the oracle: for every window, the rightmost minimum,
// deduplicated (selected positions are nondecreasing, so consecutive
// dedupe suffices).
func naiveWinnow(hashes []uint64, w int) []winnow.Fingerprint {
	var out []winnow.Fingerprint
	for start := 0; start+w <= len(hashes); start++ {
		mp := start
		for p := start + 1; p < start+w; p++ {
			if hashes[p] <= hashes[mp] {
				mp = p
			}
		}
		fp := winnow.Fingerprint{Pos: mp, Hash: hashes[mp]}
		if len(out) == 0 || out[len(out)-1] != fp {
			out = append(out, fp)
		}
	}
	return out
}

func TestHashesMatchesOracle(t *testing.T) {
	rng := hashutil.SplitMix64(111)
	for _, n := range []int{0, 1, 4, 5, 6, 100, 1000} {
		for _, w := range []int{1, 2, 5, 16} {
			hashes := make([]uint64, n)
			for i := range hashes {
				hashes[i] = rng.Next() % 64 // small range forces ties
			}
			got := slices.Collect(winnow.Hashes(slices.Values(hashes), w))
			want := naiveWinnow(hashes, w)
			if !slices.Equal(got, want) {
				t.Errorf("n=%d w=%d: Hashes diverges from oracle\n got %v\nwant %v", n, w, got, want)
			}
		}
	}
}

func TestTextPositions(t *testing.T) {
	const text = "the quick brown fox jumps over the lazy dog"
	const k = 5
	for fp := range winnow.Text(text, k, 4) {
		if want := xxhash.Sum64String(text[fp.Pos : fp.Pos+k]); fp.Hash != want {
			t.Errorf("fingerprint at %d: hash %#x is not the k-gram hash %#x", fp.Pos, fp.Hash, want)
		}
	}
}

func TestTextBytesMatchesText(t *testing.T) {
	const text = "winnowing fingerprints locate shared passages précisément"
	a := slices.Collect(winnow.Text(text, 4, 5))
	b := slices.Collect(winnow.TextBytes([]byte(text), 4, 5))
	if !slices.Equal(a, b) {
		t.Error("TextBytes diverges from Text")
	}
}

// TestGolden pins the frozen selection scheme; a change breaks stored
// fingerprints and is a major-version event.
func TestGolden(t *testing.T) {
	got := slices.Collect(winnow.Text("the quick brown fox", 4, 4))
	want := []string{
		"2:274f474d374b10e3",
		"5:079e5e25f2fca95a",
		"8:22ab2f80dfaf8a01",
		"12:337380428eae6a42",
	}
	var gotStr []string
	for _, fp := range got {
		gotStr = append(gotStr, fmt.Sprintf("%d:%016x", fp.Pos, fp.Hash))
	}
	if !slices.Equal(gotStr, want) {
		t.Errorf("golden mismatch:\n got %v\nwant %v", gotStr, want)
	}
}

// TestGuarantee: documents sharing a substring of >= w+k-1 bytes share a
// fingerprint hash.
func TestGuarantee(t *testing.T) {
	const k, w = 4, 5
	const shared = "this exact shared passage is long enough to guarantee a match"
	a := "prefix junk before! " + shared + " and unrelated tail A"
	b := "totally different opening……" + shared + " different ending B"

	hashesOf := func(text string) map[uint64]bool {
		set := map[uint64]bool{}
		for fp := range winnow.Text(text, k, w) {
			set[fp.Hash] = true
		}
		return set
	}
	ha, hb := hashesOf(a), hashesOf(b)
	common := 0
	for h := range ha {
		if hb[h] {
			common++
		}
	}
	if common == 0 {
		t.Fatal("documents sharing a long passage have no common fingerprints")
	}
}

func FuzzGuarantee(f *testing.F) {
	f.Add("prefix one ", "shared middle passage here", " suffix two")
	f.Fuzz(func(t *testing.T, pre, shared, suf string) {
		const k, w = 4, 5
		if len(shared) < w+k-1 {
			t.Skip()
		}
		// Bound the boundary effect: wrap shared with definite separators
		// so its k-grams appear intact in both documents.
		a := pre + " " + shared + " x"
		b := "y " + shared + " " + suf
		seen := map[uint64]bool{}
		for fp := range winnow.Text(a, k, w) {
			seen[fp.Hash] = true
		}
		for fp := range winnow.Text(b, k, w) {
			if seen[fp.Hash] {
				return // guarantee held
			}
		}
		t.Fatalf("no shared fingerprint for common passage of %d bytes", len(shared))
	})
}

func TestDensity(t *testing.T) {
	rng := hashutil.SplitMix64(112)
	hashes := make([]uint64, 10000)
	for i := range hashes {
		hashes[i] = rng.Next()
	}
	const w = 9
	n := 0
	for range winnow.Hashes(slices.Values(hashes), w) {
		n++
	}
	// Expected density 2/(w+1) = 0.2; allow wide slack.
	if n < 1500 || n > 2500 {
		t.Errorf("selected %d fingerprints from 10000 hashes, expected ~2000", n)
	}
}

func TestEarlyStopAndPanics(t *testing.T) {
	count := 0
	for range winnow.Text("the quick brown fox jumps over the lazy dog", 3, 3) {
		count++
		if count == 2 {
			break
		}
	}
	if count != 2 {
		t.Fatalf("early stop consumed %d fingerprints, want 2", count)
	}
	for name, fn := range map[string]func(){
		"w=0": func() { winnow.Hashes(slices.Values([]uint64{1}), 0) },
		"k=0": func() { winnow.Text("abc", 0, 3) },
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
