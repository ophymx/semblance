package simhash_test

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/ophymx/semblance/simhash"
)

var update = flag.Bool("update", false, "rewrite golden testdata")

// Golden fingerprints freeze the full pipeline: tokenization, shingle
// mixing, and the SimHash bit rule. A change here breaks stored
// fingerprints — a deliberate, reviewed, major-version event only.

const goldenPath = "testdata/golden.json"

type goldenCase struct {
	Name        string `json:"name"`
	Text        string `json:"text"`
	W           int    `json:"w"`
	Fingerprint string `json:"fingerprint"` // hex
}

func TestGoldenFingerprints(t *testing.T) {
	cases := []goldenCase{
		{Name: "basic", Text: "The quick brown fox jumps over the lazy dog", W: 3},
		{Name: "bigram", Text: "The quick brown fox jumps over the lazy dog", W: 2},
		{Name: "mixed case ascii", Text: "Deterministic, Heuristic, FAST text similarity sketching for Go 1.25", W: 2},
		{Name: "stable non-ascii", Text: "résumé façade naïve puzzle", W: 2},
		{Name: "empty", Text: "", W: 3},
	}

	if *update {
		for i := range cases {
			cases[i].Fingerprint = fmt.Sprintf("%016x", uint64(simhash.SketchText(cases[i].Text, cases[i].W)))
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
			if g.Name != tc.Name || g.Text != tc.Text || g.W != tc.W {
				t.Fatalf("golden case %d parameters diverge from test definition; run -update after review", i)
			}
			got := fmt.Sprintf("%016x", uint64(simhash.SketchText(tc.Text, tc.W)))
			if got != g.Fingerprint {
				t.Errorf("fingerprint = %s, golden %s — stored fingerprints are BROKEN if this is intentional", got, g.Fingerprint)
			}
		})
	}
}
