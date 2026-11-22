package persistence

import (
	"context"
	"errors"
	"sort"
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
// new Book is To Read, chosen for the definition's language, and linked by an
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
// unit, or creates the Book, its membership, its To Read disposition and its
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
	if err := upsertBookDisposition(ctx, q, owner, book.ID, domain.BookDispositionToRead); err != nil {
		return domain.ReadingUnitBookRegistration{}, err
	}
	if err := q.InsertBookReadingUnit(ctx, sqlcgen.InsertBookReadingUnitParams{OwnerID: owner, BookID: book.ID, ReadingUnitID: readingUnitID}); err != nil {
		return domain.ReadingUnitBookRegistration{}, err
	}
	return domain.ReadingUnitBookRegistration{BookID: book.ID, Created: true}, nil
}

// LegacyClassicalSourceMaterial is one source material the shared legacy-Book
// definition counts. It carries the reviewed Reading-unit facts and the source
// bytes startup conversion reads before rewriting the row to unit-only content.
type LegacyClassicalSourceMaterial struct {
	OwnerID, BookID, SourceMaterialID string
	ContentRevisionID, SnapshotID     string
	AnalysisRunID, CorpusID           string
	// AnalyzerVersion is the published run's recorded identity. Imported runs
	// prefix it with the interpretation version they actually ran under.
	AnalyzerVersion                     string
	ReadingUnitKey                      string
	CitationStart, CitationEnd          string
	SourceFileName                      string
	SourceFileSize                      int64
	SourceFileSHA256                    string
	SourceFormat                        string
	InterpretationVersion               string
	DefinitionRevision                  int
	Withdrawn                           bool
	WorkTitle, WorkAuthor, EditionLabel string
}

// ListLegacyClassicalSourceMaterials returns every source material the shared
// legacy classical Book definition counts. The catalog precondition counts the
// same rows through listLegacyLanguageRows, so conversion sees exactly the
// Books Add and preparation are blocked by.
func (s *PostgresStore) ListLegacyClassicalSourceMaterials(ctx context.Context, language string) ([]LegacyClassicalSourceMaterial, error) {
	rows, err := s.queries().ListLegacyClassicalSourceCandidates(ctx, sqlcgen.ListLegacyClassicalSourceCandidatesParams{
		Language:  language,
		MediaType: domain.ImportedSourceMediaType,
	})
	if err != nil {
		return nil, err
	}
	return legacyClassicalSourceMaterials(rows), nil
}

// legacyClassicalSourceMaterials maps the shared query rows onto the
// persistence-neutral shape conversion consumes.
func legacyClassicalSourceMaterials(rows []sqlcgen.ListLegacyClassicalSourceCandidatesRow) []LegacyClassicalSourceMaterial {
	materials := make([]LegacyClassicalSourceMaterial, 0, len(rows))
	for _, row := range rows {
		materials = append(materials, LegacyClassicalSourceMaterial{
			OwnerID: row.OwnerID, BookID: row.BookID,
			SourceMaterialID:  row.SourceMaterialID,
			ContentRevisionID: row.ContentRevisionID, SnapshotID: row.SnapshotID,
			AnalysisRunID: row.AnalysisRunID, CorpusID: row.CorpusID,
			AnalyzerVersion: row.AnalyzerVersion,
			ReadingUnitKey:  row.ReadingUnitKey,
			CitationStart:   row.CitationStart, CitationEnd: row.CitationEnd,
			SourceFileName: row.SourceFileName, SourceFileSize: row.SourceFileSize,
			SourceFileSHA256: row.SourceFileSha256, SourceFormat: row.SourceFormat,
			InterpretationVersion: row.InterpretationVersion, DefinitionRevision: row.DefinitionRevision,
			Withdrawn:    row.Withdrawn,
			WorkTitle:    row.WorkTitle,
			WorkAuthor:   row.WorkAuthor,
			EditionLabel: row.EditionLabel,
		})
	}
	return materials
}

// ListLegacyLanguageRows counts rows carrying an existing language identity
// that predates the classical registration. It returns one entry per owner and
// surface so the caller can refuse to mix old data with the new identities.
func (s *PostgresStore) ListLegacyLanguageRows(ctx context.Context, language string) ([]domain.LegacyLanguageRows, error) {
	return listLegacyLanguageRows(ctx, s.queries(), language)
}

// listLegacyLanguageRows is the transaction-aware form, so an Add can hold its
// legacy-row precondition and its write in one transaction. The classical
// source-material surface is counted from the shared candidate query so the
// check and startup conversion cannot drift apart.
func listLegacyLanguageRows(ctx context.Context, q *sqlcgen.Queries, language string) ([]domain.LegacyLanguageRows, error) {
	rows, err := q.ListLegacyLanguageRows(ctx, textArg(language))
	if err != nil {
		return nil, err
	}
	type key struct{ owner, surface string }
	counts := make(map[key]int64, len(rows))
	for _, row := range rows {
		counts[key{row.OwnerID, row.Surface}] += row.RowCount
	}
	candidates, err := q.ListLegacyClassicalSourceCandidates(ctx, sqlcgen.ListLegacyClassicalSourceCandidatesParams{
		Language:  language,
		MediaType: domain.ImportedSourceMediaType,
	})
	if err != nil {
		return nil, err
	}
	for _, candidate := range candidates {
		counts[key{candidate.OwnerID, "source_materials"}]++
	}
	result := make([]domain.LegacyLanguageRows, 0, len(counts))
	for group, count := range counts {
		result = append(result, domain.LegacyLanguageRows{OwnerID: group.owner, Surface: group.surface, Count: count})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].OwnerID != result[j].OwnerID {
			return result[i].OwnerID < result[j].OwnerID
		}
		return result[i].Surface < result[j].Surface
	})
	return result, nil
}
