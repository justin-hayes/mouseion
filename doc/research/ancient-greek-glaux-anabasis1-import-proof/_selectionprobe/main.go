// Command selectionprobe runs Mouseion's production vocabulary selection code
// over the GLAUx Anabasis Book 1 analysis written by import_proof.py (#2050).
//
// It is research tooling: the leading underscore keeps it out of `./...`.
// The library holds only this Book, Known and Reserved are empty, and there
// are no learner decisions. persistence.currentReadingCandidateFrequency
// supplies the across-Books total only below the in-Book floor; with one
// analyzed Book that total is the Book's own count.
//
// Usage: go run ./doc/research/ancient-greek-glaux-anabasis1-import-proof/_selectionprobe <analysis.json> <out.tsv>
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/selection"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: selectionprobe <analysis.json> <out.tsv>")
		os.Exit(2)
	}
	if err := run(os.Args[1], os.Args[2]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(in, out string) error {
	raw, err := os.ReadFile(in)
	if err != nil {
		return err
	}
	var corpus analyzer.Result
	if err := json.Unmarshal(raw, &corpus); err != nil {
		return err
	}
	cfg := selection.DefaultConfig("glaux-anabasis-book1")
	candidates, err := selection.Project(corpus, cfg, nil)
	if err != nil {
		return err
	}
	eligibility := selection.NewEligibility(nil, nil)
	var b strings.Builder
	b.WriteString("canonical_key\tupos\toccurrences\tselected\n")
	selected := 0
	for _, c := range candidates {
		row := domain.SelectionCandidate{Language: c.Identity.Language, CanonicalLemma: c.Identity.CanonicalLemma, UPOS: c.Identity.UPOS, OccurrenceCount: c.OccurrenceCount}
		var acrossBooks int64
		if c.OccurrenceCount < selection.DefaultRecurringMinOccurrences {
			acrossBooks = int64(c.OccurrenceCount)
		}
		mark := "no"
		if eligibility.AllowsBookDeckCandidate(row, acrossBooks) {
			mark = "yes"
			selected++
		}
		fmt.Fprintf(&b, "%s\t%s\t%d\t%s\n", c.Identity.CanonicalLemma, c.Identity.UPOS, c.OccurrenceCount, mark)
	}
	if err := os.WriteFile(out, []byte(b.String()), 0o644); err != nil {
		return err
	}
	stats := selection.AnalyzableStatistics(corpus, cfg)
	fmt.Printf("candidates=%d selected=%d analyzable_tokens=%d distinct_identities=%d sentences=%d\n",
		len(candidates), selected, stats.AnalyzableTokenCount, stats.DistinctLemmaCount, stats.TextProfile.SentenceCount)
	return nil
}
