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
	return mapConcordanceRows(rows, func(row sqlcgen.ListBookOccurrencesByLemmaRow) domain.ConcordanceOccurrence {
		return concordanceOccurrenceFromFields(row.Surface, row.CanonicalLemma, row.Upos, row.SentenceText, row.SentenceStartOffset, row.SentenceEndOffset, row.UnitStartOffset, row.UnitEndOffset, row.BookStartOffset, row.BookEndOffset, row.BookID, row.BookTitle, row.SourceMaterialID, row.AnalysisRunID, row.CorpusID, row.UnitID, row.ChapterTitle, row.UnitOrder, row.SentenceOrdinal, row.TokenOrdinal, int(row.BookPosition.Int32), row.BookPosition.Valid)
	}), nil
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
	return mapConcordanceRows(rows, func(row sqlcgen.ListBookOccurrencesBySurfaceRow) domain.ConcordanceOccurrence {
		return concordanceOccurrenceFromFields(row.Surface, row.CanonicalLemma, row.Upos, row.SentenceText, row.SentenceStartOffset, row.SentenceEndOffset, row.UnitStartOffset, row.UnitEndOffset, row.BookStartOffset, row.BookEndOffset, row.BookID, row.BookTitle, row.SourceMaterialID, row.AnalysisRunID, row.CorpusID, row.UnitID, row.ChapterTitle, row.UnitOrder, row.SentenceOrdinal, row.TokenOrdinal, int(row.BookPosition.Int32), row.BookPosition.Valid)
	}), nil
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
	return mapConcordanceRows(rows, func(row sqlcgen.ListStudyLanguageOccurrencesByLemmaRow) domain.ConcordanceOccurrence {
		return concordanceOccurrenceFromFields(row.Surface, row.CanonicalLemma, row.Upos, row.SentenceText, row.SentenceStartOffset, row.SentenceEndOffset, row.UnitStartOffset, row.UnitEndOffset, row.BookStartOffset, row.BookEndOffset, row.BookID, row.BookTitle, row.SourceMaterialID, row.AnalysisRunID, row.CorpusID, row.UnitID, row.ChapterTitle, row.UnitOrder, row.SentenceOrdinal, row.TokenOrdinal, int(row.BookPosition.Int32), row.BookPosition.Valid)
	}), nil
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
	return mapConcordanceRows(rows, func(row sqlcgen.ListStudyLanguageOccurrencesBySurfaceRow) domain.ConcordanceOccurrence {
		return concordanceOccurrenceFromFields(row.Surface, row.CanonicalLemma, row.Upos, row.SentenceText, row.SentenceStartOffset, row.SentenceEndOffset, row.UnitStartOffset, row.UnitEndOffset, row.BookStartOffset, row.BookEndOffset, row.BookID, row.BookTitle, row.SourceMaterialID, row.AnalysisRunID, row.CorpusID, row.UnitID, row.ChapterTitle, row.UnitOrder, row.SentenceOrdinal, row.TokenOrdinal, int(row.BookPosition.Int32), row.BookPosition.Valid)
	}), nil
}

func concordanceOccurrenceFromFields(
	surface, canonicalLemma, upos, sentenceText string,
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
		Surface: surface, CanonicalLemma: canonicalLemma, UPOS: upos, SentenceText: sentenceText,
		SentenceStartOffset: sentenceStartOffset, SentenceEndOffset: sentenceEndOffset,
		UnitStartOffset: unitStartOffset, UnitEndOffset: unitEndOffset,
		BookStartOffset: bookStartOffset, BookEndOffset: bookEndOffset,
		BookID: bookID, BookTitle: bookTitle, SourceMaterialID: sourceMaterialID,
		AnalysisRunID: analysisRunID, CorpusID: corpusID, UnitID: unitID,
		ChapterTitle: chapterTitle, UnitOrder: unitOrder, SentenceOrdinal: sentenceOrdinal,
		TokenOrdinal: tokenOrdinal, BookPosition: position,
	}
}

func mapConcordanceRows[T any](rows []T, mapRow func(T) domain.ConcordanceOccurrence) []domain.ConcordanceOccurrence {
	occurrences := make([]domain.ConcordanceOccurrence, 0, len(rows))
	for _, row := range rows {
		occurrences = append(occurrences, mapRow(row))
	}
	return occurrences
}
