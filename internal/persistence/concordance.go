package persistence

import (
	"context"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/lexical"
	"golang.org/x/text/unicode/norm"
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

// ListVocabularyConcordance returns one page of occurrences of an evidenced
// effective lemma or, failing that, of the literal typed word form across the
// owner's current analyses in the language. A fresh lookup (empty Kind) lets an
// evidenced lemma take precedence over a simultaneous form match; applied
// lookups carry their kind and are never re-recognized. Fetching one extra row
// determines whether a next page exists without inventing a total.
func (s *PostgresStore) ListVocabularyConcordance(ctx context.Context, owner, language string, lookup domain.ConcordanceLookup) (pageResult domain.ConcordanceResult, errResult error) {
	language = canonicalization.NormalizeLanguage(language)
	if lookup.Page < 1 {
		lookup.Page = 1
	}
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if err != nil {
		return domain.ConcordanceResult{}, err
	}
	defer func() {
		if err := tx.Rollback(ctx); !errors.Is(err, pgx.ErrTxClosed) {
			errResult = errors.Join(errResult, err)
		}
	}()
	var revision string
	err = tx.QueryRow(ctx, `
		SELECT md5(
		  COALESCE((SELECT jsonb_agg(jsonb_build_array(ca.book_id, ca.analysis_run_id, ca.corpus_id, b.title) ORDER BY ca.book_id)::text
		    FROM current_analysis_identity ca
		    JOIN books b ON b.owner_id=ca.owner_id AND b.id=ca.book_id
		    WHERE ca.owner_id=$1 AND b.language_state='chosen' AND b.language_tag=$2), '[]')
		  || '|' || COALESCE((SELECT jsonb_agg(jsonb_build_array(d.book_id, d.corpus_id, d.analysis_run_id,
		        d.source_document_id, d.start_offset, d.end_offset, d.canonical_lemma, d.excluded)
		        ORDER BY d.book_id, d.corpus_id, d.analysis_run_id, d.source_document_id, d.start_offset, d.end_offset)::text
		    FROM occurrence_lemma_corrections d
		    JOIN current_analysis_identity ca ON ca.owner_id=d.owner_id AND ca.book_id=d.book_id
		      AND ca.analysis_run_id=d.analysis_run_id AND ca.corpus_id=d.corpus_id
		    JOIN books b ON b.owner_id=ca.owner_id AND b.id=ca.book_id
		    WHERE d.owner_id=$1 AND b.language_state='chosen' AND b.language_tag=$2), '[]')
		)`, owner, language).Scan(&revision)
	if err != nil {
		return domain.ConcordanceResult{}, err
	}
	if lookup.Revision != "" && lookup.Revision != revision {
		return domain.ConcordanceResult{Page: lookup.Page, HasPrevious: lookup.Page > 1, Revision: revision, Stale: true}, nil
	}
	const maxConcordancePage = int((1<<31-1)/25 + 1)
	if lookup.Page > maxConcordancePage {
		if err := tx.Commit(ctx); err != nil {
			return domain.ConcordanceResult{}, err
		}
		return domain.ConcordanceResult{Page: lookup.Page, HasPrevious: true, Revision: revision, Kind: lookup.Kind}, nil
	}
	queries := sqlcgen.New(tx)
	lemmaKey, formKey := concordanceTermKeys(language, lookup.Term)
	kind := lookup.Kind
	switch {
	case kind == "" && lookup.UPOS != "":
		kind = domain.ConcordanceKindLemma
	case kind == "":
		kind = domain.ConcordanceKindForm
		if lexical.IsLemma(lemmaKey) {
			evidenced, err := queries.ConcordanceHasEvidencedLemma(ctx, sqlcgen.ConcordanceHasEvidencedLemmaParams{
				Owner: owner, Language: language, LemmaKey: lemmaKey,
			})
			if err != nil {
				return domain.ConcordanceResult{}, err
			}
			if evidenced {
				kind = domain.ConcordanceKindLemma
			}
		}
	}
	rows, err := queries.ListVocabularyConcordance(ctx, sqlcgen.ListVocabularyConcordanceParams{
		Owner: owner, Language: language, Kind: kind, LemmaKey: lemmaKey, FormKey: formKey,
		Upos:   lookup.UPOS,
		Offset: (int64(lookup.Page) - 1) * 25,
	})
	if err != nil {
		return domain.ConcordanceResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.ConcordanceResult{}, err
	}
	result := domain.ConcordanceResult{Page: lookup.Page, HasPrevious: lookup.Page > 1, Revision: revision, Kind: kind}
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

// concordanceTermKeys derives the lookup keys for one typed term without any
// query-time analysis: the canonical lemma identity used by stored effective
// lemmas, and a case-insensitive, accent-preserving surface key. Final sigma is
// folded because lowercasing alone leaves word-final ς distinct from σ.
func concordanceTermKeys(language, term string) (lemmaKey, formKey string) {
	term = norm.NFC.String(term)
	// A term the profile cannot normalize has no lemma identity; an empty key
	// never matches, so it falls through to literal form matching.
	if normalized, err := canonicalization.Normalize(language, term); err == nil {
		lemmaKey = normalized.CanonicalLemma
	}
	formKey = strings.ReplaceAll(strings.ToLower(term), "ς", "σ")
	return lemmaKey, formKey
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
