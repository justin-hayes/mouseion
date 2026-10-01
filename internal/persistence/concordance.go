package persistence

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
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

// ListVocabularyConcordance returns one page of exact effective, observed
// surface, or analyzer-evidence matches from current analyses. Fetching one
// extra row determines whether a next page exists without inventing a total.
func (s *PostgresStore) ListVocabularyConcordance(ctx context.Context, owner, language string, lookup domain.ConcordanceLookup) (domain.ConcordanceResult, error) {
	language = canonicalization.NormalizeLanguage(language)
	if lookup.Page < 1 {
		lookup.Page = 1
	}
	const maxConcordancePage = int((1<<31-1)/25 + 1)
	if lookup.Page > maxConcordancePage {
		return domain.ConcordanceResult{Page: lookup.Page, HasPrevious: true}, nil
	}
	rows, err := s.queries().ListVocabularyConcordance(ctx, sqlcgen.ListVocabularyConcordanceParams{
		Owner: owner, Language: language, Mode: lookup.Mode, Term: lookup.Term,
		Upos: lookup.UPOS, BookIds: strings.Join(lookup.BookIDs, ","),
		GrammarDirection: lookup.GrammarDirection, Relation: lookup.Relation,
		Offset: (int64(lookup.Page) - 1) * 25,
	})
	if err != nil {
		return domain.ConcordanceResult{}, err
	}
	result := domain.ConcordanceResult{Page: lookup.Page, HasPrevious: lookup.Page > 1}
	if len(rows) > 25 {
		result.HasNext = true
		rows = rows[:25]
	}
	for _, row := range rows {
		occurrence := domain.ConcordanceResultOccurrence{
			ConcordanceOccurrence: concordanceOccurrenceFromFields(
				row.Surface, row.CanonicalLemma, row.Upos, row.Dependency,
				row.HeadOrdinal, pgText(row.HeadSurface), row.SentenceText,
				row.SentenceStartOffset, row.SentenceEndOffset, row.UnitStartOffset,
				row.UnitEndOffset, row.BookStartOffset, row.BookEndOffset,
				row.BookID, row.BookTitle, row.SourceMaterialID, row.AnalysisRunID,
				row.CorpusID, row.UnitID, row.ChapterTitle, row.UnitOrder,
				row.SentenceOrdinal, row.TokenOrdinal,
			),
			RawLemma: row.RawLemma, EffectiveLemma: row.EffectiveLemma,
			Corrected: row.Corrected, Excluded: row.Excluded,
		}
		result.Occurrences = append(result.Occurrences, occurrence)
	}
	return result, nil
}

// GetVocabularySentenceStudy returns the complete token-level analyzer parse
// for one sentence in the Book's still-current analysis.
func (s *PostgresStore) GetVocabularySentenceStudy(ctx context.Context, owner, book, run, corpus, unit string, sentence, target int64, targetSurface string) (domain.SentenceStudy, error) {
	rows, err := s.queries().GetVocabularySentenceStudy(ctx, sqlcgen.GetVocabularySentenceStudyParams{
		Owner: owner, Book: book, AnalysisRun: run, Corpus: corpus, UnitID: unit,
		SentenceOrdinal: sentence, TargetOrdinal: target,
	})
	if err != nil {
		return domain.SentenceStudy{}, err
	}
	if len(rows) == 0 {
		return domain.SentenceStudy{}, pgx.ErrNoRows
	}
	first := rows[0]
	study := domain.SentenceStudy{BookID: first.BookID, BookTitle: first.BookTitle,
		ChapterTitle: first.ChapterTitle, SentenceText: first.SentenceText,
		SentenceOrdinal: first.SentenceOrdinal, TargetOrdinal: first.TargetOrdinal,
		TargetSurface: targetSurface,
		Tokens:        make([]domain.SentenceStudyToken, 0, len(rows))}
	for _, row := range rows {
		if row.TokenOrdinal < 0 {
			continue // A sentence may be valid source text with no syntax tokens.
		}
		study.Tokens = append(study.Tokens, domain.SentenceStudyToken{Surface: row.Surface,
			RawLemma: row.RawLemma, EffectiveLemma: row.EffectiveLemma, UPOS: row.Upos,
			Dependency: row.Dependency, HeadSurface: row.HeadSurface,
			Ordinal: row.TokenOrdinal, HeadOrdinal: row.Head, Corrected: row.Corrected,
			Excluded: row.Excluded})
	}
	return study, nil
}

func concordanceOccurrenceFromFields(
	surface, canonicalLemma, upos, dependency string,
	headOrdinal int64, headSurface, sentenceText string,
	sentenceStartOffset, sentenceEndOffset, unitStartOffset, unitEndOffset,
	bookStartOffset, bookEndOffset int64,
	bookID, bookTitle, sourceMaterialID, analysisRunID, corpusID, unitID, chapterTitle string,
	unitOrder, sentenceOrdinal, tokenOrdinal int64,
) domain.ConcordanceOccurrence {
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
		TokenOrdinal: tokenOrdinal,
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
			commonRow.TokenOrdinal,
		))
	}
	return occurrences
}
