// Package ancientgreek registers the compiled, reviewed classical Reading unit
// definitions this installation ships, starting with Xenophon's Anabasis Book 1.
// Registration creates an unprepared Book; importing its pinned annotation is a
// separate step.
package ancientgreek

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/justin-hayes/mouseion/internal/domain"
)

const (
	// Language is the canonical language tag Ancient Greek is registered under.
	Language = "grc"
	// DisplayName is the language drop-down name for Language.
	DisplayName = "Ancient Greek"
)

// AnabasisBook1 is the reviewed definition of Xenophon's Anabasis Book 1. The
// pinned file is the whole Anabasis in the single-file CoNLL-U packaging of
// oga_src release 0.2.0 (Zenodo DOI 10.5281/zenodo.15199546); size and SHA-256
// are the reviewed commitments the import checks.
var AnabasisBook1 = domain.ReadingUnitDefinition{
	Key:                   "xenophon-anabasis-1",
	Language:              Language,
	WorkTitle:             "Anabasis",
	WorkAuthor:            "Xenophon",
	EditionLabel:          "Marchant 1904",
	CitationStart:         "1.1.1",
	CitationEnd:           "1.10.19",
	SourceFileName:        "tlg0032.tlg006.perseus-grc2_oga020_src.conllu",
	SourceFileSize:        7438037,
	SourceFileSHA256:      "c51bba2df6e80e2eb8dd2543d00cf6d6e6b8c65cc9ed03c7d27176fed063eb61",
	InterpretationVersion: "oga-agdt-upos/4",
}

// Store is the persistence registration needs.
type Store interface {
	GetUserByUsername(context.Context, string) (domain.User, string, error)
	PutSupportedLanguage(context.Context, string, string) (domain.SupportedLanguage, error)
	ListLegacyLanguageRows(context.Context, string) ([]domain.LegacyLanguageRows, error)
	RegisterReadingUnitBook(context.Context, string, domain.ReadingUnitDefinition) (domain.ReadingUnitBookRegistration, error)
}

// Service registers compiled Reading unit definitions for named learners.
type Service struct {
	store Store
}

func NewService(store Store) *Service {
	return &Service{store: store}
}

// Report is the outcome of one registration. It names only codes and
// identifiers, never text from the definition.
type Report struct {
	Username string
	BookID   string
	Created  bool
}

// Register refuses when the definition's language already has rows on a
// pre-existing surface, then ensures the language's display name and gives the
// learner a Book for the definition's Reading unit. It is idempotent: a repeat
// returns the existing Book.
func (s *Service) Register(ctx context.Context, username string, definition domain.ReadingUnitDefinition) (Report, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return Report{}, errors.New("ancientgreek: learner username is required")
	}
	if err := definition.Validate(); err != nil {
		return Report{}, err
	}
	learner, _, err := s.store.GetUserByUsername(ctx, username)
	if err != nil {
		return Report{}, fmt.Errorf("ancientgreek: resolve learner %q: %w", username, err)
	}
	legacy, err := s.store.ListLegacyLanguageRows(ctx, definition.Language)
	if err != nil {
		return Report{}, fmt.Errorf("ancientgreek: inspect %s rows: %w", definition.Language, err)
	}
	if len(legacy) > 0 {
		return Report{}, legacyRowsError(definition.Language, legacy)
	}
	if _, err := s.store.PutSupportedLanguage(ctx, definition.Language, DisplayName); err != nil {
		return Report{}, fmt.Errorf("ancientgreek: ensure display name: %w", err)
	}
	registration, err := s.store.RegisterReadingUnitBook(ctx, learner.ID, definition)
	if err != nil {
		return Report{}, fmt.Errorf("ancientgreek: register book: %w", err)
	}
	return Report{Username: username, BookID: registration.BookID, Created: registration.Created}, nil
}

// legacyRowsError refuses registration, naming each owner, surface and count.
// It reports codes and counts only, never any stored text.
func legacyRowsError(language string, rows []domain.LegacyLanguageRows) error {
	var b strings.Builder
	fmt.Fprintf(&b, "ancientgreek: refusing to register %s: legacy %s rows exist", DisplayName, language)
	for _, row := range rows {
		fmt.Fprintf(&b, "; owner=%s surface=%s count=%d", row.OwnerID, row.Surface, row.Count)
	}
	return errors.New(b.String())
}
