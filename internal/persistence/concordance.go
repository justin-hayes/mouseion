package persistence

import (
	"context"

	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// ListBookOccurrencesByLemma returns occurrences in a Book's current analysis
// matching one canonical lemma and UPOS identity.
func (s *PostgresStore) ListBookOccurrencesByLemma(ctx context.Context, owner, book, language, canonicalLemma, upos string) ([]domain.ConcordanceOccurrence, error) {
	language = canonicalization.NormalizeLanguage(language)
	rows, err := s.queries().ListBookOccurrencesByLemma(ctx, sqlcgen.ListBookOccurrencesByLemmaParams{
		Owner: owner, Book: book, Language: language, CanonicalLemma: canonicalLemma, Upos: upos,
	})
	if err != nil {
		return nil, err
	}
	return mapConcordanceRows(rows), nil
}

// ListBookOccurrencesByLemmaAndDependency returns Book occurrences matching a
// canonical lemma, UPOS identity, and dependency relation.
func (s *PostgresStore) ListBookOccurrencesByLemmaAndDependency(ctx context.Context, owner, book, language, canonicalLemma, upos, dependency string) ([]domain.ConcordanceOccurrence, error) {
	language = canonicalization.NormalizeLanguage(language)
	rows, err := s.queries().ListBookOccurrencesByLemmaAndDependency(ctx, sqlcgen.ListBookOccurrencesByLemmaAndDependencyParams{
		Owner: owner, Book: book, Language: language, CanonicalLemma: canonicalLemma, Upos: upos, Dependency: dependency,
	})
	if err != nil {
		return nil, err
	}
	return mapConcordanceRows(rows), nil
}

// ListBookOccurrencesBySurface returns occurrences in a Book's current
// analysis matching one exact surface form.
func (s *PostgresStore) ListBookOccurrencesBySurface(ctx context.Context, owner, book, language, surface string) ([]domain.ConcordanceOccurrence, error) {
	language = canonicalization.NormalizeLanguage(language)
	rows, err := s.queries().ListBookOccurrencesBySurface(ctx, sqlcgen.ListBookOccurrencesBySurfaceParams{
		Owner: owner, Book: book, Language: language, Surface: surface,
	})
	if err != nil {
		return nil, err
	}
	return mapConcordanceRows(rows), nil
}

// ListBookOccurrencesBySurfaceAndDependency returns Book occurrences matching
// an exact surface form and dependency relation.
func (s *PostgresStore) ListBookOccurrencesBySurfaceAndDependency(ctx context.Context, owner, book, language, surface, dependency string) ([]domain.ConcordanceOccurrence, error) {
	language = canonicalization.NormalizeLanguage(language)
	rows, err := s.queries().ListBookOccurrencesBySurfaceAndDependency(ctx, sqlcgen.ListBookOccurrencesBySurfaceAndDependencyParams{
		Owner: owner, Book: book, Language: language, Surface: surface, Dependency: dependency,
	})
	if err != nil {
		return nil, err
	}
	return mapConcordanceRows(rows), nil
}

// ListBookDependentsByGovernorLemma returns Book occurrences attached to a
// governor identified by canonical lemma and UPOS in one dependency relation.
func (s *PostgresStore) ListBookDependentsByGovernorLemma(ctx context.Context, owner, book, language, governorLemma, governorUPOS, dependency string) ([]domain.ConcordanceOccurrence, error) {
	language = canonicalization.NormalizeLanguage(language)
	rows, err := s.queries().ListBookDependentsByGovernorLemma(ctx, sqlcgen.ListBookDependentsByGovernorLemmaParams{
		Owner: owner, Book: book, Language: language, GovernorCanonicalLemma: governorLemma, GovernorUpos: governorUPOS, Dependency: dependency,
	})
	if err != nil {
		return nil, err
	}
	return mapConcordanceRows(rows), nil
}

// ListStudyLanguageOccurrencesByLemma returns occurrences across all active
// chosen Books in a study language, restricted to current analyses.
func (s *PostgresStore) ListStudyLanguageOccurrencesByLemma(ctx context.Context, owner, language, canonicalLemma, upos string) ([]domain.ConcordanceOccurrence, error) {
	language = canonicalization.NormalizeLanguage(language)
	rows, err := s.queries().ListStudyLanguageOccurrencesByLemma(ctx, sqlcgen.ListStudyLanguageOccurrencesByLemmaParams{
		Owner: owner, Language: language, CanonicalLemma: canonicalLemma, Upos: upos,
	})
	if err != nil {
		return nil, err
	}
	return mapConcordanceRows(rows), nil
}

// ListStudyLanguageOccurrencesByLemmaAndDependency returns occurrences across
// current analyses matching a canonical lemma, UPOS identity, and relation.
func (s *PostgresStore) ListStudyLanguageOccurrencesByLemmaAndDependency(ctx context.Context, owner, language, canonicalLemma, upos, dependency string) ([]domain.ConcordanceOccurrence, error) {
	language = canonicalization.NormalizeLanguage(language)
	rows, err := s.queries().ListStudyLanguageOccurrencesByLemmaAndDependency(ctx, sqlcgen.ListStudyLanguageOccurrencesByLemmaAndDependencyParams{
		Owner: owner, Language: language, CanonicalLemma: canonicalLemma, Upos: upos, Dependency: dependency,
	})
	if err != nil {
		return nil, err
	}
	return mapConcordanceRows(rows), nil
}

// ListStudyLanguageOccurrencesBySurface returns occurrences across all active
// chosen Books in a study language, restricted to current analyses.
func (s *PostgresStore) ListStudyLanguageOccurrencesBySurface(ctx context.Context, owner, language, surface string) ([]domain.ConcordanceOccurrence, error) {
	language = canonicalization.NormalizeLanguage(language)
	rows, err := s.queries().ListStudyLanguageOccurrencesBySurface(ctx, sqlcgen.ListStudyLanguageOccurrencesBySurfaceParams{
		Owner: owner, Language: language, Surface: surface,
	})
	if err != nil {
		return nil, err
	}
	return mapConcordanceRows(rows), nil
}

// ListStudyLanguageOccurrencesBySurfaceAndDependency returns occurrences across
// current analyses matching an exact surface form and relation.
func (s *PostgresStore) ListStudyLanguageOccurrencesBySurfaceAndDependency(ctx context.Context, owner, language, surface, dependency string) ([]domain.ConcordanceOccurrence, error) {
	language = canonicalization.NormalizeLanguage(language)
	rows, err := s.queries().ListStudyLanguageOccurrencesBySurfaceAndDependency(ctx, sqlcgen.ListStudyLanguageOccurrencesBySurfaceAndDependencyParams{
		Owner: owner, Language: language, Surface: surface, Dependency: dependency,
	})
	if err != nil {
		return nil, err
	}
	return mapConcordanceRows(rows), nil
}

// ListStudyLanguageDependentsByGovernorLemma returns occurrences across current
// analyses attached to a governor identity in one dependency relation.
func (s *PostgresStore) ListStudyLanguageDependentsByGovernorLemma(ctx context.Context, owner, language, governorLemma, governorUPOS, dependency string) ([]domain.ConcordanceOccurrence, error) {
	language = canonicalization.NormalizeLanguage(language)
	rows, err := s.queries().ListStudyLanguageDependentsByGovernorLemma(ctx, sqlcgen.ListStudyLanguageDependentsByGovernorLemmaParams{
		Owner: owner, Language: language, GovernorCanonicalLemma: governorLemma, GovernorUpos: governorUPOS, Dependency: dependency,
	})
	if err != nil {
		return nil, err
	}
	return mapConcordanceRows(rows), nil
}

func concordanceOccurrenceFromFields(
	surface, canonicalLemma, upos, dependency string,
	headOrdinal int64, headSurface, sentenceText string,
	sentenceStartOffset, sentenceEndOffset, unitStartOffset, unitEndOffset,
	bookStartOffset, bookEndOffset int64,
	bookID, bookTitle, sourceMaterialID, analysisRunID, corpusID, unitID, chapterTitle string,
	unitOrder, sentenceOrdinal, tokenOrdinal int64,
	bookPosition int,
	bookPositionValid bool,
) domain.ConcordanceOccurrence {
	var position *int
	if bookPositionValid {
		position = &bookPosition
	}
	return domain.ConcordanceOccurrence{
		Surface: surface, CanonicalLemma: canonicalLemma, UPOS: upos,
		Dependency: dependency, HeadOrdinal: headOrdinal, HeadSurface: headSurface,
		SentenceText:        sentenceText,
		SentenceStartOffset: sentenceStartOffset, SentenceEndOffset: sentenceEndOffset,
		UnitStartOffset: unitStartOffset, UnitEndOffset: unitEndOffset,
		BookStartOffset: bookStartOffset, BookEndOffset: bookEndOffset,
		BookID: bookID, BookTitle: bookTitle, SourceMaterialID: sourceMaterialID,
		AnalysisRunID: analysisRunID, CorpusID: corpusID, UnitID: unitID,
		ChapterTitle: chapterTitle, UnitOrder: unitOrder, SentenceOrdinal: sentenceOrdinal,
		TokenOrdinal: tokenOrdinal, BookPosition: position,
	}
}

type concordanceSQLRow interface {
	sqlcgen.ListBookOccurrencesByLemmaRow |
		sqlcgen.ListBookOccurrencesByLemmaAndDependencyRow |
		sqlcgen.ListBookOccurrencesBySurfaceRow |
		sqlcgen.ListBookOccurrencesBySurfaceAndDependencyRow |
		sqlcgen.ListBookDependentsByGovernorLemmaRow |
		sqlcgen.ListStudyLanguageOccurrencesByLemmaRow |
		sqlcgen.ListStudyLanguageOccurrencesByLemmaAndDependencyRow |
		sqlcgen.ListStudyLanguageOccurrencesBySurfaceRow |
		sqlcgen.ListStudyLanguageOccurrencesBySurfaceAndDependencyRow |
		sqlcgen.ListStudyLanguageDependentsByGovernorLemmaRow
}

func mapConcordanceRows[T concordanceSQLRow](rows []T) []domain.ConcordanceOccurrence {
	occurrences := make([]domain.ConcordanceOccurrence, 0, len(rows))
	for _, row := range rows {
		commonRow := sqlcgen.ListBookOccurrencesByLemmaRow(row)
		occurrences = append(occurrences, concordanceOccurrenceFromFields(
			commonRow.Surface, commonRow.CanonicalLemma, commonRow.Upos, commonRow.Dependency,
			commonRow.HeadOrdinal, pgText(commonRow.HeadSurface), commonRow.SentenceText,
			commonRow.SentenceStartOffset, commonRow.SentenceEndOffset,
			commonRow.UnitStartOffset, commonRow.UnitEndOffset,
			commonRow.BookStartOffset, commonRow.BookEndOffset,
			commonRow.BookID, commonRow.BookTitle, commonRow.SourceMaterialID,
			commonRow.AnalysisRunID, commonRow.CorpusID, commonRow.UnitID,
			commonRow.ChapterTitle, commonRow.UnitOrder, commonRow.SentenceOrdinal,
			commonRow.TokenOrdinal, int(commonRow.BookPosition.Int32), commonRow.BookPosition.Valid,
		))
	}
	return occurrences
}
