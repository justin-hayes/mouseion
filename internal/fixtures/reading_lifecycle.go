package fixtures

// Contract status: contractual. Current reading lifecycle transitions must
// match internal/storecontract scenarios shared with PostgreSQL (ADR 0088). The
// fixtures harness in storecontract_test.go runs them.

import (
	"context"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
)

func (s *Store) ImportPreviouslyRead(_ context.Context, owner, bookID string) (domain.ReadingCompletion, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := fixtureDispositionKey(owner, bookID)
	if completion, ok := s.importedHistory[key]; ok {
		return completion, nil
	}
	if !s.markFixturePreviouslyReadLocked(owner, bookID) {
		return domain.ReadingCompletion{}, domain.ErrNotFound
	}
	return s.importedHistory[key], nil
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

// StartCurrentReadingResult is Read next: it applies domain.DecideStart and
// makes the Book the current reading, replacing any other. A replayed Start
// returns the existing reading and writes nothing.
func (s *Store) StartCurrentReadingResult(_ context.Context, owner, language, bookID string) (domain.StartResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	language = normalizeFixtureLanguage(language)
	goal := domain.CurrentReading{OwnerID: owner, Language: language, BookID: bookID}
	if err := goal.Validate(); err != nil {
		return domain.StartResult{}, err
	}
	if !s.fixtureBookExists(owner, bookID) {
		return domain.StartResult{}, errNotFound
	}
	bookID = s.fixtureBookID(owner, bookID)
	key := fixtureGoalKey(owner, language)
	decision := domain.DecideStart(s.fixtureStartFacts(owner, language, bookID))
	if err := domain.StartDecisionError(decision); err != nil {
		return domain.StartResult{}, err
	}
	if decision.Outcome == domain.StartReplayed {
		return domain.StartResult{Reading: s.currentReadings[key], Replayed: true}, nil
	}
	now := time.Now()
	createdAt := now
	if previous, ok := s.currentReadings[key]; ok {
		createdAt = previous.CreatedAt
	}
	goal = s.fixtureGoalFromBook(owner, language, bookID, createdAt)
	goal.UpdatedAt = now
	s.currentReadings[key] = goal
	return domain.StartResult{Reading: goal}, nil
}

// fixtureStartFacts loads the facts domain.DecideStart decides over. Fixture
// IDs are not uuids, so an ID Postgres would reject as malformed is rejected
// here the same way: it names no fixture Book and Start reports ErrNotFound.
func (s *Store) fixtureStartFacts(owner, language, bookID string) domain.StartFacts {
	current, existing := s.currentReadings[fixtureGoalKey(owner, language)]
	facts := domain.StartFacts{
		Language:      language,
		CurrentIsBook: existing && current.BookID == bookID,
		Disposition:   s.bookDispositionLocked(owner, bookID),
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

func (s *Store) fixtureGoalFromBook(owner, language, bookID string, createdAt time.Time) domain.CurrentReading {
	return domain.CurrentReading{OwnerID: owner, Language: language, BookID: bookID, CreatedAt: createdAt}
}

// EndCurrentReading clears the language's current reading. It names no
// commitment; domain.ErrNotFound reports that none was current.
func (s *Store) EndCurrentReading(_ context.Context, owner, language string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := fixtureGoalKey(owner, normalizeFixtureLanguage(language))
	if _, ok := s.currentReadings[key]; !ok {
		return domain.ErrNotFound
	}
	delete(s.currentReadings, key)
	return nil
}

// FinishCurrentReading appends a Reading completion for the language's current
// reading, returns its Book to the Inbox, and clears the role. It has no
// vocabulary effect; domain.ErrNotFound reports that none was current.
func (s *Store) FinishCurrentReading(_ context.Context, owner, language string) (domain.CurrentReadingFinishResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	language = normalizeFixtureLanguage(language)
	key := fixtureGoalKey(owner, language)
	goal, ok := s.currentReadings[key]
	if !ok {
		return domain.CurrentReadingFinishResult{}, domain.ErrNotFound
	}
	now := time.Now()
	completion := domain.ReadingCompletion{
		OwnerID: owner, Language: language, BookID: goal.BookID, CompletedAt: now,
		Source: domain.ReadingCompletionCurrentReading,
	}
	s.readingHistory[fixtureReadingHistoryKey(owner, language, len(s.readingHistory))] = completion
	s.dispositions[fixtureDispositionKey(owner, goal.BookID)] = domain.BookDispositionInbox
	delete(s.currentReadings, key)
	return domain.CurrentReadingFinishResult{Completion: domain.CurrentReadingCompletion{
		OwnerID: owner, Language: language, BookID: goal.BookID, CompletedAt: now,
	}}, nil
}
