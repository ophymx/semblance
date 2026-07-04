package minhash_test

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ophymx/semblance/minhash"
	"github.com/ophymx/semblance/shingle"
)

var update = flag.Bool("update", false, "rewrite golden testdata")

// Golden signatures freeze the full pipeline: tokenization, shingle mixing,
// permutation parameters, and sketching. A change here means stored
// signatures are broken — it must be a deliberate, reviewed, major-version
// event, never a casual test update.

const goldenPath = "testdata/golden.json"

type goldenCase struct {
	Name      string   `json:"name"`
	Text      string   `json:"text"`
	Mode      string   `json:"mode"` // "words" or "char"
	N         int      `json:"n"`    // w or k for the shingler
	K         int      `json:"k"`
	Seed      uint64   `json:"seed"`
	Signature []string `json:"signature"` // hex, one per slot
}

func computeSignature(tc goldenCase) minhash.Signature {
	m := minhash.New(tc.K, tc.Seed)
	switch tc.Mode {
	case "words":
		return m.Sketch(shingle.Words(tc.Text, tc.N))
	case "char":
		return m.Sketch(shingle.Char(tc.Text, tc.N))
	default:
		panic("unknown mode " + tc.Mode)
	}
}

func TestGoldenSignatures(t *testing.T) {
	cases := []goldenCase{
		{Name: "words basic", Text: "The quick brown fox jumps over the lazy dog", Mode: "words", N: 3, K: 16, Seed: 0},
		{Name: "words seed 1", Text: "The quick brown fox jumps over the lazy dog", Mode: "words", N: 3, K: 16, Seed: 1},
		{Name: "words ascii mixed case", Text: "Deterministic, Heuristic, FAST text similarity sketching for Go 1.25", Mode: "words", N: 2, K: 16, Seed: 0},
		{Name: "char basic", Text: "the quick brown fox", Mode: "char", N: 4, K: 16, Seed: 0},
		{Name: "char stable non-ascii", Text: "résumé façade naïve", Mode: "char", N: 3, K: 16, Seed: 0},
		{Name: "empty", Text: "", Mode: "words", N: 3, K: 16, Seed: 0},
	}

	if *update {
		for i := range cases {
			sig := computeSignature(cases[i])
			cases[i].Signature = make([]string, len(sig))
			for j, v := range sig {
				cases[i].Signature[j] = fmt.Sprintf("%016x", v)
			}
		}
		buf, err := json.MarshalIndent(cases, "", "  ")
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Dir(goldenPath), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(goldenPath, append(buf, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("rewrote %s", goldenPath)
		return
	}

	buf, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden file (run with -update to generate): %v", err)
	}
	var want []goldenCase
	if err := json.Unmarshal(buf, &want); err != nil {
		t.Fatal(err)
	}
	if len(want) != len(cases) {
		t.Fatalf("golden file has %d cases, test defines %d (run -update after review)", len(want), len(cases))
	}
	for i, tc := range cases {
		t.Run(tc.Name, func(t *testing.T) {
			g := want[i]
			if g.Name != tc.Name || g.Text != tc.Text || g.Mode != tc.Mode || g.N != tc.N || g.K != tc.K || g.Seed != tc.Seed {
				t.Fatalf("golden case %d parameters diverge from test definition; run -update after review", i)
			}
			sig := computeSignature(tc)
			if len(sig) != len(g.Signature) {
				t.Fatalf("signature length %d, golden %d", len(sig), len(g.Signature))
			}
			for j, v := range sig {
				if got := fmt.Sprintf("%016x", v); got != g.Signature[j] {
					t.Errorf("slot %d = %s, golden %s — stored signatures are BROKEN if this is intentional", j, got, g.Signature[j])
				}
			}
		})
	}
}
