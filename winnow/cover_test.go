package winnow_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/ophymx/semblance/winnow"
)

// The mining corpus: five articles with unique bodies, all carrying the
// same footer, three also carrying an ad prelude.
const (
	coverAd     = "[ Support ASSTR - your donations fund the servers. Visit the donations page today! ]\n\n"
	coverFooter = "\n\n<1st Amendment publication - ASSM sponsored. Please read the ASSM FAQ before posting. Donations keep the archive alive.>\n"
)

var coverBodies = map[string]string{
	"a": "The lighthouse keeper counted the ships as they passed the narrow strait each morning.",
	"b": "Deep in the ravine the survey team found quartz veins nobody had mapped before today.",
	"c": "Her grandmother's recipe called for wild plums picked before the first frost of autumn.",
	"d": "The orchestra tuned in the empty hall while snow gathered on the skylight above them.",
	"e": "A cartographer by trade, he distrusted every map he had not walked over himself first.",
}

func coverCorpus() (*winnow.Index, map[string]string) {
	docs := map[string]string{}
	for id, body := range coverBodies {
		if id <= "c" {
			docs[id] = coverAd + body + coverFooter
		} else {
			docs[id] = body + coverFooter
		}
	}
	ix := winnow.NewIndex(idxK, idxW)
	for id, text := range docs {
		ix.Add(id, text)
	}
	return ix, docs
}

func TestDocFreqOracle(t *testing.T) {
	ix, docs := coverCorpus()
	oracle := map[uint64]int{}
	for _, text := range docs {
		for h := range fpHashes(text) {
			oracle[h]++
		}
	}
	for h, want := range oracle {
		if got := ix.DocFreq(h); got != want {
			t.Errorf("DocFreq(%#x) = %d, oracle %d", h, got, want)
		}
	}
	for h := range fpHashes("text that was never indexed anywhere in this corpus at all") {
		if oracle[h] != 0 {
			continue
		}
		if got := ix.DocFreq(h); got != 0 {
			t.Errorf("DocFreq of unindexed hash %#x = %d, want 0", h, got)
		}
	}
	// The footer separates from content by DF: some fingerprint reaches
	// DF=5, story fingerprints stay at DF=1.
	maxDF := 0
	for _, df := range oracle {
		maxDF = max(maxDF, df)
	}
	if maxDF != 5 {
		t.Errorf("max DF = %d, want 5 for a footer shared by all five docs", maxDF)
	}
}

func TestCoverFindsBoilerplate(t *testing.T) {
	ix, docs := coverCorpus()
	regions := ix.Cover(docs["a"], 3, 5)
	if len(regions) == 0 {
		t.Fatal("no regions found in a doc carrying an ad prelude and a footer")
	}

	var covered []string
	for _, r := range regions {
		covered = append(covered, docs["a"][r.Pos:r.Pos+r.Len])
	}
	joined := strings.Join(covered, "\x00")
	if !strings.Contains(joined, "donations fund the servers") {
		t.Errorf("ad prelude (DF=3) not covered; regions: %q", covered)
	}
	if !strings.Contains(joined, "ASSM FAQ") {
		t.Errorf("footer (DF=5) not covered; regions: %q", covered)
	}
	for _, word := range []string{"lighthouse", "keeper", "strait"} {
		if strings.Contains(joined, word) {
			t.Errorf("story content %q wrongly covered; regions: %q", word, covered)
		}
	}

	// Sorted by Pos, non-overlapping, within bounds.
	for i, r := range regions {
		if r.Pos < 0 || r.Len <= 0 || r.Pos+r.Len > len(docs["a"]) {
			t.Errorf("region %d out of bounds: %+v", i, r)
		}
		if i > 0 && r.Pos < regions[i-1].Pos+regions[i-1].Len {
			t.Errorf("regions overlap or unsorted: %+v then %+v", regions[i-1], r)
		}
	}
}

func TestCoverUnindexedQuery(t *testing.T) {
	ix, _ := coverCorpus()
	text := "An entirely new story about tide pools and patient crabs waiting out the ebb." + coverFooter
	regions := ix.Cover(text, 3, 5)
	if len(regions) == 0 {
		t.Fatal("footer not found in an unindexed query text")
	}
	for _, r := range regions {
		if strings.Contains(text[r.Pos:r.Pos+r.Len], "tide pools") {
			t.Errorf("new story content wrongly covered: %q", text[r.Pos:r.Pos+r.Len])
		}
	}
	if !strings.Contains(text[regions[len(regions)-1].Pos:], "ASSM FAQ") {
		t.Errorf("footer region missing; regions: %v", regions)
	}
}

func TestCoverThresholds(t *testing.T) {
	ix, docs := coverCorpus()

	// minDocs=4 keeps the footer (DF=5) and drops the ad (DF=3).
	var covered []string
	for _, r := range ix.Cover(docs["a"], 4, 5) {
		covered = append(covered, docs["a"][r.Pos:r.Pos+r.Len])
	}
	joined := strings.Join(covered, "\x00")
	if strings.Contains(joined, "fund the servers") {
		t.Errorf("ad (DF=3) survived minDocs=4; regions: %q", covered)
	}
	if !strings.Contains(joined, "ASSM FAQ") {
		t.Errorf("footer (DF=5) missing at minDocs=4; regions: %q", covered)
	}

	// Thresholds beyond the corpus yield nothing.
	if got := ix.Cover(docs["a"], 6, 5); got != nil {
		t.Errorf("minDocs beyond corpus size: %v, want nil", got)
	}
	if got := ix.Cover(docs["a"], 3, 1000); got != nil {
		t.Errorf("minRun beyond any run length: %v, want nil", got)
	}
}

func TestCoverCacheInvalidation(t *testing.T) {
	ix, docs := coverCorpus()
	if len(ix.Cover(docs["a"], 4, 5)) == 0 {
		t.Fatal("footer (DF=5) not found at minDocs=4")
	}
	// Dropping two docs takes the footer to DF=3, under the threshold.
	ix.Remove("d")
	ix.Remove("e")
	if got := ix.Cover(docs["a"], 4, 5); got != nil {
		t.Errorf("stale DF after Remove: Cover = %v, want nil", got)
	}
	// Re-adding restores DF=4.
	ix.Add("d", docs["d"])
	if len(ix.Cover(docs["a"], 4, 5)) == 0 {
		t.Error("stale DF after Add: footer not found again")
	}
}

func TestCoverEmptyAndDeterministic(t *testing.T) {
	empty := winnow.NewIndex(idxK, idxW)
	if got := empty.Cover("a reasonably long query against an empty index finds nothing", 1, 1); got != nil {
		t.Errorf("Cover on empty index = %v, want nil", got)
	}

	ix, docs := coverCorpus()
	if got := ix.Cover("short", 1, 1); got != nil {
		t.Errorf("Cover of sub-fingerprint text = %v, want nil", got)
	}
	first := ix.Cover(docs["b"], 3, 5)
	for range 5 {
		if got := ix.Cover(docs["b"], 3, 5); !slices.Equal(got, first) {
			t.Fatal("Cover varies across calls")
		}
	}
}
