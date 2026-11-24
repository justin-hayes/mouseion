// Command convert is the one-off conversion that built judgments.tsv. It reads
// the two committed research tables from the branch
// research/grc-construction-detection-measurement (commit db6ba186):
//
//	doc/research/grc-construction-detection-measurement/occurrences.tsv  (#2607)
//	doc/research/grc-construction-detection-remeasure/occurrences.tsv    (#2615)
//
// and writes the ID-only judgment file to standard output. It is kept so the
// file can be rebuilt and audited; the merge rules live in releasecheck.Convert.
//
//	git show db6ba186:doc/research/grc-construction-detection-measurement/occurrences.tsv > a.tsv
//	git show db6ba186:doc/research/grc-construction-detection-remeasure/occurrences.tsv > b.tsv
//	go run ./internal/constructions/releasecheck/convert a.tsv b.tsv > judgments.tsv
package main

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/justin-hayes/mouseion/internal/constructions/releasecheck"
)

func main() {
	if err := run(os.Args[1:], os.Stdout, os.Stderr); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(args []string, stdout, stderr io.Writer) error {
	if len(args) != 2 {
		return errors.New("usage: convert <2607 occurrences.tsv> <2615 occurrences.tsv>")
	}
	first, err := os.ReadFile(args[0]) //nolint:gosec // the maintainer names the research table.
	if err != nil {
		return err
	}
	second, err := os.ReadFile(args[1]) //nolint:gosec // the maintainer names the research table.
	if err != nil {
		return err
	}
	judgments, stats, err := releasecheck.Convert(bytes.NewReader(first), bytes.NewReader(second))
	if err != nil {
		return err
	}
	summary := fmt.Sprintf("read %d (#2607) + %d (#2615) rows; %d #2607 rows superseded; %d dropped as unsure or empty; %d kept\n",
		stats.Rows2607, stats.Rows2615, stats.Superseded, stats.Unsure, stats.Kept)
	if _, err := io.WriteString(stderr, summary); err != nil {
		return err
	}
	return releasecheck.WriteJudgments(stdout, judgments)
}
