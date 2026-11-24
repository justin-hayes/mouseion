// Package releasecheck is the construction release check: it joins the Go
// detectors' output over the pinned OGA files to a committed, ID-only reviewed
// judgment file and computes precision against the shipping bar (ADR 0123).
// Nothing here holds OGA text: the judgments name sentences and tokens by id.
package releasecheck

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"strings"
)

// Measurement names the research measurement a judgment came from.
const (
	Measurement2607 = "2607"
	Measurement2615 = "2615"
)

// Strata of the research sample.
const (
	StratumHit      = "hit"
	StratumHitTopup = "hit-topup"
	StratumExcluded = "excluded"
	StratumLoose    = "loose"
	StratumAbstain  = "abstain"
)

// JudgmentsHeader is the exact header line of judgments.tsv.
const JudgmentsHeader = "work\tsent_id\tanchor\tconstruction\tmeasurement\titem\tstratum\tstratum_detail\tsubtype\tjudgment\tjudged_subtype\tjudge\temitted_baseline"

// Judgment is one reviewed item: a construction occurrence the research
// sampled and a reviewer judged. It carries ids and judgments only.
type Judgment struct {
	Work, SentID, Anchor, Construction string
	Measurement, Item                  string
	Stratum, StratumDetail, Subtype    string
	// Judgment is true when the item is a real occurrence of its construction.
	Judgment bool
	// JudgedSubtype is the reviewer's subtype for a true item.
	JudgedSubtype string
	// Judge is "justin" or "agent".
	Judge string
	// EmittedBaseline records whether the Go output emitted the item when the
	// baseline was written.
	EmittedBaseline bool
}

// Key identifies the occurrence a judgment is about.
type Key struct{ Work, SentID, Anchor, Construction string }

// Key returns the judgment's unique key.
func (j Judgment) Key() Key { return Key{j.Work, j.SentID, j.Anchor, j.Construction} }

// ParseJudgments reads judgments.tsv: the exact header, then one row per
// judgment with a unique key.
func ParseJudgments(r io.Reader) ([]Judgment, error) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 0, 64*1024), 1<<20)
	if !scanner.Scan() {
		return nil, errors.New("releasecheck: judgments file is empty")
	}
	if scanner.Text() != JudgmentsHeader {
		return nil, errors.New("releasecheck: judgments header is not the expected one")
	}
	var out []Judgment
	seen := map[Key]bool{}
	line := 1
	for scanner.Scan() {
		line++
		fields := strings.Split(scanner.Text(), "\t")
		if len(fields) != 13 {
			return nil, fmt.Errorf("releasecheck: judgments line %d has %d fields, want 13", line, len(fields))
		}
		judgment, err := parseBool(fields[9])
		if err != nil {
			return nil, fmt.Errorf("releasecheck: judgments line %d judgment: %w", line, err)
		}
		baseline, err := parseBool(fields[12])
		if err != nil {
			return nil, fmt.Errorf("releasecheck: judgments line %d emitted_baseline: %w", line, err)
		}
		j := Judgment{
			Work: fields[0], SentID: fields[1], Anchor: fields[2], Construction: fields[3],
			Measurement: fields[4], Item: fields[5], Stratum: fields[6], StratumDetail: fields[7],
			Subtype: fields[8], Judgment: judgment, JudgedSubtype: fields[10], Judge: fields[11],
			EmittedBaseline: baseline,
		}
		if seen[j.Key()] {
			return nil, fmt.Errorf("releasecheck: judgments line %d repeats key %v", line, j.Key())
		}
		seen[j.Key()] = true
		out = append(out, j)
	}
	return out, scanner.Err()
}

func parseBool(value string) (bool, error) {
	switch value {
	case "true":
		return true, nil
	case "false":
		return false, nil
	}
	return false, fmt.Errorf("%q is not true or false", value)
}

// WriteJudgments writes judgments.tsv.
func WriteJudgments(w io.Writer, judgments []Judgment) error {
	var b strings.Builder
	b.WriteString(JudgmentsHeader + "\n")
	for _, j := range judgments {
		fmt.Fprintf(&b, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\t%t\t%s\t%s\t%t\n",
			j.Work, j.SentID, j.Anchor, j.Construction, j.Measurement, j.Item, j.Stratum, j.StratumDetail,
			j.Subtype, j.Judgment, j.JudgedSubtype, j.Judge, j.EmittedBaseline)
	}
	_, err := io.WriteString(w, b.String())
	return err
}
