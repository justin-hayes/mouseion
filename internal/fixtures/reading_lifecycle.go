package fixtures

// Contract status: contractual. Current reading lifecycle transitions must
// match internal/storecontract scenarios shared with PostgreSQL (ADR 0088). The
// fixtures harness in storecontract_test.go runs them.

import (
	"context"
	"fmt"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

func (s *Store) ImportPreviouslyRead(_ context.Context, owner, bookID string) (domain.ReadingCompletion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := fixtureDispositionKey(owner, bookID)
	if completion, ok := s.importedHistory[key]; ok {
		return completion, nil
	}
	var book domain.MyBook
	found := false
	for _, candidate := range s.myBooksForOwner(owner) {
		if candidate.Book.ID == bookID {
			book, found = candidate, true
			break
		}
	}
	if !found || book.Book.LanguageState != domain.LanguageChosen || book.IsCurrentReading {
		return domain.ReadingCompletion{}, persistence.ErrNotFound
	}
	completion := domain.ReadingCompletion{
		OwnerID: owner, BookID: bookID, Language: book.Book.LanguageTag,
		CompletedAt: time.Now().UTC(), Source: domain.ReadingCompletionPreviouslyRead,
	}
	s.importedHistory[key] = completion
	return completion, nil
}

func (s *Store) GetCurrentReading(_ context.Context, owner, language string) (domain.CurrentReading, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	goal, ok := s.currentReadings[fixtureGoalKey(owner, language)]
	if !ok {
		return domain.CurrentReading{}, nil
	}
	return goal, nil
}

func (s *Store) StartCurrentReading(ctx context.Context, owner, language, bookID string) (domain.CurrentReading, error) {
	start, err := s.StartCurrentReadingResult(ctx, owner, language, bookID)
	return start.Reading, err
}

// StartCurrentReadingResult applies domain.DecideStart. A replayed Start
// returns the existing reading and writes nothing.
func (s *Store) StartCurrentReadingResult(_ context.Context, owner, language, bookID string) (persistence.StartResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	language = normalizeFixtureLanguage(language)
	goal := domain.CurrentReading{OwnerID: owner, Language: language, BookID: bookID}
	if err := goal.Validate(); err != nil {
		return persistence.StartResult{}, err
	}
	if !s.fixtureBookExists(owner, bookID) {
		return persistence.StartResult{}, errNotFound
	}
	bookID = s.fixtureBookID(owner, bookID)
	key := fixtureGoalKey(owner, language)
	decision := domain.DecideStart(s.fixtureStartFacts(owner, language, bookID))
	if err := persistence.StartDecisionError(decision); err != nil {
		return persistence.StartResult{}, err
	}
	if decision.Outcome == domain.StartReplayed {
		return persistence.StartResult{Reading: s.currentReadings[key], Replayed: true}, nil
	}
	now := time.Now()
	goal = s.fixtureGoalFromBook(owner, language, bookID, now)
	goal.CreatedAt, goal.UpdatedAt = now, now
	s.currentReadings[key] = goal
	s.snapshotLifecycles[goal.SnapshotID] = domain.CurrentReadingSnapshotLifecycle{BookID: bookID, CreatedAt: now}
	return persistence.StartResult{Reading: goal}, nil
}

// fixtureStartFacts loads the facts domain.DecideStart decides over. Fixture
// IDs are not uuids, so an ID Postgres would reject as malformed is rejected
// here the same way: it names no fixture Book and Start reports ErrNotFound.
func (s *Store) fixtureStartFacts(owner, language, bookID string) domain.StartFacts {
	current, existing := s.currentReadings[fixtureGoalKey(owner, language)]
	facts := domain.StartFacts{
		Language:                   language,
		AlreadyCurrent:             existing,
		CurrentIsBook:              existing && current.BookID == bookID,
		Disposition:                s.bookDispositionLocked(owner, bookID),
		UnresolvedLemmaReviewFlags: s.hasUnresolvedLemmaReviewFlag(owner, bookID),
	}
	for _, book := range s.books {
		if s.fixtureBookID(owner, book.Source.ID) != bookID {
			continue
		}
		facts.Signals = book.Signals
		facts.BookLanguage = normalizeFixtureLanguage(book.Source.Language)
		facts.IdentityPublished = true
		return facts
	}
	return facts
}

func (s *Store) SwitchCurrentReading(_ context.Context, owner, language, bookID, expectedBookID, expectedSnapshotID string) (domain.CurrentReading, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	language = normalizeFixtureLanguage(language)
	if !s.fixtureBookExists(owner, bookID) {
		return domain.CurrentReading{}, errNotFound
	}
	bookID = s.fixtureBookID(owner, bookID)
	key := fixtureGoalKey(owner, language)
	goal := s.currentReadings[key]
	facts := domain.SwitchCurrentReadingFacts{
		TargetBookID:      bookID,
		Expected:          domain.CurrentReadingCommitment{BookID: expectedBookID, SnapshotID: expectedSnapshotID},
		Current:           goal,
		ExpectedSnapshot:  s.snapshotLifecycle(expectedSnapshotID),
		CurrentSnapshot:   s.snapshotLifecycle(goal.SnapshotID),
		TargetEligibility: s.fixtureCurrentReadingEligibility(owner, language, bookID),
		UnresolvedFlags:   s.hasUnresolvedLemmaReviewFlag(owner, bookID),
	}
	decision := domain.DecideSwitchCurrentReading(facts)
	if err := fixtureCurrentReadingRejection(decision, persistence.ErrNotFound); err != nil {
		return domain.CurrentReading{}, err
	}
	if decision.Verdict == domain.CurrentReadingReplay {
		return goal, nil
	}
	now := time.Now()
	s.releaseSnapshotLocked(goal, now)
	next := s.fixtureGoalFromBook(owner, language, bookID, goal.CreatedAt)
	next.UpdatedAt = now
	s.currentReadings[key] = next
	s.snapshotLifecycles[next.SnapshotID] = domain.CurrentReadingSnapshotLifecycle{BookID: bookID, CreatedAt: now}
	return next, nil
}

// fixtureCurrentReadingRejection translates a rejecting decision to the same
// sentinel errors the Postgres store returns, and accepted or replayed
// decisions to nil.
func fixtureCurrentReadingRejection(decision domain.CurrentReadingDecision, noCurrent error) error {
	switch decision.Verdict {
	case domain.CurrentReadingApply, domain.CurrentReadingReplay:
		return nil
	case domain.CurrentReadingRejectNoCurrent:
		return noCurrent
	case domain.CurrentReadingRejectIneligible:
		return persistence.CurrentReadingIneligibleError{Reason: decision.Ineligible}
	case domain.CurrentReadingRejectUnresolvedFlags:
		return persistence.ErrUnresolvedLemmaReviewFlags
	case domain.CurrentReadingRejectStale:
		return persistence.ErrCurrentReadingStale
	}
	return persistence.ErrCurrentReadingStale
}

// snapshotLifecycle returns the recorded lifecycle of a snapshot, nil when none
// is recorded. A completion recorded in reading history marks it completed.
func (s *Store) snapshotLifecycle(snapshotID string) *domain.CurrentReadingSnapshotLifecycle {
	lifecycle, ok := s.snapshotLifecycles[snapshotID]
	if !ok {
		return nil
	}
	for _, completion := range s.readingHistory {
		if completion.GoalSnapshotID == snapshotID {
			lifecycle.Completed = true
			break
		}
	}
	return &lifecycle
}

// releaseSnapshotLocked records that goal's snapshot released its Reserved
// vocabulary at releasedAt.
func (s *Store) releaseSnapshotLocked(goal domain.CurrentReading, releasedAt time.Time) {
	if goal.SnapshotID == "" {
		return
	}
	lifecycle, ok := s.snapshotLifecycles[goal.SnapshotID]
	if !ok {
		lifecycle = domain.CurrentReadingSnapshotLifecycle{BookID: goal.BookID, CreatedAt: goal.CreatedAt}
	}
	lifecycle.ReleasedAt = releasedAt
	s.snapshotLifecycles[goal.SnapshotID] = lifecycle
}

func (s *Store) fixtureGoalFromBook(owner, language, bookID string, createdAt time.Time) domain.CurrentReading {
	sequenceKey := fixtureGoalKey(owner, language) + "\x00" + bookID
	if s.goalSnapshotSequence == nil {
		s.goalSnapshotSequence = make(map[string]int)
	}
	s.goalSnapshotSequence[sequenceKey]++
	snapshotID := fmt.Sprintf("fixture-goal-%s-%s-%d", language, bookID, s.goalSnapshotSequence[sequenceKey])
	if _, exists := s.goalSnapshotVocabulary[snapshotID]; !exists {
		baseSnapshotID := ""
		if language == "de" && bookID == routeMatchBookID {
			baseSnapshotID = "fixture-de-goal-snapshot"
		}
		if baseSnapshotID != "" {
			vocabulary := append([]domain.DeckPreparationVocabulary(nil), s.goalSnapshotVocabulary[baseSnapshotID]...)
			for i := range vocabulary {
				vocabulary[i].OwnerID = owner
				vocabulary[i].Language = language
			}
			s.goalSnapshotVocabulary[snapshotID] = vocabulary
		}
		if seeded, ok := s.bookSnapshotVocabulary[bookID]; ok {
			s.goalSnapshotVocabulary[snapshotID] = append([]domain.DeckPreparationVocabulary(nil), seeded...)
		}
	}
	goal := domain.CurrentReading{OwnerID: owner, Language: language, BookID: bookID, SnapshotID: snapshotID, CreatedAt: createdAt}
	goal.SnapshotSize = len(s.goalSnapshotVocabulary[goal.SnapshotID])
	for _, book := range s.books {
		if book.Source.OwnerID != owner || s.fixtureBookID(owner, book.Source.ID) != bookID {
			continue
		}
		goal.SourceMaterialID = book.Source.ID
		goal.AnalysisRunID = book.AnalysisRunID
		goal.ContentRevisionID = book.Source.ContentRevisionID
		goal.ContentSnapshotID = book.Source.ContentSnapshotID
		goal.CorpusID = book.CorpusID
		break
	}
	return goal
}

// fixtureCurrentReadingEligibility classifies a Book's Analysis evidence for a
// study language with the same domain rules as the Postgres store.
func (s *Store) fixtureCurrentReadingEligibility(owner, language, bookID string) domain.CurrentReadingEligibilityReason {
	language = normalizeFixtureLanguage(language)
	disposition := s.bookDispositionLocked(owner, bookID)
	for _, book := range s.books {
		if s.fixtureBookID(owner, book.Source.ID) != bookID {
			continue
		}
		bookLanguage := normalizeFixtureLanguage(book.Source.Language)
		return domain.CurrentReadingEligibilityIn(domain.ClassifyBookEvidence(book.Signals, disposition, bookLanguage), bookLanguage, language)
	}
	return domain.CurrentReadingNoCompletedAnalysis
}

// EndCurrentReading clears the exact expected commitment. domain.DecideEndCurrentReading
// accepts, rejects, or proves a replay from the recorded snapshot lifecycle.
func (s *Store) EndCurrentReading(_ context.Context, owner, language, expectedBookID, expectedSnapshotID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	language = normalizeFixtureLanguage(language)
	key := fixtureGoalKey(owner, language)
	goal := s.currentReadings[key]
	decision := domain.DecideEndCurrentReading(domain.EndCurrentReadingFacts{
		Expected: domain.CurrentReadingCommitment{BookID: expectedBookID, SnapshotID: expectedSnapshotID},
		Current:  goal,
		Snapshot: s.snapshotLifecycle(expectedSnapshotID),
	})
	if err := fixtureCurrentReadingRejection(decision, persistence.ErrCurrentReadingStale); err != nil {
		return err
	}
	if decision.Verdict == domain.CurrentReadingReplay {
		return nil
	}
	s.releaseSnapshotLocked(goal, time.Now())
	delete(s.currentReadings, key)
	return nil
}

// FinishCurrentReading applies domain.PlanCurrentReadingFinish to the fixture's
// facts: it records the reading fact, accepts the frozen snapshot into modeled
// Known vocabulary, returns the Book to Inbox, and ends the current reading. A
// replay for a finished snapshot returns the recorded completion.
func (s *Store) FinishCurrentReading(_ context.Context, owner, language, expectedBookID, expectedSnapshotID string) (domain.CurrentReadingFinishResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	language = normalizeFixtureLanguage(language)
	key := fixtureGoalKey(owner, language)
	historyKey := fixtureReadingHistoryKey(owner, language, expectedSnapshotID)
	goal := s.currentReadings[key]
	facts := domain.CurrentReadingFinishFacts{
		Current: goal, ExpectedBookID: expectedBookID, ExpectedSnapshotID: expectedSnapshotID,
		Snapshot: s.fixtureSnapshotIdentitiesLocked(owner, language), Known: s.knownForOwnerLocked(owner, language),
	}
	completion, completed := s.readingHistory[historyKey]
	if completed {
		facts.Completion = &domain.CurrentReadingCompletion{BookID: completion.BookID}
	}
	decision := domain.PlanCurrentReadingFinish(facts)
	switch decision.Outcome {
	case domain.CurrentReadingFinishRejected:
		if decision.Rejection == domain.CurrentReadingFinishUnknown {
			return domain.CurrentReadingFinishResult{}, persistence.ErrNotFound
		}
		return domain.CurrentReadingFinishResult{}, persistence.ErrCurrentReadingStale
	case domain.CurrentReadingFinishReplayed:
		// The recorded completion is the result.
	case domain.CurrentReadingFinishPlanned:
		plan := decision.Plan
		now := time.Now()
		for _, identity := range plan.Accept {
			s.known = append(s.known, domain.KnownVocabulary{
				OwnerID: owner, Language: language, CanonicalLemma: identity.CanonicalLemma,
				UPOS: identity.UPOS, CreatedAt: now,
			})
		}
		completion = domain.ReadingCompletion{
			OwnerID: owner, Language: language, BookID: expectedBookID, CompletedAt: now,
			GoalSnapshotID: goal.SnapshotID, SnapshotVocabularyCount: plan.SnapshotCount,
			EligibleVocabularyCount: plan.EligibleCount, GraduatedVocabularyCount: plan.NewlyKnownCount,
			AlreadyKnownVocabularyCount: plan.AlreadyKnownCount, Source: domain.ReadingCompletionCurrentReading,
		}
		s.readingHistory[historyKey] = completion
	}
	result := domain.CurrentReadingFinishResult{Completion: domain.CurrentReadingCompletion{
		OwnerID: completion.OwnerID, Language: completion.Language, BookID: completion.BookID,
		CompletedAt: completion.CompletedAt, SnapshotID: completion.GoalSnapshotID,
		SnapshotVocabularyCount:     completion.SnapshotVocabularyCount,
		EligibleVocabularyCount:     completion.EligibleVocabularyCount,
		GraduatedVocabularyCount:    completion.GraduatedVocabularyCount,
		AlreadyKnownVocabularyCount: completion.AlreadyKnownVocabularyCount,
	}}
	if !goal.IsActive() {
		return result, nil
	}
	now := time.Now()
	for index := range s.preps {
		preparation := &s.preps[index]
		if preparation.OwnerID != owner || preparation.GraduatedAt != nil || preparation.StudyingAt == nil {
			continue
		}
		matchesSnapshot := preparation.SnapshotID != "" && preparation.SnapshotID == goal.SnapshotID
		matchesLegacyIdentity := preparation.SourceMaterialID == goal.SourceMaterialID && preparation.AnalysisRunID == goal.AnalysisRunID
		if !matchesSnapshot && !matchesLegacyIdentity {
			continue
		}
		preparation.StudyingAt = nil
		preparation.ReleasedAt = &now
		preparation.UpdatedAt = now
	}
	s.dispositions[fixtureDispositionKey(owner, expectedBookID)] = domain.BookDispositionInbox
	s.releaseSnapshotLocked(goal, now)
	delete(s.currentReadings, key)
	return result, nil
}
