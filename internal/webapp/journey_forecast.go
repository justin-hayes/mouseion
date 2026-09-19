package webapp

import (
	"context"
	"fmt"

	"github.com/justin-hayes/mouseion/internal/domain"
)

type journeyForecastProvider interface {
	JourneyForecast(context.Context, string, string) (domain.JourneyForecast, error)
}

func journeyForecastPercent(coverage *domain.JourneyForecastCoverage) float64 {
	if coverage == nil || coverage.AnalyzableTokenCount == 0 {
		return 0
	}
	return min(float64(coverage.KnownTokenCount)*100/float64(coverage.AnalyzableTokenCount), 100)
}

func journeyForecastCoverageLabel(coverage *domain.JourneyForecastCoverage) string {
	if coverage == nil {
		return "unavailable"
	}
	return fmt.Sprintf("%.1f%% (%d of %d analyzable tokens)", journeyForecastPercent(coverage), coverage.KnownTokenCount, coverage.AnalyzableTokenCount)
}

func sameForecastCoverage(left, right *domain.JourneyForecastCoverage) bool {
	return left != nil && right != nil && left.KnownTokenCount == right.KnownTokenCount && left.AnalyzableTokenCount == right.AnalyzableTokenCount
}

func journeyForecastAfterGoalLabel(entry domain.JourneyForecastEntry) string {
	if sameForecastCoverage(entry.AfterGoal, entry.Current) {
		return "same as current (" + journeyForecastCoverageLabel(entry.Current) + ")"
	}
	return journeyForecastCoverageLabel(entry.AfterGoal)
}

func journeyForecastOnArrivalLabel(entry domain.JourneyForecastEntry) string {
	if sameForecastCoverage(entry.OnArrival, entry.Current) {
		return "same as current (" + journeyForecastCoverageLabel(entry.Current) + ")"
	}
	if sameForecastCoverage(entry.OnArrival, entry.AfterGoal) {
		return "same as after Goal (" + journeyForecastCoverageLabel(entry.AfterGoal) + ")"
	}
	return journeyForecastCoverageLabel(entry.OnArrival)
}
