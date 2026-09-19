package analysisinsights

import (
	"context"
	"errors"
	"fmt"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/lexical"
	"github.com/justin-hayes/mouseion/internal/selection"
)

// JourneyForecast computes the read-only, sequential coverage explanation for
// the learner's stored Journey order.
func (s *Service) JourneyForecast(ctx context.Context, owner, language string) (domain.JourneyForecast, error) {
	journeyStore, ok := s.store.(JourneyStore)
	if !ok {
		return domain.JourneyForecast{}, errors.New("journey forecast: store does not provide Journey")
	}
	evidenceStore, ok := s.store.(BookEvidenceStore)
	if !ok {
		return domain.JourneyForecast{}, errors.New("journey forecast: store does not provide book evidence")
	}
	goalStore, ok := s.store.(PrimaryGoalStore)
	if !ok {
		return domain.JourneyForecast{}, errors.New("journey forecast: store does not provide Primary Goal")
	}

	journey, err := journeyStore.GetReadingJourney(ctx, owner, language)
	if err != nil {
		return domain.JourneyForecast{}, fmt.Errorf("load reading journey: %w", err)
	}
	goal, err := goalStore.GetPrimaryGoal(ctx, owner, language)
	if err != nil {
		return domain.JourneyForecast{}, fmt.Errorf("load primary goal: %w", err)
	}
	evidence, err := evidenceStore.ListMyBooksWithEvidence(ctx, owner)
	if err != nil {
		return domain.JourneyForecast{}, fmt.Errorf("load book evidence: %w", err)
	}

	knownRows, err := s.store.ListKnownVocabulary(ctx, owner, language)
	if err != nil {
		return domain.JourneyForecast{}, fmt.Errorf("list known vocabulary for %s: %w", language, err)
	}
	known := make(map[selection.Identity]struct{}, len(knownRows))
	for _, word := range knownRows {
		known[selection.Identity{Language: word.Language, CanonicalLemma: word.CanonicalLemma, UPOS: word.UPOS}] = struct{}{}
	}
	reservedRows, err := s.store.ListReservedVocabulary(ctx, owner, language)
	if err != nil {
		return domain.JourneyForecast{}, fmt.Errorf("list reserved vocabulary for %s: %w", language, err)
	}
	reserved := make(map[selection.Identity]struct{}, len(reservedRows))
	for _, word := range reservedRows {
		reserved[selection.Identity{Language: word.Language, CanonicalLemma: word.CanonicalLemma, UPOS: word.UPOS}] = struct{}{}
	}
	goalVocabulary := make(map[selection.Identity]struct{})
	if goal.IsActive() && goal.SnapshotID != "" {
		snapshotStore, ok := s.store.(GoalSnapshotStore)
		if !ok {
			return domain.JourneyForecast{}, errors.New("journey forecast: store does not provide Goal snapshot vocabulary")
		}
		snapshot, snapshotErr := snapshotStore.ListPrimaryGoalSnapshotVocabulary(ctx, owner, goal.SnapshotID)
		if snapshotErr != nil {
			return domain.JourneyForecast{}, fmt.Errorf("load primary goal snapshot: %w", snapshotErr)
		}
		for _, candidate := range snapshot {
			if candidate.Language == language && lexical.IsLemma(candidate.CanonicalLemma) {
				goalVocabulary[selection.Identity{Language: candidate.Language, CanonicalLemma: candidate.CanonicalLemma, UPOS: candidate.UPOS}] = struct{}{}
			}
		}
	}

	byBook := make(map[string]domain.MyBook, len(evidence))
	for _, book := range evidence {
		byBook[book.Book.ID] = book
	}
	goalKnown := unionForecastVocabulary(known, goalVocabulary)
	goalInJourney := false
	var goalEntry domain.ReadingJourneyEntry
	for _, entry := range journey.Entries {
		if goal.IsActive() && entry.BookID == goal.BookID {
			goalInJourney = true
			goalEntry = entry
			break
		}
	}
	orderedEntries := journey.Entries
	if goalInJourney {
		orderedEntries = make([]domain.ReadingJourneyEntry, 0, len(journey.Entries))
		orderedEntries = append(orderedEntries, goalEntry)
		for _, entry := range journey.Entries {
			if entry.BookID != goal.BookID {
				orderedEntries = append(orderedEntries, entry)
			}
		}
	}
	accumulated := cloneForecastVocabulary(known)
	if goal.IsActive() && !goalInJourney {
		accumulated = cloneForecastVocabulary(goalKnown)
	}

	result := domain.JourneyForecast{OwnerID: owner, Language: language}
	if goal.IsActive() {
		goalCopy := goal
		result.Goal = &goalCopy
	}
	lowerBound := false
	for i, journeyEntry := range orderedEntries {
		position := journeyEntry.Position
		if position < 1 {
			position = i + 1
		}
		entry := domain.JourneyForecastEntry{BookID: journeyEntry.BookID, Position: position}
		if book, found := byBook[journeyEntry.BookID]; found && book.Acquired != nil {
			entry.SourceMaterialID = book.Acquired.Source.ID
			entry.Language = book.Acquired.Source.Language
			entry.CorpusID = book.Acquired.CorpusID
		}

		input, reason := s.forecastEvidence(ctx, owner, language, journeyEntry.BookID, byBook)
		if reason != "" {
			entry.UnavailableReason = reason
			if !goal.IsActive() || journeyEntry.BookID != goal.BookID {
				lowerBound = true
			}
			entry.LowerBound = lowerBound
			result.Entries = append(result.Entries, entry)
			if goal.IsActive() && journeyEntry.BookID == goal.BookID {
				accumulated = cloneForecastVocabulary(goalKnown)
			}
			continue
		}

		entry.Current = forecastCoverage(input, known)
		entry.AfterGoal = forecastCoverage(input, goalKnown)
		entry.OnArrival = forecastCoverage(input, accumulated)
		entry.LowerBound = lowerBound
		result.Entries = append(result.Entries, entry)

		if goal.IsActive() && journeyEntry.BookID == goal.BookID {
			accumulated = cloneForecastVocabulary(goalKnown)
			continue
		}
		accumulated = addRecurringVocabulary(accumulated, reserved, input)
	}
	return result, nil
}

func (s *Service) forecastEvidence(ctx context.Context, owner, language, bookID string, books map[string]domain.MyBook) (domain.AnalysisCorpusVocabulary, string) {
	book, found := books[bookID]
	if !found || book.Acquired == nil {
		return domain.AnalysisCorpusVocabulary{}, "unavailable: no current acquired source"
	}
	acquired := book.Acquired
	if acquired.Source.Language != language {
		return domain.AnalysisCorpusVocabulary{}, "unavailable: book language does not match the Journey"
	}
	if acquired.EvidenceState() == domain.BookStale {
		return domain.AnalysisCorpusVocabulary{}, "stale: analysis no longer matches the current book scope"
	}
	if acquired.EvidenceState() != domain.BookAnalyzed || acquired.AnalysisState != "completed" || acquired.AnalysisRunID == "" {
		return domain.AnalysisCorpusVocabulary{}, "unavailable: completed analysis evidence is unavailable"
	}
	if acquired.Source.ID == "" || acquired.Source.ContentRevisionID == "" || acquired.Source.ContentSnapshotID == "" {
		return domain.AnalysisCorpusVocabulary{}, "unavailable: current book content is unavailable"
	}
	if acquired.CorpusID == "" {
		return domain.AnalysisCorpusVocabulary{}, "unavailable: no current analyzed corpus"
	}
	input, err := s.store.GetAnalysisCorpusVocabulary(ctx, owner, acquired.CorpusID)
	if err != nil {
		return domain.AnalysisCorpusVocabulary{}, "unavailable: current analysis cannot be loaded"
	}
	if input.Statistics == nil {
		return domain.AnalysisCorpusVocabulary{}, "unavailable: corpus statistics are unavailable"
	}
	if input.SourceMaterialID != acquired.Source.ID {
		return domain.AnalysisCorpusVocabulary{}, "stale: corpus does not represent the current source"
	}
	if input.AnalysisRunID == "" || input.AnalysisRunID != acquired.AnalysisRunID {
		return domain.AnalysisCorpusVocabulary{}, "stale: corpus does not represent the current analysis run"
	}
	if !corpusLanguageMatches(input, language) {
		return domain.AnalysisCorpusVocabulary{}, "stale: corpus evidence is not modeled in the Journey language"
	}
	return input, ""
}

func corpusLanguageMatches(input domain.AnalysisCorpusVocabulary, language string) bool {
	for _, lemma := range input.Lemmas {
		if lemma.Language != language {
			return false
		}
	}
	return true
}

func forecastCoverage(input domain.AnalysisCorpusVocabulary, known map[selection.Identity]struct{}) *domain.JourneyForecastCoverage {
	result := &domain.JourneyForecastCoverage{AnalyzableTokenCount: input.Statistics.AnalyzableTokenCount}
	for _, lemma := range input.Lemmas {
		if !lexical.IsLemma(lemma.CanonicalLemma) {
			result.AnalyzableTokenCount = max(result.AnalyzableTokenCount-lemma.OccurrenceCount, 0)
			continue
		}
		if forecastKnown(known, selection.Identity{Language: lemma.Language, CanonicalLemma: lemma.CanonicalLemma, UPOS: lemma.UPOS}) {
			result.KnownTokenCount += lemma.OccurrenceCount
		}
	}
	result.KnownTokenCount = min(result.KnownTokenCount, result.AnalyzableTokenCount)
	return result
}

func addRecurringVocabulary(known, reserved map[selection.Identity]struct{}, input domain.AnalysisCorpusVocabulary) map[selection.Identity]struct{} {
	result := cloneForecastVocabulary(known)
	for _, lemma := range input.Lemmas {
		if !lexical.IsLemma(lemma.CanonicalLemma) || lemma.OccurrenceCount < selection.DefaultRecurringMinOccurrences {
			continue
		}
		identity := selection.Identity{Language: lemma.Language, CanonicalLemma: lemma.CanonicalLemma, UPOS: lemma.UPOS}
		if forecastKnown(result, identity) {
			continue
		}
		if _, isReserved := reserved[identity]; isReserved {
			continue
		}
		result[identity] = struct{}{}
	}
	return result
}

func forecastKnown(known map[selection.Identity]struct{}, identity selection.Identity) bool {
	if _, ok := known[identity]; ok {
		return true
	}
	_, ok := known[selection.Identity{Language: identity.Language, CanonicalLemma: identity.CanonicalLemma}]
	return ok
}

func unionForecastVocabulary(left, right map[selection.Identity]struct{}) map[selection.Identity]struct{} {
	result := cloneForecastVocabulary(left)
	for identity := range right {
		result[identity] = struct{}{}
	}
	return result
}

func cloneForecastVocabulary(source map[selection.Identity]struct{}) map[selection.Identity]struct{} {
	result := make(map[selection.Identity]struct{}, len(source))
	for identity := range source {
		result[identity] = struct{}{}
	}
	return result
}
