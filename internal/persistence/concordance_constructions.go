package persistence

import (
	"context"
	"errors"
	"strconv"

	"github.com/jackc/pgx/v5"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/constructions"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// concordanceConstructionPageSize is the rows one parallels page shows.
const concordanceConstructionPageSize = 25

// listConstructionConcordance serves the construction match of Concordance (ADR
// 0125): other occurrences of one construction, in three tiers measured from the
// origin occurrence's Book and author. It runs in the caller's read-only
// repeatable-read transaction, which it commits once the page and its words are
// read.
func (s *PostgresStore) listConstructionConcordance(ctx context.Context, tx pgx.Tx, owner, language string, lookup domain.ConcordanceLookup) (domain.ConcordanceResult, error) {
	result := domain.ConcordanceResult{Page: lookup.Page, HasPrevious: lookup.Page > 1, Match: domain.ConcordanceMatchConstruction, Term: lookup.Term}
	queries := sqlcgen.New(tx)
	origin, found, err := loadConstructionOrigin(ctx, queries, owner, language, lookup)
	if err != nil {
		return domain.ConcordanceResult{}, err
	}
	if !found {
		result.InvalidOrigin = true
		return result, nil
	}
	revision, err := constructionConcordanceRevision(ctx, tx, owner, language)
	if err != nil {
		return domain.ConcordanceResult{}, err
	}
	result.Revision = revision
	if lookup.Revision != "" && lookup.Revision != revision {
		result.Stale = true
		return result, nil
	}
	result.PriorityBookID, result.PriorityTitle = origin.row.BookID, origin.row.BookTitle
	result.Origin = domain.ConcordanceConstructionOriginSentence{
		BookID: origin.row.BookID, BookTitle: origin.row.BookTitle, CorpusID: origin.row.CorpusID, UnitID: origin.row.UnitID,
		AnchorSurface: origin.row.AnchorSurface, WorkTitle: origin.row.WorkTitle, Author: origin.row.Author,
	}
	const maxPage = int((1<<31-1)/concordanceConstructionPageSize + 1)
	if lookup.Page > maxPage {
		return result, tx.Commit(ctx)
	}
	rows, err := loadConstructionRows(ctx, queries, owner, language, lookup, origin)
	if err != nil {
		return domain.ConcordanceResult{}, err
	}
	rows, result.HasNext, result.Continued = trimConstructionRows(rows, lookup.Page)
	for _, row := range rows {
		occurrence, err := constructionOccurrence(ctx, queries, owner, row)
		if err != nil {
			return domain.ConcordanceResult{}, err
		}
		result.Occurrences = append(result.Occurrences, occurrence)
	}
	return result, tx.Commit(ctx)
}

// constructionOrigin is a validated origin with the identities the query needs.
type constructionOrigin struct {
	lookup domain.ConcordanceLookup
	row    sqlcgen.GetConstructionOriginRow
}

// loadConstructionOrigin validates the origin occurrence: the lookup's priority
// must be the Book of the origin run, the run must be the learner's current
// analysis of a chosen-language Book with a Reading unit, and the sentence and
// anchor word must exist. Anything else is not found, never an error.
func loadConstructionOrigin(ctx context.Context, queries *sqlcgen.Queries, owner, language string, lookup domain.ConcordanceLookup) (constructionOrigin, bool, error) {
	if language != domain.ConcordanceConstructionLanguage || lookup.Origin == nil ||
		!concordanceBookIDPattern.MatchString(lookup.Origin.RunID) || !concordanceBookIDPattern.MatchString(lookup.Priority) {
		return constructionOrigin{}, false, nil
	}
	row, err := queries.GetConstructionOrigin(ctx, sqlcgen.GetConstructionOriginParams{
		Language: language, SentenceOrdinal: int64(lookup.Origin.Sentence), AnchorOrdinal: int64(lookup.Origin.Anchor),
		Owner: owner, AnalysisRun: lookup.Origin.RunID, Book: lookup.Priority,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return constructionOrigin{}, false, nil
	}
	if err != nil {
		return constructionOrigin{}, false, err
	}
	return constructionOrigin{lookup: lookup, row: row}, true, nil
}

// loadConstructionRows fetches the page. Later pages start one row early so the
// preceding row can say whether the first Book group continues.
func loadConstructionRows(ctx context.Context, queries *sqlcgen.Queries, owner, language string, lookup domain.ConcordanceLookup, origin constructionOrigin) ([]sqlcgen.ListConstructionConcordanceRow, error) {
	offset := (int64(lookup.Page) - 1) * concordanceConstructionPageSize
	if lookup.Page > 1 {
		offset--
	}
	return queries.ListConstructionConcordance(ctx, sqlcgen.ListConstructionConcordanceParams{
		Owner: owner, Language: language, Construction: lookup.Term, Subtype: lookup.Subtype,
		OriginRun: lookup.Origin.RunID, OriginSentence: int64(lookup.Origin.Sentence), OriginAnchor: int64(lookup.Origin.Anchor),
		DetectorVersion: constructions.AGDTDetectorVersion,
		OriginBook:      origin.row.BookID, OriginAuthor: origin.row.Author, OriginWork: origin.row.WorkID,
		Offset: offset,
	})
}

// trimConstructionRows turns the one-extra-row fetch into a page, as the word
// lookups do: the leading row of a later page only decides "continued", and a
// row past the page proves a next page.
func trimConstructionRows(rows []sqlcgen.ListConstructionConcordanceRow, page int) (trimmed []sqlcgen.ListConstructionConcordanceRow, hasNext, continued bool) {
	var previousBookID string
	if page > 1 && len(rows) > 0 {
		previousBookID, rows = rows[0].BookID, rows[1:]
	}
	if len(rows) > concordanceConstructionPageSize {
		hasNext = true
		rows = rows[:concordanceConstructionPageSize]
	}
	return rows, hasNext, len(rows) > 0 && rows[0].BookID == previousBookID
}

// constructionOccurrence projects one row, with the words of its sentence so the
// expanded row can reprint the Sentence study extract.
func constructionOccurrence(ctx context.Context, queries *sqlcgen.Queries, owner string, row sqlcgen.ListConstructionConcordanceRow) (domain.ConcordanceResultOccurrence, error) {
	words, err := queries.ListConstructionSentenceWords(ctx, sqlcgen.ListConstructionSentenceWordsParams{
		Owner: owner, AnalysisRun: row.AnalysisRunID, Corpus: row.CorpusID, SentenceOrdinal: row.SentenceOrdinal,
	})
	if err != nil {
		return domain.ConcordanceResultOccurrence{}, err
	}
	extent := make([]int, len(row.ExtentTokenOrdinals))
	for i, ordinal := range row.ExtentTokenOrdinals {
		extent[i] = int(ordinal)
	}
	anchor := int(row.TokenOrdinal)
	study := domain.SentenceStudy{
		BookID: row.BookID, BookTitle: row.BookTitle, ChapterTitle: row.ChapterTitle, SentenceText: row.SentenceText,
		UnitText: row.UnitText, SentenceStartOffset: row.SentenceUnitStartOffset, SentenceEndOffset: row.SentenceUnitEndOffset,
		SentenceOrdinal: row.SentenceOrdinal, TargetOrdinal: -1, SyntaxState: syntaxStateNativeAGDT,
		Constructions: []domain.SentenceStudyConstruction{{
			ConstructionID: row.ConstructionID, SubtypeID: row.SubtypeID, Anchor: anchor, Extent: extent,
		}},
	}
	for _, word := range words {
		study.Tokens = append(study.Tokens, domain.SentenceStudyToken{
			Surface: word.Surface, Ordinal: word.TokenOrdinal, StartOffset: word.StartOffset, EndOffset: word.EndOffset,
		})
	}
	return domain.ConcordanceResultOccurrence{
		ConcordanceOccurrence: concordanceOccurrenceFromFields(
			row.Surface, row.CanonicalLemma, row.Upos, row.Headword, row.Dependency,
			row.HeadOrdinal, "", row.SentenceText,
			row.SentenceStartOffset, row.SentenceEndOffset, row.UnitStartOffset,
			row.UnitEndOffset, row.BookStartOffset, row.BookEndOffset,
			row.BookID, row.BookTitle, row.SourceMaterialID, row.AnalysisRunID,
			row.CorpusID, row.UnitID, row.ChapterTitle, row.UnitOrder,
			row.SentenceOrdinal, row.TokenOrdinal,
		),
		EffectiveLemma: row.CanonicalLemma,
		Tier:           row.Tier, Anchor: anchor, ConstructionExtent: extent,
		ConstructionID: row.ConstructionID, SubtypeID: row.SubtypeID, Study: study,
	}, nil
}

// constructionConcordanceRevision fingerprints what a parallels page is built
// from: the current analyses of the owner's study-language Books, each one's
// readiness marker (its detector version and when it was built), and the
// detector version the page reads. A rebuild or a version bump between two page
// requests changes it. Lemma corrections and exclusions are not part of it, as
// detection ignores them.
func constructionConcordanceRevision(ctx context.Context, tx pgx.Tx, owner, language string) (string, error) {
	var revision string
	err := tx.QueryRow(ctx, `
		SELECT md5(
		  COALESCE((SELECT jsonb_agg(jsonb_build_array(ca.book_id, ca.analysis_run_id, ca.corpus_id, b.title,
		        m.detector_version, m.built_at) ORDER BY ca.book_id)::text
		    FROM current_analysis_identity ca
		    JOIN books b ON b.owner_id=ca.owner_id AND b.id=ca.book_id
		    LEFT JOIN construction_run_markers m ON m.owner_id=ca.owner_id AND m.analysis_run_id=ca.analysis_run_id
		    WHERE ca.owner_id=$1 AND b.language_state='chosen' AND b.language_tag=$2), '[]')
		  || '|construction|' || $3::text)`, owner, language, strconv.Itoa(constructions.AGDTDetectorVersion)).Scan(&revision)
	return revision, err
}
