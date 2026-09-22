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
		return "Unavailable"
	}
	return fmt.Sprintf("%.1f%%", journeyForecastPercent(coverage))
}

func sameForecastCoverage(left, right *domain.JourneyForecastCoverage) bool {
	return left != nil && right != nil && left.KnownTokenCount == right.KnownTokenCount && left.AnalyzableTokenCount == right.AnalyzableTokenCount
}

func journeyForecastDeltaLabel(coverage, comparison *domain.JourneyForecastCoverage) string {
	if coverage == nil || comparison == nil {
		return ""
	}
	if sameForecastCoverage(coverage, comparison) {
		return "No change"
	}
	delta := journeyForecastPercent(coverage) - journeyForecastPercent(comparison)
	if delta > -0.05 && delta < 0.05 {
		return fmt.Sprintf("%+.2f percentage points", delta)
	}
	return fmt.Sprintf("%+.1f percentage points", delta)
}

func journeyForecastCurrentCoverage(item journeyBookView) *domain.JourneyForecastCoverage {
	if item.Forecast != nil {
		return item.Forecast.Current
	}
	if item.Coverage == nil {
		return nil
	}
	return &domain.JourneyForecastCoverage{
		KnownTokenCount:      item.Coverage.KnownTokenCount,
		AnalyzableTokenCount: item.Coverage.AnalyzableTokenCount,
	}
}

type journeyForecastStageView struct {
	Label             string
	Coverage          *domain.JourneyForecastCoverage
	Comparison        *domain.JourneyForecastCoverage
	ComparisonLabel   string
	UnavailableReason string
	Qualifier         string
	Context           string
}

func journeyForecastStages(item journeyBookView, primary bool) []journeyForecastStageView {
	forecast := item.Forecast
	current := journeyForecastCurrentCoverage(item)
	reason := ""
	if forecast != nil {
		reason = forecast.UnavailableReason
	}
	if reason == "" && forecast == nil && item.ForecastUnavailable {
		reason = "the Journey forecast is unavailable"
	}
	if reason == "" && forecast == nil && current == nil {
		reason = "current coverage cannot be calculated from available evidence"
	}
	currentReason := ""
	if current == nil {
		currentReason = reason
		if currentReason == "" {
			currentReason = "current coverage cannot be calculated from available evidence"
		}
	}
	stages := []journeyForecastStageView{
		{
			Label:             "Current coverage",
			Coverage:          current,
			UnavailableReason: currentReason,
			Context:           "Modeled Known vocabulary in this Book's current analysis.",
		},
	}
	if item.ForecastHasGoal {
		var after *domain.JourneyForecastCoverage
		if forecast != nil {
			after = forecast.AfterGoal
		}
		afterReason := reason
		if after == nil && afterReason == "" {
			afterReason = "after-Goal coverage cannot be calculated from available evidence"
		}
		stages = append(stages, journeyForecastStageView{
			Label:             "After Primary Goal coverage",
			Coverage:          after,
			Comparison:        current,
			ComparisonLabel:   "Current coverage",
			UnavailableReason: afterReason,
			Context:           "Modeled Known vocabulary plus the active Primary Goal's frozen Reserved vocabulary after completion.",
		})
	} else {
		stages = append(stages, journeyForecastStageView{
			Label:     "After Primary Goal coverage",
			Qualifier: "No active Primary Goal",
			Context:   "This stage requires an active Primary Goal and its frozen Reserved vocabulary.",
		})
	}
	if !primary {
		var after, arrival *domain.JourneyForecastCoverage
		if forecast != nil {
			after = forecast.AfterGoal
			arrival = forecast.OnArrival
		}
		arrivalReason := reason
		if arrival == nil && arrivalReason == "" {
			arrivalReason = "on-arrival coverage cannot be calculated from available evidence"
		}
		qualifier := ""
		if forecast != nil && forecast.LowerBound {
			qualifier = "Lower bound: an earlier Journey Book has unavailable evidence, so this value excludes any vocabulary it might contribute."
		}
		stages = append(stages, journeyForecastStageView{
			Label:             "On arrival coverage",
			Coverage:          arrival,
			Comparison:        after,
			ComparisonLabel:   "After Primary Goal coverage",
			UnavailableReason: arrivalReason,
			Qualifier:         qualifier,
			Context:           "Modeled Known vocabulary plus trustworthy recurring vocabulary from earlier Books in Your order.",
		})
	}
	return stages
}
