package persistence

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/idiom"
)

// IdiomCountBuilderVersion identifies how a Book's Idiom candidate counts are
// derived. Raise it when the derivation changes so every Book is rebuilt.
const IdiomCountBuilderVersion = 1

// IdiomIndexVersion names the idiom evidence the counts were derived against:
// the matching rules and the dictionary index (which carries the reviewed idiom
// list) version. A count built against another value is stale.
func IdiomIndexVersion(dictionaryVersion string) string {
	return idiom.MatcherVersion + "|" + strings.TrimSpace(dictionaryVersion)
}

// ListCorpusIdiomCandidateCounts sums each Idiom headword's candidates across
// the owner's currently analyzed Books in the language. Only Books whose counts
// are ready for their current analysis contribute.
func (s *PostgresStore) ListCorpusIdiomCandidateCounts(ctx context.Context, owner, language string) ([]domain.IdiomCandidateCount, error) {
	rows, err := sqlcgen.New(s.pool).ListCorpusIdiomCandidateCounts(ctx, sqlcgen.ListCorpusIdiomCandidateCountsParams{
		BuilderVersion: IdiomCountBuilderVersion, OwnerID: owner, Language: language,
	})
	if err != nil {
		return nil, fmt.Errorf("list Corpus Idiom candidate counts: %w", err)
	}
	counts := make([]domain.IdiomCandidateCount, len(rows))
	for i, row := range rows {
		counts[i] = domain.IdiomCandidateCount{Headword: row.Headword, Candidates: row.Candidates, Books: row.Books}
	}
	return counts, nil
}

// BuildIdiomCandidateCountsTx replaces a Book's Idiom candidate counts for its
// current analysis, matching each sentence on effective lemmas: a confirmed
// Lemma correction supplies the lemma and an excluded Occurrence matches
// nothing. It reports false when the Book has no current analysis. Call it from
// a job, never a request. The publication fence matches Browse count builds: a
// decision committed during the scan makes the candidate roll back.
func BuildIdiomCandidateCountsTx(ctx context.Context, tx pgx.Tx, owner, book string, index idiom.Index, indexVersion string) (bool, error) {
	scope, found, err := currentIdiomCountScope(ctx, tx, owner, book)
	if err != nil || !found {
		return false, err
	}
	revision, err := BrowseCountDecisionRevision(ctx, tx, owner, book, scope.run)
	if err != nil {
		return false, fmt.Errorf("read occurrence-decision revision for Idiom counts: %w", err)
	}
	counts, err := deriveIdiomCandidateCounts(ctx, tx, scope, index)
	if err != nil {
		return false, err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM idiom_candidate_counts WHERE owner_id=$1 AND book_id=$2`, owner, book); err != nil {
		return false, fmt.Errorf("clear previous Idiom counts: %w", err)
	}
	headwords, candidates := make([]string, 0, len(counts)), make([]int64, 0, len(counts))
	for headword, count := range counts {
		headwords, candidates = append(headwords, headword), append(candidates, count)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO idiom_candidate_counts(owner_id,book_id,language,analysis_run_id,corpus_id,headword,candidate_count)
SELECT $1,$2,$3,$4,$5,i.headword,i.candidates FROM unnest($6::text[],$7::bigint[]) AS i(headword,candidates)`,
		owner, book, scope.language, scope.run, scope.corpus, headwords, candidates); err != nil {
		return false, fmt.Errorf("write Idiom counts: %w", err)
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 193))`, owner+":"+book); err != nil {
		return false, fmt.Errorf("fence Idiom count publication: %w", err)
	}
	current, err := BrowseCountDecisionRevision(ctx, tx, owner, book, scope.run)
	if err != nil {
		return false, fmt.Errorf("verify occurrence-decision revision for Idiom counts: %w", err)
	}
	if current != revision {
		return false, ErrBrowseCountDecisionsChanged
	}
	if _, err := tx.Exec(ctx, `INSERT INTO idiom_candidate_count_readiness(owner_id,book_id,language,analysis_run_id,corpus_id,builder_version,index_version)
VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(owner_id,book_id) DO UPDATE SET language=excluded.language,
analysis_run_id=excluded.analysis_run_id,corpus_id=excluded.corpus_id,builder_version=excluded.builder_version,
index_version=excluded.index_version,ready_at=now()`,
		owner, book, scope.language, scope.run, scope.corpus, IdiomCountBuilderVersion, indexVersion); err != nil {
		return false, fmt.Errorf("publish Idiom count readiness: %w", err)
	}
	return true, nil
}

// IdiomCountDecisionRevision is the occurrence-decision revision a queued
// rebuild is keyed by, so a new decision is never absorbed by a job already
// running.
func IdiomCountDecisionRevision(ctx context.Context, tx pgx.Tx, owner, book, run string) (string, error) {
	return BrowseCountDecisionRevision(ctx, tx, owner, book, run)
}

type idiomCountScope struct {
	owner, book, source, snapshot, run, corpus, language string
}

func currentIdiomCountScope(ctx context.Context, tx pgx.Tx, owner, book string) (idiomCountScope, bool, error) {
	scope := idiomCountScope{owner: owner, book: book}
	err := tx.QueryRow(ctx, `SELECT cai.source_material_id::text,ar.snapshot_id::text,cai.analysis_run_id::text,cai.corpus_id::text,s.language
		FROM current_analysis_identity cai
		JOIN source_materials s ON s.owner_id=cai.owner_id AND s.id=cai.source_material_id
		JOIN analysis_runs ar ON ar.owner_id=cai.owner_id AND ar.id=cai.analysis_run_id
		WHERE cai.owner_id=$1 AND cai.book_id=$2`, owner, book).
		Scan(&scope.source, &scope.snapshot, &scope.run, &scope.corpus, &scope.language)
	if errors.Is(err, pgx.ErrNoRows) {
		return idiomCountScope{}, false, nil
	}
	if err != nil {
		return idiomCountScope{}, false, fmt.Errorf("load current analysis for Idiom counts: %w", err)
	}
	return scope, true, nil
}

// deriveIdiomCandidateCounts streams the Book's tokens sentence by sentence and
// counts the candidates of each headword. The index is wrapped in a per-word
// cache because common words recur in nearly every sentence.
func deriveIdiomCandidateCounts(ctx context.Context, tx pgx.Tx, scope idiomCountScope, index idiom.Index) (map[string]int64, error) {
	rows, err := tx.Query(ctx, `
SELECT t.sentence_ordinal,t.surface,t.canonical_lemma,d.canonical_lemma,COALESCE(d.excluded,false)
FROM corpus_tokens t
JOIN corpus_sentences s ON s.owner_id=t.owner_id AND s.analysis_run_id=t.analysis_run_id
 AND s.corpus_id=t.corpus_id AND s.sentence_ordinal=t.sentence_ordinal
JOIN source_material_units u ON u.owner_id=$1 AND u.source_material_id=$3
 AND u.snapshot_id=$7 AND u.unit_id=s.unit_id
LEFT JOIN occurrence_lemma_corrections d ON d.owner_id=$1 AND d.book_id=$2
 AND d.analysis_run_id=t.analysis_run_id AND d.corpus_id=t.corpus_id
 AND d.source_document_id=s.unit_id AND d.start_offset=t.start_offset AND d.end_offset=t.end_offset
WHERE t.owner_id=$1 AND t.analysis_run_id=$4 AND t.corpus_id=$5 AND t.language=$6
ORDER BY t.sentence_ordinal,t.token_ordinal`,
		scope.owner, scope.book, scope.source, scope.run, scope.corpus, scope.language, scope.snapshot)
	if err != nil {
		return nil, fmt.Errorf("load Idiom count tokens: %w", err)
	}
	defer rows.Close()
	counter := &idiomSentenceCounter{ctx: ctx, index: idiom.NewCachedIndex(index), language: scope.language, counts: make(map[string]int64)}
	current := int64(-1)
	for rows.Next() {
		var ordinal int64
		var surface, lemma string
		var correction *string
		var excluded bool
		if err := rows.Scan(&ordinal, &surface, &lemma, &correction, &excluded); err != nil {
			return nil, fmt.Errorf("scan Idiom count tokens: %w", err)
		}
		if ordinal != current {
			if err := counter.flush(); err != nil {
				return nil, err
			}
			current = ordinal
		}
		counter.sentence = append(counter.sentence, effectiveIdiomToken(surface, lemma, correction, excluded))
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read Idiom count tokens: %w", err)
	}
	if err := counter.flush(); err != nil {
		return nil, err
	}
	return counter.counts, nil
}

// idiomSentenceCounter accumulates one sentence's tokens and, on flush, adds
// the sentence's candidates to the per-headword counts.
type idiomSentenceCounter struct {
	ctx      context.Context
	index    idiom.Index
	language string
	sentence []analyzer.Token
	counts   map[string]int64
}

func (c *idiomSentenceCounter) flush() error {
	if len(c.sentence) == 0 {
		return nil
	}
	candidates, err := idiom.CandidatesInSentence(c.ctx, c.index, idiom.DefaultParameters(), c.language, c.sentence)
	if err != nil {
		return fmt.Errorf("match Idiom candidates: %w", err)
	}
	for _, candidate := range candidates {
		c.counts[candidate.Entry.Headword]++
	}
	c.sentence = c.sentence[:0]
	return nil
}

// effectiveIdiomToken applies the Book's decisions to one token: an excluded
// Occurrence keeps its position but matches nothing, and a confirmed correction
// replaces the lemma.
func effectiveIdiomToken(surface, lemma string, correction *string, excluded bool) analyzer.Token {
	if excluded {
		return analyzer.Token{}
	}
	if correction != nil && strings.TrimSpace(*correction) != "" {
		lemma = *correction
	}
	return analyzer.Token{Surface: surface, CanonicalLemma: strings.TrimSpace(lemma)}
}
