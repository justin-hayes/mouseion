package persistence

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// classicalRegistrationLockSalt namespaces the per-owner/definition advisory
// lock taken while registering a Reading unit's Book.
const classicalRegistrationLockSalt = 469

// RegisterReadingUnitBook upserts one compiled definition's Work, Edition and
// Reading unit, then gives the owner a Book for that Reading unit. The
// definition rows stay authoritative, so a repeated call leaves them unchanged,
// and an owner who already has the Book gets it back without a second Book. A
// new Book is Inbox, chosen for the definition's language, and linked by an
// immutable association.
func (s *PostgresStore) RegisterReadingUnitBook(ctx context.Context, owner string, definition domain.ReadingUnitDefinition) (domain.ReadingUnitBookRegistration, error) {
	owner = strings.TrimSpace(owner)
	if owner == "" {
		return domain.ReadingUnitBookRegistration{}, errors.New("persistence: reading unit registration owner is required")
	}
	if err := definition.Validate(); err != nil {
		return domain.ReadingUnitBookRegistration{}, err
	}
	var result domain.ReadingUnitBookRegistration
	err := withTx(ctx, s.pool, func(ctx context.Context, tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, $2))`, owner+":"+definition.Key, classicalRegistrationLockSalt); err != nil {
			return err
		}
		q := sqlcgen.New(tx)
		readingUnitID, err := upsertReadingUnitDefinition(ctx, q, definition)
		if err != nil {
			return err
		}
		result, err = registerReadingUnitBookTx(ctx, q, owner, definition, readingUnitID)
		return err
	})
	if err != nil {
		return domain.ReadingUnitBookRegistration{}, err
	}
	return result, nil
}

// upsertReadingUnitDefinition writes the shared Work, Edition and Reading unit
// rows for a definition and returns the Reading unit's identifier. The rows are
// keyed by the definition key, so a repeat writes the same values.
func upsertReadingUnitDefinition(ctx context.Context, q *sqlcgen.Queries, definition domain.ReadingUnitDefinition) (string, error) {
	work, err := q.UpsertWork(ctx, sqlcgen.UpsertWorkParams{
		DefinitionKey: definition.Key,
		Title:         definition.WorkTitle,
		Author:        definition.WorkAuthor,
	})
	if err != nil {
		return "", err
	}
	edition, err := q.UpsertEdition(ctx, sqlcgen.UpsertEditionParams{
		WorkID:        work.ID,
		DefinitionKey: definition.Key,
		Label:         definition.EditionLabel,
	})
	if err != nil {
		return "", err
	}
	readingUnit, err := q.UpsertReadingUnit(ctx, sqlcgen.UpsertReadingUnitParams{
		EditionID:             edition.ID,
		DefinitionKey:         definition.Key,
		CitationStart:         definition.CitationStart,
		CitationEnd:           definition.CitationEnd,
		SourceFileName:        definition.SourceFileName,
		SourceFileSize:        definition.SourceFileSize,
		SourceFileSha256:      definition.SourceFileSHA256,
		InterpretationVersion: definition.InterpretationVersion,
	})
	if err != nil {
		return "", err
	}
	return readingUnit.ID, nil
}

// registerReadingUnitBookTx returns the owner's existing Book for the Reading
// unit, or creates the Book, its membership, its Inbox disposition and its
// immutable association.
func registerReadingUnitBookTx(ctx context.Context, q *sqlcgen.Queries, owner string, definition domain.ReadingUnitDefinition, readingUnitID string) (domain.ReadingUnitBookRegistration, error) {
	existing, err := q.GetRegisteredBookID(ctx, sqlcgen.GetRegisteredBookIDParams{OwnerID: owner, ReadingUnitID: readingUnitID})
	err = missing(err)
	switch {
	case err == nil:
		return domain.ReadingUnitBookRegistration{BookID: existing, Created: false}, nil
	case !errors.Is(err, ErrNotFound):
		return domain.ReadingUnitBookRegistration{}, err
	}
	book, err := q.InsertBook(ctx, sqlcgen.InsertBookParams{
		OwnerID:            owner,
		Title:              definition.WorkTitle,
		Author:             definition.WorkAuthor,
		MetadataProvenance: domain.MetadataProvenanceOperatorRegistration,
		LanguageState:      domain.LanguageChosen,
		LanguageTag:        textArg(definition.Language),
	})
	if err != nil {
		return domain.ReadingUnitBookRegistration{}, err
	}
	if err := q.InsertBookMembership(ctx, sqlcgen.InsertBookMembershipParams{OwnerID: owner, BookID: book.ID}); err != nil {
		return domain.ReadingUnitBookRegistration{}, err
	}
	if err := q.InsertInboxBookDisposition(ctx, sqlcgen.InsertInboxBookDispositionParams{OwnerID: owner, BookID: book.ID}); err != nil {
		return domain.ReadingUnitBookRegistration{}, err
	}
	if err := q.InsertBookReadingUnit(ctx, sqlcgen.InsertBookReadingUnitParams{OwnerID: owner, BookID: book.ID, ReadingUnitID: readingUnitID}); err != nil {
		return domain.ReadingUnitBookRegistration{}, err
	}
	return domain.ReadingUnitBookRegistration{BookID: book.ID, Created: true}, nil
}

// ListLegacyLanguageRows counts rows carrying an existing language identity
// that predates the classical registration. It returns one entry per owner and
// surface so the caller can refuse to mix old data with the new identities.
func (s *PostgresStore) ListLegacyLanguageRows(ctx context.Context, language string) ([]domain.LegacyLanguageRows, error) {
	rows, err := s.queries().ListLegacyLanguageRows(ctx, textArg(language))
	if err != nil {
		return nil, err
	}
	result := make([]domain.LegacyLanguageRows, 0, len(rows))
	for _, row := range rows {
		result = append(result, domain.LegacyLanguageRows{OwnerID: row.OwnerID, Surface: row.Surface, Count: row.RowCount})
	}
	return result, nil
}
