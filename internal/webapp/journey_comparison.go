package webapp

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/justin-hayes/mouseion/internal/domain"
)

type journeyProjectionProvider interface {
	JourneyProjection(context.Context, string, string) (domain.JourneyProjectionResult, error)
}

type routeComparisonView struct {
	Language              string
	LearnerOrder          []routeComparisonBookView
	AdvisoryOrder         []routeComparisonBookView
	ConditionalOrder      []routeComparisonBookView
	ComparableCount       int
	IncomparableCount     int
	HasConditional        bool
	ComparisonUnavailable bool
}

type routeComparisonBookView struct {
	domain.JourneyRouteBook
	Title                    string
	EvidenceBadge            string
	EvidenceTone             StatusTone
	CurrentCoverageLabel     string
	ConditionalCoverageLabel string
	ThresholdLabel           string
	RankLabel                string
	PlacementReason          string
	IncomparableDetail       string
}

func routeEvidenceState(book domain.JourneyRouteBook) string {
	if book.Comparable && book.Coverage != nil {
		if book.ConditionalCoverage != nil {
			return "conditional"
		}
		return "current"
	}
	reason := strings.ToLower(book.IncomparableReason)
	if strings.Contains(reason, "stale") {
		return "stale"
	}
	return "unavailable"
}

func routeEvidenceLabel(book domain.JourneyRouteBook) string {
	switch routeEvidenceState(book) {
	case "current":
		return "Current"
	case "conditional":
		return "Conditional"
	case "stale":
		return "Stale"
	default:
		return "Coverage unavailable"
	}
}

func routeEvidenceTone(book domain.JourneyRouteBook) StatusTone {
	switch routeEvidenceState(book) {
	case "current":
		return StatusSuccess
	case "conditional":
		return StatusInfo
	case "stale":
		return StatusWarning
	default:
		return StatusDanger
	}
}

func routeCoverageLabel(coverage *domain.AnalysisCoverage) string {
	if coverage == nil {
		return "—"
	}
	return fmt.Sprintf("%.1f%%", knownCoveragePercent(*coverage))
}

func routeConditionalCoverageLabel(coverage *domain.AnalysisCoverage) string {
	if coverage == nil {
		return "—"
	}
	return fmt.Sprintf("%.1f%%", reservedCoveragePercent(*coverage))
}

func routeThresholdLabel(coverage *domain.AnalysisCoverage) string {
	if coverage == nil {
		return "unavailable"
	}
	labels := make([]string, 0, len(coverage.Thresholds))
	for _, threshold := range coverage.Thresholds {
		if threshold.Reachable {
			labels = append(labels, fmt.Sprintf("%d%%: %d lemmas", threshold.TargetPercent, threshold.LemmaCount))
		} else {
			labels = append(labels, fmt.Sprintf("%d%%: unavailable", threshold.TargetPercent))
		}
	}
	if len(labels) == 0 {
		return "not reported"
	}
	return strings.Join(labels, "; ")
}

func routeRankLabel(book domain.JourneyRouteBook) string {
	if book.Rank == nil || !book.Comparable {
		return "—"
	}
	return strconv.Itoa(*book.Rank)
}

func routeIncomparableDetail(book domain.JourneyRouteBook) string {
	if book.IncomparableReason == "" {
		return "No comparable evidence is available."
	}
	return book.IncomparableReason
}

func newRouteComparisonBookView(book domain.JourneyRouteBook, titles map[string]string) routeComparisonBookView {
	title := strings.TrimSpace(titles[book.BookID])
	if title == "" {
		title = book.BookID
	}
	view := routeComparisonBookView{
		JourneyRouteBook:         book,
		Title:                    title,
		EvidenceBadge:            routeEvidenceLabel(book),
		EvidenceTone:             routeEvidenceTone(book),
		CurrentCoverageLabel:     routeCoverageLabel(book.Coverage),
		ConditionalCoverageLabel: routeConditionalCoverageLabel(book.ConditionalCoverage),
		ThresholdLabel:           routeThresholdLabel(book.Coverage),
		RankLabel:                routeRankLabel(book),
		PlacementReason:          book.PlacementReason,
		IncomparableDetail:       routeIncomparableDetail(book),
	}
	if view.PlacementReason == "" && book.Comparable {
		view.PlacementReason = "ranked by current known-token coverage"
	}
	return view
}

func buildRouteComparisonView(result domain.JourneyProjectionResult, titles map[string]string) *routeComparisonView {
	view := &routeComparisonView{
		Language:              result.Language,
		ComparableCount:       result.ComparableCount,
		IncomparableCount:     result.IncomparableCount,
		ComparisonUnavailable: len(result.LearnerOrder) == 0 || len(result.AdvisoryOrder) == 0,
	}
	for _, book := range result.AdvisoryOrder {
		view.AdvisoryOrder = append(view.AdvisoryOrder, newRouteComparisonBookView(book, titles))
	}
	ranks := make(map[string]string, len(view.AdvisoryOrder))
	for _, book := range view.AdvisoryOrder {
		if book.Comparable {
			ranks[book.BookID] = book.RankLabel
		}
	}
	for _, book := range result.LearnerOrder {
		item := newRouteComparisonBookView(book, titles)
		if rank, ok := ranks[item.BookID]; ok {
			item.RankLabel = rank
		}
		view.LearnerOrder = append(view.LearnerOrder, item)
	}
	for _, book := range result.ConditionalAdvisoryOrder {
		view.ConditionalOrder = append(view.ConditionalOrder, newRouteComparisonBookView(book, titles))
	}
	view.HasConditional = len(view.ConditionalOrder) > 0
	return view
}

func journeyRouteComparison(ctx context.Context, provider journeyProjectionProvider, owner, language string, titles map[string]string) (*routeComparisonView, error) {
	result, err := provider.JourneyProjection(ctx, owner, language)
	if err != nil {
		return nil, err
	}
	return buildRouteComparisonView(result, titles), nil
}

func conditionalRouteDiffers(current, conditional []routeComparisonBookView) bool {
	if len(current) != len(conditional) {
		return true
	}
	for i := range current {
		if current[i].BookID != conditional[i].BookID {
			return true
		}
	}
	return false
}

func routePlacementLabel(item routeComparisonBookView) string {
	if item.Fixed {
		return "fixed: Primary Goal anchored at first comparable Journey position"
	}
	return item.PlacementReason
}

func routeIncomparableSuffix(item routeComparisonBookView) string {
	if item.Comparable {
		return ""
	}
	return " — " + item.IncomparableDetail
}
