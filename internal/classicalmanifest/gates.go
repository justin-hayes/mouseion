package classicalmanifest

import (
	"fmt"
	"sort"
	"strings"

	"github.com/justin-hayes/mouseion/internal/ancientgreek"
	"github.com/justin-hayes/mouseion/internal/classical"
)

// MaxSplitWordRate is the share of a Latin Work's words that may be one-letter
// fragments before gate (ii) flags it.
//
// Evidence (2026-11-25, OLA v0.2.0, 569 of its CoNLL-U members measured with the
// rule below, read by Range request): across the 353 Works of 5,000 words or
// more the rate has median 0.36%, p90 0.78%, p95 1.01%, p99 1.67% and maximum
// 3.58%. At 1% the gate would flag 19 of 353 and send every one to a reviewer;
// at 1.5% it flags 5; at 2% it flags 3, all from phi2349, whose rate comes from
// a thousand-odd stray "v" tokens (2.7-3.6%). A 1% rate is ordinary: stray
// "c", "m", "s" and "t" are enclitic and abbreviation tokens the source splits.
// 2% therefore catches the outliers without flagging the bulk of a batch, which
// would reopen ADR 0132.
const MaxSplitWordRate = 0.02

// Gate names, as the report prints them.
const (
	GateCitations = "citations"
	GateSplitWord = "split-words"
)

// GateResult is one gate's outcome for one Work.
type GateResult struct {
	Name   string
	Passed bool
	// Detail states the measured evidence. It never carries source text.
	Detail string
	// Clearable is false for a failure no reviewer note can clear.
	Clearable bool
}

// WorkGateReport is every gate result of one Latin Work.
type WorkGateReport struct {
	WorkKey string
	// Evaluated is false when the run had no upstream archive, so the gates
	// could not run; verify mode then relies on the committed manifest.
	Evaluated bool
	Gates     []GateResult
	// ReviewerNote is the override's clearance, if any.
	ReviewerNote string
}

// Failed lists the gates the Work failed.
func (r WorkGateReport) Failed() []GateResult {
	var failed []GateResult
	for _, gate := range r.Gates {
		if !gate.Passed {
			failed = append(failed, gate)
		}
	}
	return failed
}

// Cleared reports whether a reviewer note clears every failed gate.
func (r WorkGateReport) Cleared() bool {
	if r.ReviewerNote == "" || len(r.Failed()) == 0 {
		return false
	}
	for _, gate := range r.Failed() {
		if !gate.Clearable {
			return false
		}
	}
	return true
}

// Admitted reports whether the Work may enter the manifest: it passed every
// gate, or a reviewer note cleared the ones it failed.
func (r WorkGateReport) Admitted() bool { return len(r.Failed()) == 0 || r.Cleared() }

// Lines renders the report for the notes and the command output.
func (r WorkGateReport) Lines() []string {
	if !r.Evaluated {
		return []string{fmt.Sprintf("- Gates for %s: not evaluated, the upstream archive was absent", r.WorkKey)}
	}
	lines := []string{fmt.Sprintf("- Gates for %s:", r.WorkKey)}
	for _, gate := range r.Gates {
		status := "pass"
		if !gate.Passed {
			status = "FAIL"
		}
		lines = append(lines, fmt.Sprintf("  - %s: %s (%s)", gate.Name, status, gate.Detail))
	}
	switch {
	case r.Cleared():
		lines = append(lines, "  - cleared by reviewer note: "+r.ReviewerNote)
	case len(r.Failed()) > 0:
		lines = append(lines, "  - excluded")
	}
	return lines
}

// GateError refuses a Work that failed a gate and has no clearing note.
type GateError struct{ Report WorkGateReport }

func (e *GateError) Error() string {
	names := make([]string, 0, len(e.Report.Failed()))
	for _, gate := range e.Report.Failed() {
		names = append(names, gate.Name)
	}
	return fmt.Sprintf("classical: %s failed the %s gate", e.Report.WorkKey, strings.Join(names, " and "))
}

// evaluateLatinGates applies gates (i) and (ii) to one Work's scan.
func evaluateLatinGates(override classical.Override, scan ancientgreek.LatinScan) WorkGateReport {
	report := WorkGateReport{WorkKey: override.WorkKey, Evaluated: true, ReviewerNote: override.ReviewerNote}
	report.Gates = append(report.Gates, citationGate(scan), splitWordGate(scan))
	return report
}

// citationGate is gate (i): every token has a Citation and Citations never run
// backwards. Uncited tokens are the one exception to clearance by reviewer
// note: the Work could not be prepared, so a note would admit an unreadable
// Work. An uncited token cannot be cleared, because the Work could not be
// read; a backwards run can, by a reviewer who has read it.
func citationGate(scan ancientgreek.LatinScan) GateResult {
	gate := GateResult{Name: GateCitations, Passed: true, Clearable: true, Detail: fmt.Sprintf("%d tokens, all cited, none backwards", scan.Tokens)}
	switch {
	case scan.Uncited > 0:
		gate.Passed, gate.Clearable = false, false
		gate.Detail = fmt.Sprintf("%d of %d tokens have no Citation", scan.Uncited, scan.Tokens)
	case scan.Backwards > 0:
		gate.Passed = false
		gate.Detail = fmt.Sprintf("Citations run backwards %d time(s), first at %s", scan.Backwards, scan.FirstBackwards)
	}
	return gate
}

// splitWordGate is gate (ii): the split-word rate stays at or under
// MaxSplitWordRate.
func splitWordGate(scan ancientgreek.LatinScan) GateResult {
	rate := scan.SplitWordRate()
	gate := GateResult{Name: GateSplitWord, Passed: rate < MaxSplitWordRate, Clearable: true}
	gate.Detail = fmt.Sprintf("%d of %d words, %.3f%%, limit %.3f%%", scan.SplitWords, scan.Words, rate*100, MaxSplitWordRate*100)
	return gate
}

// recordGates adds a report to the run and the notes.
func (b *builder) recordGates(report WorkGateReport) {
	b.gates = append(b.gates, report)
	for _, line := range report.Lines() {
		fmt.Fprintln(&b.notes, line)
	}
}

// recordUnevaluatedGates reports a Latin Work whose source was absent.
func (b *builder) recordUnevaluatedGates(override classical.Override) {
	b.recordGates(WorkGateReport{WorkKey: override.WorkKey, ReviewerNote: override.ReviewerNote})
}

// GateReport renders every Latin Work's gate results, one block per Work.
func (r Result) GateReport() string {
	reports := append([]WorkGateReport(nil), r.Gates...)
	sort.Slice(reports, func(i, j int) bool { return reports[i].WorkKey < reports[j].WorkKey })
	var out strings.Builder
	for _, report := range reports {
		for _, line := range report.Lines() {
			fmt.Fprintln(&out, line)
		}
	}
	return out.String()
}
