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
	return fmt.Sprintf("%.1f%%", min(float64(tokens)*100/float64(analyzable), 100))
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
