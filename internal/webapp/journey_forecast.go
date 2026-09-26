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
		reason = "the Reading forecast is unavailable"
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
		qualifier := ""
		if primary {
			qualifier = "After completion of this current reading"
		}
		stages = append(stages, journeyForecastStageView{
			Label:             "After current reading coverage",
			Coverage:          after,
			Comparison:        current,
			ComparisonLabel:   "Current coverage",
			UnavailableReason: afterReason,
			Qualifier:         qualifier,
			Context:           "Modeled Known vocabulary plus the active current reading's frozen Reserved vocabulary after completion.",
		})
	} else {
		stages = append(stages, journeyForecastStageView{
			Label:     "After current reading coverage",
			Qualifier: "There is no current Book",
			Context:   "This stage requires an active current reading and its frozen Reserved vocabulary.",
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
			qualifier = "Lower bound: an earlier To Read book has unavailable evidence, so this value excludes any vocabulary it might contribute."
		}
		comparison := after
		comparisonLabel := "After current reading coverage"
		if !item.ForecastHasGoal {
			comparison = current
			comparisonLabel = "Current coverage"
		}
		stages = append(stages, journeyForecastStageView{
			Label:             "On arrival coverage",
			Coverage:          arrival,
			Comparison:        comparison,
			ComparisonLabel:   comparisonLabel,
			UnavailableReason: arrivalReason,
			Qualifier:         qualifier,
			Context:           "Modeled Known vocabulary plus trustworthy recurring vocabulary from earlier To Read books.",
		})
	}
	return stages
}
