package winnow

// Region is a byte range within a single text, as returned by
// [Index.Cover]. It reads like [Span] restricted to one document: a
// located region, not a proof of byte equality, with edges within about
// w+k bytes of the true boundaries.
type Region struct {
	Pos int // byte offset of the region in the text
	Len int // byte length of the region
}

// DocFreq returns the number of distinct indexed documents whose
// fingerprints include hash — the document frequency of a shingle, the
// signal separating corpus boilerplate (high) from content (low). Zero if
// hash was never indexed.
func (ix *Index) DocFreq(hash uint64) int {
	if ix.docFreq != nil {
		return ix.docFreq[hash]
	}
	ps := ix.postings[hash]
	if len(ps) == 0 {
		return 0
	}
	seen := make(map[string]bool)
	n := 0
	for _, p := range ps {
		if !seen[p.id] {
			seen[p.id] = true
			n++
		}
	}
	return n
}

// Cover returns the byte ranges of text that recur across the corpus: the
// regions whose fingerprints each appear in at least minDocs indexed
// documents and that form a run of at least minRun consecutive
// fingerprints. Runs of consecutive qualifying fingerprints collapse into
// one Region, so a shared footer is one range, ready to trim or flag.
// Results are sorted by Pos and do not overlap. Returns nil if there are
// none.
//
// minRun screens out incidental recurrence — a common phrase that happens
// to appear in minDocs documents surfaces as an isolated fingerprint or
// two, while real boilerplate matches in long runs; around 5 is a
// sensible starting point. text need not be indexed, and if it is, it
// counts toward its own document frequencies: minDocs=3 then means "this
// document and at least two others".
//
// Like every winnow result, edges are located to fingerprint granularity:
// a Region's boundaries land within about w+k bytes of the true passage
// edge (see [Span]). For byte-exact trimming, snap the returned ranges
// out to the nearest line boundary.
//
// The first call after an [Index.Add] or [Index.Remove] builds a
// document-frequency table in one pass over the index; subsequent calls
// reuse it. Covering an entire corpus therefore costs one pass plus the
// per-text work, not one pass per document.
func (ix *Index) Cover(text string, minDocs, minRun int) []Region {
	df := ix.docFreqs()
	var out []Region
	var start, end, runLen int // current run: first/last fingerprint Pos, length
	flush := func() {
		if runLen >= minRun && runLen > 0 {
			out = append(out, Region{Pos: start, Len: end + ix.k - start})
		}
		runLen = 0
	}
	for fp := range Text(text, ix.k, ix.w) {
		if df[fp.Hash] >= minDocs {
			if runLen == 0 {
				start = fp.Pos
			}
			end = fp.Pos
			runLen++
		} else {
			flush()
		}
	}
	flush()
	return out
}

// docFreqs returns the document frequency of every indexed hash, building
// the cached table on first use. Add and Remove invalidate the cache.
func (ix *Index) docFreqs() map[uint64]int {
	if ix.docFreq == nil {
		df := make(map[uint64]int, len(ix.postings))
		seen := make(map[string]bool)
		for h, ps := range ix.postings {
			n := 0
			for _, p := range ps {
				if !seen[p.id] {
					seen[p.id] = true
					n++
				}
			}
			clear(seen)
			df[h] = n
		}
		ix.docFreq = df
	}
	return ix.docFreq
}
