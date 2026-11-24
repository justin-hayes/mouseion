package releasecheck

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
)

// Gate says which constructions and subtype labels the shipping gate emits.
type Gate interface {
	Ships(construction string) bool
	ShipsLabel(construction, subtype string) bool
}

// Detected is the Go detectors' ungated output over the pinned files.
type Detected struct {
	// Emitted holds the key of every occurrence, with its subtype.
	Emitted map[Key]string
	// Hits counts occurrences per construction.
	Hits map[string]int
}

// Input is everything Evaluate reads.
type Input struct {
	Judgments     []Judgment
	ResearchCount map[CountKey]int
	Baseline      Baseline
	Detected      Detected
	Gate          Gate
	// Constructions lists the construction ids in print order.
	Constructions []string
}

// Line is one row of the precision table.
type Line struct {
	Construction, Subtype string
	// Strata names the strata the estimate used.
	Strata string
	Judged int
	// Precision, Lower and Upper are the estimate and its 95% interval.
	Precision, Lower, Upper float64
	// EmittedOnly is the same estimate over judged rows the Go output emits.
	EmittedOnly float64
	// NotEmitted counts judged rows of the used strata the Go output no longer
	// emits.
	NotEmitted int
	// GoHits and ResearchHits are the ungated Go hit count and the research
	// corpus count (subtype lines carry the research count only).
	GoHits, ResearchHits int
	Shipped, Clears      bool
}

// Verdict is the table's last column.
func (l Line) Verdict() string {
	switch {
	case l.Shipped && l.Clears:
		return "shipped"
	case l.Shipped:
		return "shipped, BELOW THE BAR"
	case l.Clears:
		return "held off (clears the bar)"
	}
	return "held off"
}

// Report is the outcome of the release check.
type Report struct {
	Lines []Line
	// Failures are the failure conditions (a)–(c); the command exits non-zero
	// when any is set.
	Failures []string
	// Notes are informational lines printed under the table.
	Notes []string
}

// Evaluate computes precision per construction and subtype and the three
// failure conditions.
func Evaluate(in Input) (Report, error) {
	var report Report
	for _, id := range in.Constructions {
		lines, err := constructionLines(in, id)
		if err != nil {
			return Report{}, err
		}
		report.Lines = append(report.Lines, lines...)
	}
	for _, line := range report.Lines {
		if line.Shipped && !line.Clears {
			name := line.Construction
			if line.Subtype != "" {
				name += "/" + line.Subtype
			}
			report.Failures = append(report.Failures, fmt.Sprintf(
				"(a) %s is shipped below the bar: %.3f (lower bound %.3f) on %d judged", name, line.Precision, line.Lower, line.Judged))
		}
	}
	report.Failures = append(report.Failures, Flips(in.Judgments, in.Detected.Emitted)...)
	report.Failures = append(report.Failures, HitDrift(in.Baseline, in.Detected.Hits, in.Constructions)...)
	report.Notes = notes(in)
	return report, nil
}

// stratumKey is a judged stratum of one construction.
type stratumKey struct{ stratum, detail string }

func (k stratumKey) String() string {
	if k.detail == "" {
		return k.stratum
	}
	return k.stratum + "/" + k.detail
}

func isEmitted(in Input, j Judgment) bool {
	_, ok := in.Detected.Emitted[j.Key()]
	return ok
}

// constructionLines builds the construction's line and its subtype lines.
func constructionLines(in Input, id string) ([]Line, error) {
	groups := map[stratumKey][]Judgment{}
	subtypes := map[string][]Judgment{}
	for _, j := range in.Judgments {
		if j.Construction != id || j.Measurement != Measurement2615 {
			continue
		}
		switch j.Stratum {
		case StratumHit, StratumExcluded:
			groups[stratumKey{j.Stratum, j.StratumDetail}] = append(groups[stratumKey{j.Stratum, j.StratumDetail}], j)
		}
		if (j.Stratum == StratumHit || j.Stratum == StratumHitTopup) && j.Subtype != "" {
			subtypes[j.Subtype] = append(subtypes[j.Subtype], j)
		}
	}
	main, err := constructionLine(in, id, groups)
	if err != nil {
		return nil, err
	}
	lines := []Line{main}
	names := make([]string, 0, len(subtypes))
	for name := range subtypes {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		lines = append(lines, subtypeLine(in, id, name, subtypes[name]))
	}
	return lines, nil
}

func constructionLine(in Input, id string, groups map[stratumKey][]Judgment) (Line, error) {
	line := Line{
		Construction: id, GoHits: in.Detected.Hits[id], Shipped: in.Gate.Ships(id),
		ResearchHits: in.ResearchCount[CountKey{id, StratumHit, "", ""}],
	}
	used := participating(in, groups)
	if len(used) == 0 {
		line.Precision, line.Lower, line.Upper, line.EmittedOnly = math.NaN(), math.NaN(), math.NaN(), math.NaN()
		return line, nil
	}
	strata, judged, err := tally(in, id, groups, used, false)
	if err != nil {
		return Line{}, err
	}
	emitted, _, err := tally(in, id, groups, used, true)
	if err != nil {
		return Line{}, err
	}
	names := make([]string, len(used))
	for i, key := range used {
		names[i] = key.String()
		for _, j := range groups[key] {
			if !isEmitted(in, j) {
				line.NotEmitted++
			}
		}
	}
	line.Judged, line.Strata = judged, strings.Join(names, " + ")
	line.Precision, line.Lower, line.Upper = estimate(strata)
	line.EmittedOnly, _, _ = estimate(emitted)
	line.Clears = Clears(line.Precision, line.Lower, line.Judged, MinJudgedConstruction)
	return line, nil
}

// participating returns the strata with at least one judged item the current
// output emits, in name order.
func participating(in Input, groups map[stratumKey][]Judgment) []stratumKey {
	var used []stratumKey
	for key, rows := range groups {
		if slices.ContainsFunc(rows, func(j Judgment) bool { return isEmitted(in, j) }) {
			used = append(used, key)
		}
	}
	sort.Slice(used, func(i, j int) bool { return used[i].String() < used[j].String() })
	return used
}

// judgedTally counts rows and true rows, over the emitted rows only when
// emittedOnly is set.
func judgedTally(in Input, rows []Judgment, emittedOnly bool) Stratum {
	s := Stratum{}
	for _, j := range rows {
		if emittedOnly && !isEmitted(in, j) {
			continue
		}
		s.Judged++
		if j.Judgment {
			s.True++
		}
	}
	return s
}

// tally counts each participating stratum's judged and true items, over the
// emitted rows only when emittedOnly is set, and attaches the research count as
// weight when more than one stratum takes part.
func tally(in Input, id string, groups map[stratumKey][]Judgment, used []stratumKey, emittedOnly bool) ([]Stratum, int, error) {
	var strata []Stratum
	judged := 0
	for _, key := range used {
		s := judgedTally(in, groups[key], emittedOnly)
		if s.Judged == 0 {
			continue
		}
		if len(used) > 1 {
			count, ok := in.ResearchCount[CountKey{id, key.stratum, key.detail, ""}]
			if !ok {
				return nil, 0, fmt.Errorf("releasecheck: missing research count for %s %s", id, key)
			}
			s.Weight = count
		}
		judged += s.Judged
		strata = append(strata, s)
	}
	return strata, judged, nil
}

// estimate is Wilson for one stratum and the stratified estimator otherwise.
func estimate(strata []Stratum) (p, lo, hi float64) {
	switch len(strata) {
	case 0:
		return math.NaN(), math.NaN(), math.NaN()
	case 1:
		return Wilson(strata[0].True, strata[0].Judged)
	}
	return Stratified(strata)
}

// subtypeLine measures one subtype label over the hit and hit-topup rows: an
// item is correct when it is a true occurrence with that judged subtype.
func subtypeLine(in Input, id, subtype string, rows []Judgment) Line {
	line := Line{
		Construction: id, Subtype: subtype, Strata: "hit+hit-topup", Judged: len(rows),
		Shipped:      in.Gate.ShipsLabel(id, subtype),
		ResearchHits: in.ResearchCount[CountKey{id, StratumHit, "", subtype}],
	}
	k, kEmitted, nEmitted := 0, 0, 0
	for _, j := range rows {
		correct := j.Judgment && j.JudgedSubtype == subtype
		if correct {
			k++
		}
		if isEmitted(in, j) {
			nEmitted++
			if correct {
				kEmitted++
			}
		} else {
			line.NotEmitted++
		}
	}
	line.Precision, line.Lower, line.Upper = Wilson(k, len(rows))
	line.EmittedOnly, _, _ = Wilson(kEmitted, nEmitted)
	line.Clears = Clears(line.Precision, line.Lower, len(rows), MinJudgedSubtype)
	return line
}

// Flips applies failure condition (b): a judged row whose emission changed
// against its baseline in the harmful direction.
func Flips(judgments []Judgment, emitted map[Key]string) []string {
	var lost, gained []Judgment
	for _, j := range judgments {
		_, now := emitted[j.Key()]
		switch {
		case j.Judgment && j.EmittedBaseline && !now:
			lost = append(lost, j)
		case !j.Judgment && !j.EmittedBaseline && now:
			gained = append(gained, j)
		}
	}
	var out []string
	if len(lost) > 0 {
		out = append(out, fmt.Sprintf("(b) %d judged-true rows are no longer emitted: %s", len(lost), sampleKeys(lost)))
	}
	if len(gained) > 0 {
		out = append(out, fmt.Sprintf("(b) %d judged-false rows are now emitted: %s", len(gained), sampleKeys(gained)))
	}
	return out
}

func sampleKeys(rows []Judgment) string {
	var items []string
	for i, j := range rows {
		if i == 5 {
			items = append(items, "…")
			break
		}
		items = append(items, fmt.Sprintf("%s/%s/%s/%s", j.Work, j.SentID, j.Anchor, j.Construction))
	}
	return strings.Join(items, ", ")
}

// driftLimit is failure condition (c)'s tolerance.
const driftLimit = 0.10

// HitDrift applies failure condition (c): a construction's ungated hit count
// moved more than 10% from the baseline, so the sample no longer represents it.
func HitDrift(baseline Baseline, hits map[string]int, order []string) []string {
	var out []string
	for _, id := range order {
		base, ok := baseline.Hits[id]
		if !ok {
			out = append(out, fmt.Sprintf("(c) %s has no hit count in counts_baseline.tsv; run -baseline", id))
			continue
		}
		now := hits[id]
		if math.Abs(float64(now-base)) > driftLimit*float64(base) {
			out = append(out, fmt.Sprintf(
				"(c) %s ungated hits moved from %d to %d (more than 10%%): a fresh random sample of 120 is required", id, base, now))
		}
	}
	return out
}

// notes builds the informational lines under the table.
func notes(in Input) []string {
	var out []string
	var earlier, abstain, notEmitted int
	perMeasurement := map[string]int{}
	for _, j := range in.Judgments {
		perMeasurement[j.Measurement]++
		switch {
		case j.Stratum == StratumAbstain:
			abstain++
		case j.Measurement == Measurement2607:
			earlier++
		}
		if !isEmitted(in, j) && j.EmittedBaseline {
			notEmitted++
		}
	}
	out = append(out,
		fmt.Sprintf("judgment rows: %d (#2607 %d, #2615 %d)", len(in.Judgments), perMeasurement[Measurement2607], perMeasurement[Measurement2615]),
		fmt.Sprintf("earlier sample (#2607-only rows, flip check only): %d", earlier),
		fmt.Sprintf("not counted (abstain rows): %d", abstain),
		fmt.Sprintf("baseline-emitted rows no longer emitted: %d", notEmitted),
		fmt.Sprintf("detector baseline version: %d", in.Baseline.DetectorVersion))
	return out
}
