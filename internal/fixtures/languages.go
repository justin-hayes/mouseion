package fixtures

// Contract status: illustrative. Canned state for browser scenarios; not held
// to internal/storecontract parity (ADR 0088).

import (
	"context"
	"sort"
	"strings"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
)

var fixtureSyncArrivals = []domain.SupportedLanguage{
	{Language: "es", DisplayName: "Spanish"},
	{Language: "nl", DisplayName: "Dutch"},
	{Language: "pt", DisplayName: "Portuguese"},
	{Language: "sv", DisplayName: "Swedish"},
}

func normalizeFixtureLanguage(raw string) string {
	return canonicalization.NormalizeLanguage(raw)
}

func fixtureLanguageKey(owner, language string) string {
	return owner + "\x00" + normalizeFixtureLanguage(language)
}

func (s *Store) GetStoredActiveStudyLanguage(_ context.Context, owner string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if owner != OwnerID {
		return "", errNotFound
	}
	if s.storedActiveLanguage == nil {
		return "", nil
	}
	return *s.storedActiveLanguage, nil
}

func (s *Store) SetActiveStudyLanguage(_ context.Context, owner, language string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if owner != OwnerID {
		return errNotFound
	}
	language = normalizeFixtureLanguage(language)
	if language == "" {
		s.storedActiveLanguage = nil
	} else {
		s.storedActiveLanguage = &language
	}
	return nil
}

func (s *Store) MostRecentlyActivatedStudyLanguage(_ context.Context, owner string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if owner != OwnerID {
		return "", errNotFound
	}
	return s.mostRecentLanguage, nil
}

func (s *Store) arriveNextFixtureStudyLanguage() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, arrival := range fixtureSyncArrivals {
		language := normalizeFixtureLanguage(arrival.Language)
		alreadyPresent := false
		for _, book := range s.books {
			if book.Source.OwnerID == OwnerID && normalizeFixtureLanguage(book.Source.Language) == language {
				alreadyPresent = true
				break
			}
		}
		if alreadyPresent {
			continue
		}
		s.books = append(s.books, domain.SourceMaterialSummary{
			Source: domain.SourceMaterial{
				ID:        "fixture-arrival-" + language,
				OwnerID:   OwnerID,
				Language:  language,
				Title:     arrival.DisplayName + " arrival",
				MediaType: "application/epub+zip",
			},
			BookID: "fixture-arrival-" + language,
		})
		s.supported = append(s.supported, arrival)
		s.mostRecentLanguage = language
		return
	}
}

func (s *Store) PutSupportedLanguage(context.Context, string, string) (domain.SupportedLanguage, error) {
	return domain.SupportedLanguage{}, nil
}

func (s *Store) ListSupportedLanguages(_ context.Context) ([]domain.SupportedLanguage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]domain.SupportedLanguage(nil), s.supported...), nil
}

func (s *Store) SyncSupportedLanguages(context.Context, []domain.SupportedLanguage) error { return nil }

func (s *Store) ListStudyLanguages(_ context.Context, owner string) ([]domain.StudyLanguage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// Fixture source summaries represent the active acquired library books; the
	// fixture does not model a separate membership row for them.
	seen := make(map[string]struct{})
	displayNames := make(map[string]string, len(s.supported))
	for _, language := range s.supported {
		tag := normalizeFixtureLanguage(language.Language)
		if tag != "" {
			displayNames[tag] = language.DisplayName
		}
	}
	var out []domain.StudyLanguage
	addStudyLanguage := func(raw string) {
		language := normalizeFixtureLanguage(raw)
		if language == "" {
			return
		}
		if _, ok := seen[language]; ok {
			return
		}
		seen[language] = struct{}{}
		displayName := displayNames[language]
		if displayName == "" {
			displayName = language
		}
		out = append(out, domain.StudyLanguage{Language: language, DisplayName: displayName})
	}
	for _, book := range s.books {
		if book.Source.OwnerID != owner || strings.TrimSpace(book.Source.Language) == "" {
			continue
		}
		addStudyLanguage(book.Source.Language)
	}
	for _, book := range s.myBooks {
		if book.Book.OwnerID != owner || book.Book.LanguageState != domain.LanguageChosen || strings.TrimSpace(book.Book.LanguageTag) == "" {
			continue
		}
		addStudyLanguage(book.Book.LanguageTag)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Language < out[j].Language })
	return out, nil
}

func (s *Store) ListKnownVocabularyLanguages(_ context.Context, owner string) ([]domain.StudyLanguage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	seen := make(map[string]struct{})
	displayNames := make(map[string]string, len(s.supported))
	for _, language := range s.supported {
		tag := normalizeFixtureLanguage(language.Language)
		if tag != "" {
			displayNames[tag] = language.DisplayName
		}
	}
	var out []domain.StudyLanguage
	for _, entry := range s.known {
		if entry.OwnerID != owner {
			continue
		}
		language := normalizeFixtureLanguage(entry.Language)
		if language == "" {
			continue
		}
		if _, ok := seen[language]; ok {
			continue
		}
		seen[language] = struct{}{}
		name := displayNames[language]
		if name == "" {
			name = language
		}
		out = append(out, domain.StudyLanguage{Language: language, DisplayName: name})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Language < out[j].Language })
	return out, nil
}

type Capabilities struct{}

func (Capabilities) GetCapabilities(context.Context) (analyzer.Capabilities, error) {
	return analyzer.Capabilities{Languages: []analyzer.LanguageCapability{{Language: "de", DisplayName: "German", Ready: true}, {Language: "it", DisplayName: "Italian", Ready: true}}, Degraded: false}, nil
}
