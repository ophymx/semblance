package semblance_test

import (
	"math"
	"slices"
	"testing"

	semblance "github.com/ophymx/semblance"
	"github.com/ophymx/semblance/minhash"
)

func TestDefaults(t *testing.T) {
	// The default configuration is frozen: changing any of these values
	// breaks comparability of stored default-config signatures and is a
	// major-version event.
	cfg := semblance.Defaults()
	want := semblance.Config{W: 3, K: 128, Seed: 0, Bands: 16, Rows: 8}
	if cfg != want {
		t.Fatalf("Defaults() = %+v, want %+v", cfg, want)
	}
}

func TestSimilarity(t *testing.T) {
	tests := []struct {
		name     string
		a, b     string
		min, max float64
	}{
		{"identical", "the quick brown fox jumps over the lazy dog", "the quick brown fox jumps over the lazy dog", 1, 1},
		{"case and punctuation invariant", "The Quick Brown Fox, jumps!", "the quick brown fox jumps", 1, 1},
		{"similar", "the quick brown fox jumps over the lazy dog", "the quick brown fox leaps over the lazy dog", 0.2, 0.7},
		{"unrelated", "the quick brown fox jumps over the lazy dog", "entirely different words about sketching libraries in go", 0, 0.05},
		{"degenerate both", "hi", "hi", 0, 0},
		{"degenerate one side", "hi there", "the quick brown fox jumps over it", 0, 0},
		{"both empty", "", "", 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := semblance.Similarity(tt.a, tt.b)
			if got < tt.min || got > tt.max {
				t.Errorf("Similarity = %v, want in [%v, %v]", got, tt.min, tt.max)
			}
		})
	}
}

func TestNewSketcherPanics(t *testing.T) {
	tests := []struct {
		name string
		cfg  semblance.Config
	}{
		{"zero value", semblance.Config{}},
		{"W=0", semblance.Config{W: 0, K: 128}},
		{"K=0", semblance.Config{W: 3, K: 0}},
		{"bands without rows", semblance.Config{W: 3, K: 128, Bands: 16}},
		{"bands*rows != k", semblance.Config{W: 3, K: 128, Bands: 16, Rows: 4}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			defer func() {
				if recover() == nil {
					t.Error("expected panic, got none")
				}
			}()
			semblance.NewSketcher(tt.cfg)
		})
	}
}

func TestSketcherWithoutLSH(t *testing.T) {
	// Bands/Rows may be left zero when no index is needed...
	sk := semblance.NewSketcher(semblance.Config{W: 3, K: 64, Seed: 7})
	if got := sk.Similarity("one two three four", "one two three four"); got != 1 {
		t.Errorf("Similarity = %v, want 1", got)
	}
	// ...but NewIndex then panics.
	defer func() {
		if recover() == nil {
			t.Error("NewIndex without Bands/Rows did not panic")
		}
	}()
	sk.NewIndex()
}

func TestSketchMatchesSketchInto(t *testing.T) {
	sk := semblance.NewSketcher(semblance.Defaults())
	text := "the quick brown fox jumps over the lazy dog"
	dst := make(minhash.Signature, semblance.Defaults().K)
	sk.SketchInto(dst, text)
	if !slices.Equal(dst, sk.Sketch(text)) {
		t.Error("SketchInto and Sketch disagree")
	}
}

func TestEmptySignature(t *testing.T) {
	sk := semblance.NewSketcher(semblance.Defaults())
	sig := sk.Sketch("too short")
	for _, v := range sig {
		if v != math.MaxUint64 {
			t.Fatalf("short text signature slot = %#x, want MaxUint64", v)
		}
	}
}

func TestPipelineWithIndex(t *testing.T) {
	sk := semblance.NewSketcher(semblance.Defaults())
	ix := sk.NewIndex()
	if got, want := ix.Threshold(), math.Pow(1.0/16, 1.0/8); math.Abs(got-want) > 1e-12 {
		t.Fatalf("Threshold = %v, want %v", got, want)
	}

	base := "the quick brown fox jumps over the lazy dog and keeps on running through the quiet green field all day"
	near := "the quick brown fox jumps over the lazy dog and keeps on running through the quiet green field all night"
	far := "unrelated content discussing minhash banding indexes and candidate verification strategies at length here"
	ix.Add("near", sk.Sketch(near))
	ix.Add("far", sk.Sketch(far))

	got := ix.Query(sk.Sketch(base))
	if !slices.Contains(got, "near") {
		t.Errorf("Query missed near-duplicate: %v", got)
	}
	if slices.Contains(got, "far") {
		t.Errorf("Query returned unrelated document: %v", got)
	}
}
