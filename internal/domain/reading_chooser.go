package domain

// CoverageBand identifies a chooser group. It describes lexical coverage
// evidence only; it is not an estimate of difficulty or a recommendation.
type CoverageBand string

const (
	CoverageBand99Plus       CoverageBand = "99_plus"
	CoverageBand97To99       CoverageBand = "97_to_99"
	CoverageBand95To97       CoverageBand = "95_to_97"
	CoverageBandBelow95      CoverageBand = "below_95"
	CoverageBandNoComparison CoverageBand = "no_comparison"
)

// CoverageBandFor classifies exact Mature token counts without using rounded display
// percentages. A zero or negative denominator has no numeric comparison.
func CoverageBandFor(matureTokens, analyzableTokens int64) CoverageBand {
	if analyzableTokens <= 0 {
		return CoverageBandNoComparison
	}
	if matureTokens < 0 {
		matureTokens = 0
	}
	if matureTokens > analyzableTokens {
		matureTokens = analyzableTokens
	}
	switch {
	case coverageAtLeast(matureTokens, analyzableTokens, 99):
		return CoverageBand99Plus
	case coverageAtLeast(matureTokens, analyzableTokens, 97):
		return CoverageBand97To99
	case coverageAtLeast(matureTokens, analyzableTokens, 95):
		return CoverageBand95To97
	default:
		return CoverageBandBelow95
	}
}

// CoverageGapToNextBand returns the number of Mature tokens needed to reach the
// next coverage band. The target follows CoverageBandFor's exact boundaries.
func CoverageGapToNextBand(matureTokens, analyzableTokens int64) (targetPercent int, gap int64) {
	if analyzableTokens <= 0 {
		return 0, 0
	}
	if matureTokens < 0 {
		matureTokens = 0
	}
	if matureTokens > analyzableTokens {
		matureTokens = analyzableTokens
	}
	switch CoverageBandFor(matureTokens, analyzableTokens) {
	case CoverageBandBelow95:
		targetPercent = 95
	case CoverageBand95To97:
		targetPercent = 97
	case CoverageBand97To99:
		targetPercent = 99
	case CoverageBand99Plus, CoverageBandNoComparison:
		return 0, 0
	}
	return targetPercent, max(minimumTokensForCoverage(analyzableTokens, targetPercent)-matureTokens, 0)
}

// coverageAtLeast compares mature/total with percent/100 by computing the
// smallest integer token count that reaches the threshold. Splitting total by
// 100 keeps the exact arithmetic safe for the full int64 range.
func coverageAtLeast(mature, total int64, percent int64) bool {
	return mature >= minimumTokensForCoverage(total, int(percent))
}

func minimumTokensForCoverage(total int64, percent int) int64 {
	quotient, remainder := total/100, total%100
	return quotient*int64(percent) + (remainder*int64(percent)+99)/100
}
