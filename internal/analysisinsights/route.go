package analysisinsights

import (
	"context"
	"fmt"
	"math/big"
	"sort"

	"github.com/justin-hayes/mouseion/internal/domain"
)

// JourneyProjection computes the current and conditional advisory orders from
// the current Journey and evidence. It deliberately performs no writes.
func (s *Service) JourneyProjection(ctx context.Context, owner, language string) (domain.JourneyProjectionResult, error) {
	journeyStore, ok := s.store.(JourneyStore)
	if !ok {
		return domain.JourneyProjectionResult{}, fmt.Errorf("journey projection: store does not provide Journey")
	}
	evidenceStore, ok := s.store.(BookEvidenceStore)
	if !ok {
		return domain.JourneyProjectionResult{}, fmt.Errorf("journey projection: store does not provide book evidence")
	}
	journey, err := journeyStore.GetReadingJourney(ctx, owner)
	if err != nil {
		return domain.JourneyProjectionResult{}, fmt.Errorf("load reading journey: %w", err)
	}
	evidence, err := evidenceStore.ListMyBooksWithEvidence(ctx, owner)
	if err != nil {
		return domain.JourneyProjectionResult{}, fmt.Errorf("load book evidence: %w", err)
	}
	byBook := make(map[string]domain.MyBook, len(evidence))
	for _, book := range evidence {
		byBook[book.Book.ID] = book
	}
	result := domain.JourneyProjectionResult{OwnerID: owner}
	conditionalAvailable := false
	for i, entry := range journey.Entries {
		position := entry.Position
		if position < 1 {
			// Keep test/durable-read-model compatibility for callers that provide
			// the already ordered slice but omit its redundant position.
			position = i + 1
		}
		book := domain.JourneyRouteBook{BookID: entry.BookID, Position: position}
		item, found := byBook[entry.BookID]
		if found && item.Acquired != nil {
			book.SourceMaterialID = item.Acquired.Source.ID
			book.Language = item.Acquired.Source.Language
			book.CorpusID = item.Acquired.CorpusID
		}
		if language == "" && result.Language == "" && book.Language != "" {
			result.Language = book.Language
		}
		result.LearnerOrder = append(result.LearnerOrder, book)
	}
	if language != "" {
		result.Language = language
	}
	if goals, ok := s.store.(PrimaryGoalStore); ok {
		goal, goalErr := goals.GetPrimaryGoal(ctx, owner)
		if goalErr != nil {
			return domain.JourneyProjectionResult{}, fmt.Errorf("load primary goal: %w", goalErr)
		}
		for i := range result.LearnerOrder {
			if result.LearnerOrder[i].BookID == goal.BookID {
				result.LearnerOrder[i].Fixed = true
			}
		}
	}

	for i := range result.LearnerOrder {
		book := &result.LearnerOrder[i]
		item, found := byBook[book.BookID]
		switch {
		case !found || item.Acquired == nil:
			book.IncomparableReason = "unavailable: no current acquired source"
		case book.Language == "":
			book.IncomparableReason = "unavailable: book language is unknown"
		case item.EvidenceState == domain.MyBookStale:
			book.IncomparableReason = "stale: analysis no longer matches the current book scope"
		case result.Language != "" && book.Language != result.Language:
			book.IncomparableReason = "different study language"
		case book.CorpusID == "":
			book.IncomparableReason = "unassessed: no current analyzed corpus"
		default:
			input, loadErr := s.store.GetAnalysisCorpusVocabulary(ctx, owner, book.CorpusID)
			if loadErr != nil {
				book.IncomparableReason = "unavailable: current analysis cannot be loaded"
			} else if input.Statistics == nil {
				book.IncomparableReason = "stale/incomplete: corpus statistics unavailable"
			} else if input.SourceMaterialID != book.SourceMaterialID {
				book.IncomparableReason = "stale: corpus does not represent the current source"
			} else if !corpusLanguageMatches(input, book.Language) {
				book.IncomparableReason = "different study language: corpus evidence is not modeled language"
			} else {
				current, coverageErr := s.coverage(ctx, owner, input, false)
				if coverageErr != nil {
					return domain.JourneyProjectionResult{}, coverageErr
				}
				book.Coverage, book.Comparable = &current, true
				reservations, reservationErr := s.store.ListActiveLearningCampaignVocabulary(ctx, owner, book.Language)
				if reservationErr != nil {
					return domain.JourneyProjectionResult{}, fmt.Errorf("list active campaign vocabulary for %s: %w", book.Language, reservationErr)
				}
				if len(reservations) > 0 {
					conditionalAvailable = true
					conditional, conditionalErr := s.coverage(ctx, owner, input, true)
					if conditionalErr != nil {
						return domain.JourneyProjectionResult{}, conditionalErr
					}
					book.ConditionalCoverage = &conditional
				}
			}
		}
		if book.Comparable {
			result.ComparableCount++
		} else {
			result.IncomparableCount++
		}
	}
	if err := fillAdvisory(&result, false); err != nil {
		return result, err
	}
	if conditionalAvailable {
		if err := fillAdvisory(&result, true); err != nil {
			return result, err
		}
	}
	return result, nil
}

// RouteProjection is the concise alias used by callers that treat the
// Journey comparison as a route projection.
func (s *Service) RouteProjection(ctx context.Context, owner, language string) (domain.JourneyProjectionResult, error) {
	return s.JourneyProjection(ctx, owner, language)
}

func fillAdvisory(result *domain.JourneyProjectionResult, conditional bool) error {
	items := append([]domain.JourneyRouteBook(nil), result.LearnerOrder...)
	// Incomparable slots are immovable. Fixed comparable books occupy the first
	// comparable slot, so an incomparable book at learner position one still
	// remains there without preventing a Primary Goal from being anchored at
	// the front of the comparable route.
	var fixed *domain.JourneyRouteBook
	for i := range items {
		if items[i].Comparable && items[i].Fixed {
			fixed = &items[i]
			break
		}
	}
	var movable []domain.JourneyRouteBook
	for _, item := range items {
		if item.Comparable && !item.Fixed {
			movable = append(movable, item)
		}
	}
	sort.SliceStable(movable, func(i, j int) bool {
		a, b := movable[i], movable[j]
		ca, cb := a.Coverage, b.Coverage
		if conditional {
			ca, cb = a.ConditionalCoverage, b.ConditionalCoverage
		}
		if comparison := compareCoverage(ca, cb); comparison != 0 {
			return comparison > 0
		}
		if a.Position != b.Position {
			return a.Position < b.Position
		}
		if a.Language != b.Language {
			return a.Language < b.Language
		}
		return a.BookID < b.BookID
	})
	resultOrder := make([]domain.JourneyRouteBook, len(items))
	mi := 0
	firstComparable := true
	for i, item := range items {
		if !item.Comparable {
			item.PlacementReason = item.IncomparableReason
			resultOrder[i] = item
			continue
		}
		if firstComparable && fixed != nil {
			item = *fixed
			item.PlacementReason = "fixed: Primary Goal anchored at first comparable Journey position"
			firstComparable = false
		} else {
			item = movable[mi]
			mi++
			item.PlacementReason = "ranked by current known-token coverage"
			firstComparable = false
		}
		if !item.Comparable {
			item.PlacementReason = item.IncomparableReason
		}
		resultOrder[i] = item
	}
	rank := 0
	for i := range resultOrder {
		if resultOrder[i].Comparable {
			rank++
			value := rank
			resultOrder[i].Rank = &value
		}
	}
	if conditional {
		result.ConditionalAdvisoryOrder = resultOrder
	} else {
		result.AdvisoryOrder = resultOrder
	}
	return nil
}

func compareCoverage(a, b *domain.AnalysisCoverage) int {
	if a == nil || b == nil {
		return 0
	}
	// Cross multiplication preserves the exact ADR 0025 integer comparison,
	// including for counts too large for a machine-int64 product.
	left := new(big.Int).Mul(big.NewInt(a.KnownTokenCount), big.NewInt(b.AnalyzableTokenCount))
	right := new(big.Int).Mul(big.NewInt(b.KnownTokenCount), big.NewInt(a.AnalyzableTokenCount))
	return left.Cmp(right)
}

func corpusLanguageMatches(input domain.AnalysisCorpusVocabulary, language string) bool {
	for _, lemma := range input.Lemmas {
		if lemma.Language != language {
			return false
		}
	}
	return true
}
