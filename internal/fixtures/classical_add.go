package fixtures

// Contract status: contractual. The Classical catalog Add transition must
// match internal/storecontract scenarios shared with PostgreSQL (ADR 0088).
// The fixtures harness in storecontract_test.go runs them.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
)

func fixtureClassicalBookID(owner, key string) string {
	return "fixture-classical-" + key + "-" + owner
}

// ListLegacyLanguageRows reports no pre-catalog Ancient Greek rows, so Add is
// available in the fixture.
func (s *Store) ListLegacyLanguageRows(context.Context, string) ([]domain.LegacyLanguageRows, error) {
	return nil, nil
}

// ListOwnedClassicalReadingUnits reports the fixture learner's Books that are
// associated with a Reading unit.
func (s *Store) ListOwnedClassicalReadingUnits(_ context.Context, owner string) ([]classical.OwnedReadingUnit, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var owned []classical.OwnedReadingUnit
	for _, book := range s.myBooks {
		if book.Book.OwnerID != owner || book.ReadingUnit == nil || strings.TrimSpace(book.ReadingUnit.ReadingUnitKey) == "" {
			continue
		}
		if !s.fixtureBookActiveLocked(owner, book.Book.ID) {
			continue
		}
		owned = append(owned, classical.OwnedReadingUnit{
			ReadingUnitKey: book.ReadingUnit.ReadingUnitKey,
			BookID:         book.Book.ID,
		})
	}
	return owned, nil
}

// AddClassicalReadingUnits creates or reopens one metadata-only To Read Book
// per current fixture Reading unit. The fixture preparation seam reports the
// text already prepared, so the Add route starts no real job.
func (s *Store) AddClassicalReadingUnits(_ context.Context, owner string, readingUnitKeys []string) (classical.AddReport, error) {
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return classical.AddReport{}, errors.New("fixtures: classical Add owner is required")
	}
	keys, err := fixtureClassicalAddKeys(readingUnitKeys)
	if err != nil {
		return classical.AddReport{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, key := range keys {
		if definition, ok := s.classicalDefinitions[key]; !ok || definition.Withdrawn {
			return classical.AddReport{}, fmt.Errorf("fixtures: classical Reading unit %q is not current", key)
		}
	}
	report := classical.AddReport{Books: make([]classical.AddedBook, 0, len(keys))}
	for _, key := range keys {
		report.Books = append(report.Books, s.addFixtureClassicalReadingUnitLocked(owner, s.classicalDefinitions[key]))
	}
	s.ensureFixtureSupportedLanguageLocked(classical.Language, classical.LanguageDisplayName)
	return report, nil
}

// setClassicalDefinitions installs the release the fixture Add resolves units
// from, so the shared store contract can seed its own small catalog.
func (s *Store) setClassicalDefinitions(definitions []classical.Definition) {
	byKey := make(map[string]classical.Definition, len(definitions))
	for _, definition := range definitions {
		byKey[definition.Key] = definition
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.classicalDefinitions = byKey
}

// addFixtureClassicalReadingUnitLocked creates the Book or reopens the existing
// one as To Read without changing its reading history or preparation state.
func (s *Store) addFixtureClassicalReadingUnitLocked(owner string, definition classical.Definition) classical.AddedBook {
	if i := s.fixtureClassicalBookIndexLocked(owner, definition.Key); i >= 0 {
		bookID := s.myBooks[i].Book.ID
		delete(s.removedBooks, fixtureDispositionKey(owner, bookID))
		s.restoreFixtureToReadLocked(owner, bookID)
		return classical.AddedBook{ReadingUnitKey: definition.Key, BookID: bookID}
	}
	book := s.newFixtureClassicalBookLocked(owner, definition)
	s.restoreFixtureToReadLocked(owner, book.ID)
	return classical.AddedBook{ReadingUnitKey: definition.Key, BookID: book.ID, Created: true}
}

// newFixtureClassicalBookLocked creates the metadata-only Book linked to one
// current Reading unit. Callers set the disposition and any preparation state.
func (s *Store) newFixtureClassicalBookLocked(owner string, definition classical.Definition) domain.Book {
	bookID := fixtureClassicalBookID(owner, definition.Key)
	book := domain.Book{
		ID:                 bookID,
		OwnerID:            owner,
		Title:              definition.WorkTitle,
		Author:             definition.WorkAuthor,
		MetadataProvenance: domain.MetadataProvenanceOperatorRegistration,
		LanguageState:      domain.LanguageChosen,
		LanguageTag:        classical.Language,
		CreatedAt:          fixtureJourneyTime,
		UpdatedAt:          fixtureJourneyTime,
	}
	unit := &domain.BookReadingUnit{
		OwnerID:          owner,
		BookID:           bookID,
		ReadingUnitID:    definition.Key,
		ReadingUnitKey:   definition.Key,
		WorkKey:          definition.WorkKey,
		WorkTitle:        definition.WorkTitle,
		WorkAuthor:       definition.WorkAuthor,
		OriginalTitle:    definition.OriginalTitle,
		CitationStart:    definition.CitationStart,
		CitationEnd:      definition.CitationEnd,
		EditionLabel:     definition.EditionLabel,
		ConstituentBooks: definition.ConstituentBooks,
		Revision:         definition.Revision,
		Withdrawn:        definition.Withdrawn,
	}
	if definition.ConstituentBook != nil {
		unit.ConstituentBook = *definition.ConstituentBook
	}
	s.myBooks = append(s.myBooks, domain.MyBook{
		Book:        book,
		Cover:       fixtureCover(bookID),
		ReadingUnit: unit,
	})
	return book
}

// restoreFixtureToReadLocked sets the Add's reading intent, advancing the
// disposition revision only when the disposition really changes.
func (s *Store) restoreFixtureToReadLocked(owner, bookID string) {
	key := fixtureDispositionKey(owner, bookID)
	if s.bookDispositionLocked(owner, bookID) != domain.BookDispositionToRead {
		s.dispositionRevisions[key] = s.bookDispositionRevisionLocked(owner, bookID) + 1
	}
	s.dispositions[key] = domain.BookDispositionToRead
}

func (s *Store) fixtureClassicalBookIndexLocked(owner, readingUnitKey string) int {
	for i := range s.myBooks {
		unit := s.myBooks[i].ReadingUnit
		if s.myBooks[i].Book.OwnerID == owner && unit != nil && unit.ReadingUnitKey == readingUnitKey {
			return i
		}
	}
	return -1
}

// ensureFixtureSupportedLanguageLocked adds the study-language display name
// once, as the PostgreSQL Add does.
func (s *Store) ensureFixtureSupportedLanguageLocked(language, displayName string) {
	language = normalizeFixtureLanguage(language)
	for _, supported := range s.supported {
		if normalizeFixtureLanguage(supported.Language) == language {
			return
		}
	}
	s.supported = append(s.supported, domain.SupportedLanguage{Language: language, DisplayName: displayName})
}

// fixtureClassicalAddKeys trims, rejects an empty key and removes duplicates
// while preserving constituent-book order.
func fixtureClassicalAddKeys(keys []string) ([]string, error) {
	if len(keys) == 0 {
		return nil, errors.New("fixtures: classical Add needs at least one Reading unit")
	}
	seen := make(map[string]struct{}, len(keys))
	out := make([]string, 0, len(keys))
	for _, key := range keys {
		key = strings.TrimSpace(key)
		if key == "" {
			return nil, errors.New("fixtures: classical Add Reading unit key is required")
		}
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	return out, nil
}
