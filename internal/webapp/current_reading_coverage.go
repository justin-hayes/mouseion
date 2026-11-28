package webapp

import (
	"fmt"

	"github.com/justin-hayes/mouseion/internal/domain"
	"golang.org/x/text/language"
	"golang.org/x/text/message"
)

// readingCoverageNoWords is shown instead of a percentage when the Book has no
// analyzable words to compare.
const readingCoverageNoWords = "No words to compare"

// readingCoverageMatureDisplay is the share of the Book's word occurrences whose
// effective vocabulary identity is Mature.
func readingCoverageMatureDisplay(coverage domain.AnalysisCoverage) string {
	return readingCoverageShare(coverage.MatureTokenCount, coverage.AnalyzableTokenCount)
}

// readingCoverageInStudyDisplay is the lighter figure that also counts words
// introduced but not yet Mature.
func readingCoverageInStudyDisplay(coverage domain.AnalysisCoverage) string {
	return readingCoverageShare(coverage.InStudyTokenCount, coverage.AnalyzableTokenCount)
}

func readingCoverageShare(tokens, analyzable int64) string {
	if analyzable <= 0 {
		return readingCoverageNoWords
	}
	return fmt.Sprintf("%.1f%%", readingCoveragePercent(tokens, analyzable))
}

// readingCoveragePercent is tokens as a percentage of the analyzable tokens,
// capped at 100. The caller has checked that analyzable is positive.
func readingCoveragePercent(tokens, analyzable int64) float64 {
	return min(float64(tokens)*100/float64(analyzable), 100)
}

// readingCoverageToTargetText says roughly how many more words in study bring
// the Book to Target coverage (ADR 0136 §2).
func readingCoverageToTargetText(coverage domain.AnalysisCoverage) string {
	target := coverage.ToTarget
	switch {
	case coverage.AnalyzableTokenCount <= 0:
		return readingCoverageNoWords + " for Target coverage."
	case !target.Reachable:
		return fmt.Sprintf("Target coverage (%d%%): not reachable with the words in this Book.", target.TargetPercent)
	case target.LemmaCount == 0:
		return fmt.Sprintf("Target coverage (%d%%) reached.", target.TargetPercent)
	default:
		return fmt.Sprintf("About %s more words to reach Target coverage (%d%%).", readingCoverageCount(target.LemmaCount), target.TargetPercent)
	}
}

// The learner-facing interface is English; group evidence counts in that locale.
func readingCoverageCount(count int64) string {
	return message.NewPrinter(language.English).Sprintf("%d", count)
}

// currentReadingCoverageView is the hero's coverage: two figures and a bar when
// the counts are ready, otherwise a word that says why they are not.
type currentReadingCoverageView struct {
	Ready       bool
	Word        string
	MatureText  string
	InStudyText string
	MaturePct   float64
	// ExtraPct is the in-study share beyond Mature: "with words in study"
	// includes Mature words, so the bar stacks only the difference.
	ExtraPct    float64
	BarLabel    string
	Occurrences string
	Target      string
}

// currentReadingCoverageFor never reports zero for coverage it does not have:
// updating counts read "Updating"; missing or empty statistics read "Unavailable".
func currentReadingCoverageFor(item readingBookView) currentReadingCoverageView {
	coverage := item.Coverage
	if coverage == nil || item.StatisticsUnavailable || coverage.AnalyzableTokenCount <= 0 {
		if item.CountsUpdating {
			return currentReadingCoverageView{Word: "Updating"}
		}
		return currentReadingCoverageView{Word: "Unavailable"}
	}
	mature := readingCoveragePercent(coverage.MatureTokenCount, coverage.AnalyzableTokenCount)
	inStudy := readingCoveragePercent(coverage.InStudyTokenCount, coverage.AnalyzableTokenCount)
	view := currentReadingCoverageView{
		Ready:       true,
		MatureText:  readingCoverageMatureDisplay(*coverage),
		InStudyText: readingCoverageInStudyDisplay(*coverage),
		MaturePct:   mature,
		ExtraPct:    max(inStudy-mature, 0),
		Occurrences: fmt.Sprintf("%s of %s word occurrences are Mature.", readingCoverageCount(coverage.MatureTokenCount), readingCoverageCount(coverage.AnalyzableTokenCount)),
		Target:      readingCoverageToTargetText(*coverage),
	}
	view.BarLabel = fmt.Sprintf("Mature %s; with words in study %s", view.MatureText, view.InStudyText)
	return view
}

func coverageBarWidth(percent float64) string {
	return fmt.Sprintf("width:%.3f%%", percent)
}
