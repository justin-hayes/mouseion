package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"regexp"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
	"golang.org/x/text/unicode/norm"
)

// concordanceTerms normalizes one submitted term for lemma and form matching.
// The lemma key follows the language's canonical-lemma profile so it equals
// stored effective lemmas; the form key only folds case (and Greek final
// sigma) so accents and other spelling distinctions stay meaningful.
func concordanceTerms(language, term string) (lemma, form string) {
	term = norm.NFC.String(strings.TrimSpace(term))
	profile, err := canonicalization.For(language)
	if err != nil {
		profile = canonicalization.LanguageNeutral()
	}
	lemma = profile.Canonical(term)
	form = strings.ReplaceAll(strings.ToLower(term), "ς", "σ")
	return lemma, form
}

// ListVocabularyConcordance returns one page of effective-lemma or literal
// word-form matches from current analyses. An unspecified interpretation is
// recognized first: an evidenced effective lemma wins and expands its observed
// forms; otherwise only the typed source form matches. Fetching one extra row
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
	revision, err := concordanceRevision(ctx, tx, owner, language)
	if err != nil {
		return domain.ConcordanceResult{}, err
	}
	if lookup.Revision != "" && lookup.Revision != revision {
		return domain.ConcordanceResult{Page: lookup.Page, HasPrevious: lookup.Page > 1, Revision: revision, Stale: true}, nil
	}
	queries := sqlcgen.New(tx)
	interpretation, err := recognizeConcordanceMatch(ctx, queries, owner, language, lookup)
	if err != nil {
		return domain.ConcordanceResult{}, err
	}
	result := domain.ConcordanceResult{Page: lookup.Page, HasPrevious: lookup.Page > 1, Revision: revision, Match: interpretation.match, Term: interpretation.term}
	priority, err := loadConcordancePriority(ctx, queries, owner, language, lookup.Priority)
	if err != nil {
		return domain.ConcordanceResult{}, err
	}
	result.PriorityBookID, result.PriorityTitle = priority.bookID, priority.title
	if priority.invalid {
		result.InvalidPriority = true
		return result, nil
	}
	const maxConcordancePage = int((1<<31-1)/25 + 1)
	if lookup.Page > maxConcordancePage {
		if err := tx.Commit(ctx); err != nil {
			return domain.ConcordanceResult{}, err
		}
		result.HasPrevious = true
		result.PriorityState = concordancePriorityState(result.PriorityBookID, priority.analyzed, false)
		return result, nil
	}
	rows, err := loadConcordanceRows(ctx, queries, owner, language, lookup.Page, interpretation, result.PriorityBookID)
	if err != nil {
		return domain.ConcordanceResult{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return domain.ConcordanceResult{}, err
	}
	page := trimConcordanceRows(rows, lookup.Page)
	result.HasNext, result.Continued = page.hasNext, page.continued
	result.PriorityState = concordancePriorityState(result.PriorityBookID, priority.analyzed, page.priorityMatched)
	result.Occurrences = projectConcordanceOccurrences(page.rows)
	return result, nil
}

// concordanceRevision fingerprints the current analysis identity and occurrence
// corrections for the owner and study language, so a later page request can
// detect that the evidence it was built from changed.
func concordanceRevision(ctx context.Context, tx pgx.Tx, owner, language string) (string, error) {
	var revision string
	err := tx.QueryRow(ctx, `
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
	return revision, err
}

// concordanceInterpretation is a resolved lookup interpretation: the match mode,
// the UPOS filter (empty unless the match is the lemma), and the term the query
// matches on.
type concordanceInterpretation struct {
	match string
	upos  string
	term  string
}

// recognizeConcordanceMatch resolves the requested interpretation. An explicit
// match is honored; an unspecified one is recognized from evidence, preferring
// an evidenced effective lemma over the literal source form.
func recognizeConcordanceMatch(ctx context.Context, queries *sqlcgen.Queries, owner, language string, lookup domain.ConcordanceLookup) (concordanceInterpretation, error) {
	lemmaTerm, formTerm := concordanceTerms(language, lookup.Term)
	interpretation := concordanceInterpretation{match: lookup.Match, upos: lookup.UPOS}
	if interpretation.match == "" {
		evidenced, err := queries.ConcordanceLemmaEvidenced(ctx, sqlcgen.ConcordanceLemmaEvidencedParams{
			Owner: owner, Language: language, Term: lemmaTerm,
		})
		if err != nil {
			return concordanceInterpretation{}, err
		}
		interpretation.match = domain.ConcordanceMatchForm
		if evidenced {
			interpretation.match = domain.ConcordanceMatchLemma
		}
	}
	if interpretation.match != domain.ConcordanceMatchLemma {
		interpretation.upos = ""
	}
	interpretation.term = formTerm
	if interpretation.match == domain.ConcordanceMatchLemma {
		interpretation.term = lemmaTerm
	}
	return interpretation, nil
}

// concordancePriority is a captured priority Book after validation. An invalid
// identity (malformed, unowned, or in another language) is reported rather than
// treated as the explicit absence of a priority.
type concordancePriority struct {
	bookID   string
	title    string
	analyzed bool
	invalid  bool
}

// loadConcordancePriority validates a captured priority Book identity and loads
// its title and analysis state. A malformed identity or one that is not the
// owner's Book in the study language is invalid; it never opens another owner's
// Book.
func loadConcordancePriority(ctx context.Context, queries *sqlcgen.Queries, owner, language, priority string) (concordancePriority, error) {
	if priority == "" || priority == domain.ConcordancePriorityNone {
		return concordancePriority{}, nil
	}
	if !concordanceBookIDPattern.MatchString(priority) {
		return concordancePriority{invalid: true}, nil
	}
	book, err := queries.GetConcordancePriorityBook(ctx, sqlcgen.GetConcordancePriorityBookParams{
		Owner: owner, Book: priority, Language: pgtype.Text{String: language, Valid: true},
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return concordancePriority{invalid: true}, nil
	}
	if err != nil {
		return concordancePriority{}, err
	}
	return concordancePriority{bookID: priority, title: book.Title, analyzed: book.Analyzed}, nil
}

// loadConcordanceRows fetches the page's rows. Later pages start one row early
// so the preceding row can say whether the first Book group is a continuation.
func loadConcordanceRows(ctx context.Context, queries *sqlcgen.Queries, owner, language string, page int, interpretation concordanceInterpretation, priorityBook string) ([]sqlcgen.ListVocabularyConcordanceRow, error) {
	offset := (int64(page) - 1) * 25
	if page > 1 {
		offset--
	}
	return queries.ListVocabularyConcordance(ctx, sqlcgen.ListVocabularyConcordanceParams{
		Owner: owner, Language: language, Match: interpretation.match, Term: interpretation.term, Upos: interpretation.upos,
		PriorityBook: priorityBook, Offset: offset,
	})
}

// concordanceRowPage is one requested page after trimming the extra row that
// proves a next page and the leading row that carries the previous page's Book
// identity.
type concordanceRowPage struct {
	rows            []sqlcgen.ListVocabularyConcordanceRow
	priorityMatched bool
	hasNext         bool
	continued       bool
}

// trimConcordanceRows turns the one-extra-row fetch into a page: on later pages
// the leading row reconstructs whether the first Book group continues, and a
// row beyond the page size proves a next page without inventing a total.
func trimConcordanceRows(rows []sqlcgen.ListVocabularyConcordanceRow, page int) concordanceRowPage {
	pageRows := concordanceRowPage{priorityMatched: len(rows) > 0 && rows[0].PriorityMatched}
	var previousBookID string
	if page > 1 && len(rows) > 0 {
		previousBookID, rows = rows[0].BookID, rows[1:]
	}
	if len(rows) > 25 {
		pageRows.hasNext = true
		rows = rows[:25]
	}
	pageRows.rows = rows
	pageRows.continued = len(rows) > 0 && rows[0].BookID == previousBookID
	return pageRows
}

// projectConcordanceOccurrences maps result rows to the occurrence fields the
// page renders.
func projectConcordanceOccurrences(rows []sqlcgen.ListVocabularyConcordanceRow) []domain.ConcordanceResultOccurrence {
	var occurrences []domain.ConcordanceResultOccurrence
	for _, row := range rows {
		occurrences = append(occurrences, domain.ConcordanceResultOccurrence{
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
		})
	}
	return occurrences
}

// concordanceBookIDPattern is the canonical text form of a Book identity; any
// other captured priority is malformed before it reaches a uuid column.
var concordanceBookIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// concordancePriorityState separates a priority Book with no matches from one
// whose current analysis is unavailable, so a missing analysis is never
// reported as zero occurrences.
func concordancePriorityState(priorityBookID string, analyzed, matched bool) string {
	switch {
	case priorityBookID == "":
		return ""
	case !analyzed:
		return domain.ConcordancePriorityUnavailable
	case matched:
		return domain.ConcordancePriorityMatches
	default:
		return domain.ConcordancePriorityNoMatches
	}
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
		morphology := make(map[string]string)
		if len(row.Morphology) > 0 {
			if err := json.Unmarshal(row.Morphology, &morphology); err != nil {
				// A stored value the analyzer's string map cannot represent
				// contributes no features rather than failing the whole
				// sentence-study read. Missing features are shown honestly.
				morphology = map[string]string{}
			}
		}
		study.Tokens = append(study.Tokens, domain.SentenceStudyToken{Surface: row.Surface,
			RawLemma: row.RawLemma, EffectiveLemma: row.CanonicalLemma, UPOS: row.Upos,
			Dependency: row.Dependency, HeadSurface: row.HeadSurface,
			Ordinal: row.TokenOrdinal, HeadOrdinal: row.Head,
			StartOffset: row.StartOffset, EndOffset: row.EndOffset,
			Morphology: morphology})
	}
	decisions, err := s.queries().GetSentenceStudyDecisions(ctx, sqlcgen.GetSentenceStudyDecisionsParams{
		Owner: owner, Book: book, AnalysisRun: run, Corpus: corpus, UnitID: unit,
		SentenceStart: first.SentenceStartOffset, SentenceEnd: first.SentenceEndOffset,
	})
	if err != nil {
		return domain.SentenceStudy{}, err
	}
	attributeSentenceStudyDecisions(&study, decisions)
	return study, nil
}

// attributeSentenceStudyDecisions applies each decision only to the single
// persisted analyzed word its exact owner/Book/analysis/source-document/span
// addresses. A decision that addresses no word, more than one, or a word the
// selected sentence does not present is recorded as unresolved and applied to
// no word: a narrowed presentation subset never proves uniqueness, and no
// sibling is chosen by lemma, part of speech or surface.
func attributeSentenceStudyDecisions(study *domain.SentenceStudy, decisions []sqlcgen.GetSentenceStudyDecisionsRow) {
	wordBySpan := make(map[[2]int64]int, len(study.Tokens))
	for index, token := range study.Tokens {
		wordBySpan[[2]int64{token.StartOffset, token.EndOffset}] = index
	}
	for _, decision := range decisions {
		index, matched := wordBySpan[[2]int64{decision.StartOffset, decision.EndOffset}]
		// A decision with no addressed word, more than one, or a correction
		// with no proposed lemma (missing evidence) cannot be attributed.
		if !matched || decision.AddressedWords != 1 || (!decision.Excluded && decision.CanonicalLemma == "") {
			study.UnresolvedDecisions = append(study.UnresolvedDecisions, domain.SentenceStudyDecision{
				StartOffset: decision.StartOffset, EndOffset: decision.EndOffset,
				CanonicalLemma: decision.CanonicalLemma, Excluded: decision.Excluded,
				AddressedWords: decision.AddressedWords,
			})
			continue
		}
		token := &study.Tokens[index]
		if decision.Excluded {
			token.Excluded = true
			continue
		}
		token.Corrected = true
		token.EffectiveLemma = decision.CanonicalLemma
	}
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
