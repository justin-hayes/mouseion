package persistence

import (
	"context"
	"errors"
	"strings"

	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/checked"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/selection"
)

// populateMyBooksCoverage enriches a page in one token-evidence query. The
// selection projection is shared with Reading, so occurrence corrections,
// exclusions, POS filtering, and Unicode lexical rules stay consistent.
func (s *PostgresStore) populateMyBooksCoverage(ctx context.Context, owner string, books []domain.MyBook) error {
	if len(books) == 0 {
		return nil
	}
	ids := make([]string, 0, len(books))
	byID := make(map[string]*coverageProjection, len(books))
	for i := range books {
		books[i].CoverageKnownTokens = 0
		books[i].CoverageTotalTokens = 0
		ids = append(ids, books[i].Book.ID)
		byID[books[i].Book.ID] = &coverageProjection{known: make(map[selection.Identity]bool)}
	}
	rows, err := s.queries().ListMyBooksCoverageTokens(ctx, sqlcgen.ListMyBooksCoverageTokensParams{Owner: owner, BookIds: ids})
	if err != nil {
		return err
	}
	for _, row := range rows {
		projection := byID[row.BookID]
		if projection == nil {
			return errors.New("coverage query returned a book outside the requested page")
		}
		if projection.analysis.Language == "" {
			projection.corpusID = row.CorpusID
			projection.analyzableTokens = row.SourceAnalyzableTokenCount
			projection.analysis.Language = row.Language
			projection.analysis.Sentences = []analyzer.Sentence{{}}
		} else if projection.analyzableTokens != row.SourceAnalyzableTokenCount {
			return errors.New("coverage query returned inconsistent source token counts")
		}
		start, err := checked.Uint64FromInt64(row.StartOffset)
		if err != nil {
			return errors.New("coverage query returned an invalid token start offset")
		}
		end, err := checked.Uint64FromInt64(row.EndOffset)
		if err != nil {
			return errors.New("coverage query returned an invalid token end offset")
		}
		projection.analysis.Sentences[0].Tokens = append(projection.analysis.Sentences[0].Tokens, analyzer.Token{
			Surface: row.Surface, CanonicalLemma: row.CanonicalLemma, UPOS: row.Upos, Dependency: row.Dependency,
			Location: analyzer.SourceLocation{SourceDocumentID: row.SourceDocumentID, StartOffset: start, EndOffset: end},
		})
		if row.HasCorrection {
			decision := selection.OccurrenceDecision{Occurrence: selection.OccurrenceIdentity{SourceDocumentID: row.SourceDocumentID, StartOffset: start, EndOffset: end}}
			if row.CorrectionExcluded {
				decision.Excluded = true
			} else if row.CorrectionLemma.Valid {
				decision.Lemma = row.CorrectionLemma.String
			} else {
				return errors.New("coverage query returned an incomplete occurrence correction")
			}
			projection.decisions = append(projection.decisions, decision)
		}
		if row.IsKnown {
			lemma := row.CanonicalLemma
			if row.CorrectionLemma.Valid {
				lemma = row.CorrectionLemma.String
			}
			projection.known[selection.Identity{Language: row.Language, CanonicalLemma: strings.TrimSpace(lemma), UPOS: strings.ToUpper(strings.TrimSpace(row.Upos))}] = true
		}
	}
	for i := range books {
		book := &books[i]
		projection := byID[book.Book.ID]
		if projection.analysis.Language == "" {
			continue
		}
		config := selection.DefaultConfig(projection.corpusID)
		candidates, err := selection.Project(projection.analysis, config, projection.decisions)
		if err != nil {
			return err
		}
		book.CoverageTotalTokens = projection.analyzableTokens
		for _, candidate := range candidates {
			if projection.known[candidate.Identity] {
				book.CoverageKnownTokens += int64(candidate.OccurrenceCount)
			}
		}
		book.CoverageKnownTokens = min(max(book.CoverageKnownTokens, 0), book.CoverageTotalTokens)
	}
	return nil
}

type coverageProjection struct {
	analysis         analyzer.Result
	corpusID         string
	analyzableTokens int64
	decisions        []selection.OccurrenceDecision
	known            map[selection.Identity]bool
}
