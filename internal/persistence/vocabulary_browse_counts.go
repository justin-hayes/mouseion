package persistence

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/checked"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/selection"
)

// Version 3 derives counts only through selection.Project (ADR 0087). Version 2
// normalized identities like the projection in SQL; version 1 matched verbatim.
const vocabularyBrowseCountBuilderVersion = 3

var ErrBrowseCountDecisionsChanged = errors.New("occurrence decisions changed during Browse count rebuild")

type browseCountReadiness struct {
	ready    bool
	owner    string
	book     string
	source   string
	snapshot string
	run      string
	corpus   string
	language string
}

type browseCountIdentity struct {
	lemma string
	upos  string
}

// browseCountReadinessForDecision is called under the same per-Book advisory
// lock as count-build publication. Only an exact current projection is safe to
// update incrementally; every other state must be rebuilt.
func browseCountReadinessForDecision(ctx context.Context, tx pgx.Tx, owner, book string) (browseCountReadiness, error) {
	var result browseCountReadiness
	result.owner, result.book = owner, book
	err := tx.QueryRow(ctx, `SELECT cai.source_material_id::text,ar.snapshot_id::text,cai.analysis_run_id::text,cai.corpus_id::text,s.language,
		EXISTS(SELECT 1 FROM vocabulary_browse_count_readiness r WHERE r.owner_id=cai.owner_id AND r.book_id=cai.book_id
		  AND r.analysis_run_id=cai.analysis_run_id AND r.corpus_id=cai.corpus_id
		  AND r.language=s.language AND r.builder_version=$3)
		FROM current_analysis_identity cai JOIN source_materials s ON s.owner_id=cai.owner_id AND s.id=cai.source_material_id
		JOIN analysis_runs ar ON ar.owner_id=cai.owner_id AND ar.id=cai.analysis_run_id
		WHERE cai.owner_id=$1 AND cai.book_id=$2`, owner, book, vocabularyBrowseCountBuilderVersion).
		Scan(&result.source, &result.snapshot, &result.run, &result.corpus, &result.language, &result.ready)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return browseCountReadiness{}, fmt.Errorf("read exact Browse count readiness: %w", err)
	}
	return result, nil
}

// applyVocabularyBrowseDecisionDeltasTx keeps a ready projection in step with
// the decision rows in the same transaction. It re-projects only the
// occurrences of the affected identities (the old and new lemma of each
// decision), so its work is bounded by those identities, not by the Book.
func applyVocabularyBrowseDecisionDeltasTx(ctx context.Context, tx pgx.Tx, ready browseCountReadiness, decisions []domain.LemmaReviewDecision) error {
	affected := make(map[browseCountIdentity]struct{}, len(decisions)*2)
	lemmaSet := make(map[string]struct{}, len(decisions)*2)
	touch := func(lemma, upos string) {
		identity := normalizedBrowseCountIdentity(lemma, upos)
		if identity.lemma == "" {
			return
		}
		affected[identity] = struct{}{}
		lemmaSet[identity.lemma] = struct{}{}
	}
	for _, decision := range decisions {
		o := decision.Occurrence
		oldLemma := o.CorrectedLemma
		if oldLemma == "" {
			oldLemma = o.CanonicalLemma
		}
		touch(oldLemma, o.UPOS)
		touch(decision.CanonicalLemma, o.UPOS)
	}
	lemmas := make([]string, 0, len(lemmaSet))
	for lemma := range lemmaSet {
		lemmas = append(lemmas, lemma)
	}
	projected, err := projectBrowseCounts(ctx, tx, ready, lemmas)
	if err != nil {
		return err
	}
	for identity := range affected {
		count, ok := projected[identity]
		if !ok {
			if _, err := tx.Exec(ctx, `DELETE FROM vocabulary_browse_counts
				WHERE owner_id=$1 AND book_id=$2 AND analysis_run_id=$3 AND corpus_id=$4 AND canonical_lemma=$5 AND upos=$6`,
				ready.owner, ready.book, ready.run, ready.corpus, identity.lemma, identity.upos); err != nil {
				return fmt.Errorf("remove empty effective Browse identity: %w", err)
			}
			continue
		}
		if _, err := tx.Exec(ctx, `INSERT INTO vocabulary_browse_counts(owner_id,book_id,language,analysis_run_id,corpus_id,canonical_lemma,upos,occurrence_count,corrected)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)
			ON CONFLICT(owner_id,book_id,analysis_run_id,canonical_lemma,upos)
			DO UPDATE SET occurrence_count=excluded.occurrence_count,corrected=excluded.corrected`,
			ready.owner, ready.book, ready.language, ready.run, ready.corpus, identity.lemma, identity.upos, count.occurrences, count.corrected); err != nil {
			return fmt.Errorf("write effective Browse count: %w", err)
		}
	}
	return nil
}

func normalizedBrowseCountIdentity(lemma, upos string) browseCountIdentity {
	return browseCountIdentity{lemma: strings.TrimSpace(lemma), upos: strings.ToUpper(strings.TrimSpace(upos))}
}

type browseCountValue struct {
	occurrences int64
	corrected   bool
}

// projectBrowseCounts fetches persisted facts and lets selection.Project decide
// eligibility and effective identity. A non-nil lemmas list only narrows which
// token rows are fetched (by raw or corrected lemma); the returned counts are
// exact for every identity whose lemma is listed because Project judges each
// occurrence independently.
func projectBrowseCounts(ctx context.Context, tx pgx.Tx, scope browseCountReadiness, lemmas []string) (map[browseCountIdentity]browseCountValue, error) {
	rows, err := tx.Query(ctx, `
SELECT s.sentence_ordinal,s.unit_id,t.canonical_lemma,t.upos,t.dependency,t.start_offset,t.end_offset,d.canonical_lemma,COALESCE(d.excluded,false)
FROM corpus_tokens t
JOIN corpus_sentences s ON s.owner_id=t.owner_id AND s.analysis_run_id=t.analysis_run_id
 AND s.corpus_id=t.corpus_id AND s.sentence_ordinal=t.sentence_ordinal
JOIN source_material_units u ON u.owner_id=$1 AND u.source_material_id=$3
 AND u.snapshot_id=$7 AND u.unit_id=s.unit_id
LEFT JOIN occurrence_lemma_corrections d ON d.owner_id=$1 AND d.book_id=$2
 AND d.analysis_run_id=t.analysis_run_id AND d.corpus_id=t.corpus_id
 AND d.source_document_id=s.unit_id AND d.start_offset=t.start_offset AND d.end_offset=t.end_offset
WHERE t.owner_id=$1 AND t.analysis_run_id=$4 AND t.corpus_id=$5 AND t.language=$6
 AND ($8::text[] IS NULL OR btrim(t.canonical_lemma)=ANY($8) OR btrim(d.canonical_lemma)=ANY($8))
ORDER BY s.sentence_ordinal,t.token_ordinal`,
		scope.owner, scope.book, scope.source, scope.run, scope.corpus, scope.language, scope.snapshot, lemmas)
	if err != nil {
		return nil, fmt.Errorf("load Browse count facts: %w", err)
	}
	defer rows.Close()
	analysis := analyzer.Result{Language: scope.language}
	var decisions []selection.OccurrenceDecision
	currentOrdinal := int64(-1)
	for rows.Next() {
		var (
			ordinal, start, end int64
			unit, lemma, upos   string
			dependency          string
			correction          *string
			excluded            bool
		)
		if err := rows.Scan(&ordinal, &unit, &lemma, &upos, &dependency, &start, &end, &correction, &excluded); err != nil {
			return nil, fmt.Errorf("scan Browse count facts: %w", err)
		}
		startOffset, startErr := checked.Uint64FromInt64(start)
		endOffset, endErr := checked.Uint64FromInt64(end)
		if startErr != nil || endErr != nil {
			return nil, errors.New("persisted token contains invalid offsets")
		}
		if ordinal != currentOrdinal || len(analysis.Sentences) == 0 {
			analysis.Sentences = append(analysis.Sentences, analyzer.Sentence{})
			currentOrdinal = ordinal
		}
		location := analyzer.SourceLocation{SourceDocumentID: unit, StartOffset: startOffset, EndOffset: endOffset}
		last := &analysis.Sentences[len(analysis.Sentences)-1]
		last.Tokens = append(last.Tokens, analyzer.Token{CanonicalLemma: lemma, UPOS: upos, Dependency: dependency, Location: location})
		switch {
		case excluded:
			decisions = append(decisions, selection.OccurrenceDecision{Occurrence: selection.OccurrenceIdentity{SourceDocumentID: unit, StartOffset: startOffset, EndOffset: endOffset}, Excluded: true})
		case correction != nil && strings.TrimSpace(*correction) != "":
			decisions = append(decisions, selection.OccurrenceDecision{Occurrence: selection.OccurrenceIdentity{SourceDocumentID: unit, StartOffset: startOffset, EndOffset: endOffset}, Lemma: *correction})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read Browse count facts: %w", err)
	}
	corrected := make(map[selection.OccurrenceIdentity]struct{}, len(decisions))
	for _, decision := range decisions {
		if !decision.Excluded {
			corrected[decision.Occurrence] = struct{}{}
		}
	}
	candidates, err := selection.Project(analysis, selection.DefaultConfig(scope.corpus), decisions)
	if err != nil {
		return nil, fmt.Errorf("project Browse counts: %w", err)
	}
	result := make(map[browseCountIdentity]browseCountValue, len(candidates))
	for _, candidate := range candidates {
		value := browseCountValue{occurrences: int64(candidate.OccurrenceCount)}
		for _, ref := range candidate.SentenceReferences {
			if _, ok := corrected[selection.OccurrenceIdentity{SourceDocumentID: ref.Location.SourceDocumentID, StartOffset: ref.Location.StartOffset, EndOffset: ref.Location.EndOffset}]; ok {
				value.corrected = true
				break
			}
		}
		result[browseCountIdentity{lemma: candidate.Identity.CanonicalLemma, upos: candidate.Identity.UPOS}] = value
	}
	return result, nil
}

// BuildVocabularyBrowseCountsTx replaces a Book's count projection for one
// completed analysis. Call it in the same transaction that publishes that
// analysis so readers never observe a pointer without matching ready counts.
func BuildVocabularyBrowseCountsTx(ctx context.Context, tx pgx.Tx, owner, book, source, run, corpus, language string) error {
	decisionRevision, err := browseCountDecisionRevision(ctx, tx, owner, book, run)
	if err != nil {
		return fmt.Errorf("read occurrence-decision revision for Browse counts: %w", err)
	}
	var snapshot string
	if err := tx.QueryRow(ctx, `SELECT snapshot_id::text FROM analysis_runs WHERE owner_id=$1 AND id=$2`, owner, run).Scan(&snapshot); err != nil {
		return fmt.Errorf("load analysis snapshot for Browse counts: %w", err)
	}
	scope := browseCountReadiness{owner: owner, book: book, source: source, snapshot: snapshot, run: run, corpus: corpus, language: language}
	projected, err := projectBrowseCounts(ctx, tx, scope, nil)
	if err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `DELETE FROM vocabulary_browse_counts WHERE owner_id=$1 AND book_id=$2`, owner, book); err != nil {
		return fmt.Errorf("clear previous Browse counts: %w", err)
	}
	lemmas, uposes := make([]string, 0, len(projected)), make([]string, 0, len(projected))
	counts, flags := make([]int64, 0, len(projected)), make([]bool, 0, len(projected))
	for identity, value := range projected {
		lemmas, uposes = append(lemmas, identity.lemma), append(uposes, identity.upos)
		counts, flags = append(counts, value.occurrences), append(flags, value.corrected)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO vocabulary_browse_counts(owner_id,book_id,language,analysis_run_id,corpus_id,canonical_lemma,upos,occurrence_count,corrected)
SELECT $1,$2,$3,$4,$5,i.lemma,i.upos,i.occurrences,i.corrected
FROM unnest($6::text[],$7::text[],$8::bigint[],$9::boolean[]) AS i(lemma,upos,occurrences,corrected)`,
		owner, book, language, run, corpus, lemmas, uposes, counts, flags); err != nil {
		return fmt.Errorf("write effective Browse counts: %w", err)
	}
	// Match occurrence decisions' per-Book advisory lock only for this short
	// publication fence, never for the evidence scan above. A decision that
	// commits during the scan changes the revision and makes this candidate
	// transaction roll back; one that starts after the fence removes readiness
	// after this transaction commits.
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1, 193))`, owner+":"+book); err != nil {
		return fmt.Errorf("fence Browse count publication: %w", err)
	}
	currentDecisionRevision, err := browseCountDecisionRevision(ctx, tx, owner, book, run)
	if err != nil {
		return fmt.Errorf("verify occurrence-decision revision for Browse counts: %w", err)
	}
	if currentDecisionRevision != decisionRevision {
		return ErrBrowseCountDecisionsChanged
	}
	if _, err := tx.Exec(ctx, `INSERT INTO vocabulary_browse_count_readiness(owner_id,book_id,language,analysis_run_id,corpus_id,builder_version)
VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(owner_id,book_id) DO UPDATE SET language=excluded.language,
analysis_run_id=excluded.analysis_run_id,corpus_id=excluded.corpus_id,builder_version=excluded.builder_version,ready_at=now()`,
		owner, book, language, run, corpus, vocabularyBrowseCountBuilderVersion); err != nil {
		return fmt.Errorf("publish Browse count readiness: %w", err)
	}
	return nil
}

func browseCountDecisionRevision(ctx context.Context, tx pgx.Tx, owner, book, run string) (string, error) {
	var revision string
	err := tx.QueryRow(ctx, `SELECT md5(COALESCE(jsonb_agg(jsonb_build_array(source_document_id,start_offset,end_offset,canonical_lemma,excluded)
		ORDER BY source_document_id,start_offset,end_offset)::text,'[]'))
		FROM occurrence_lemma_corrections WHERE owner_id=$1 AND book_id=$2 AND analysis_run_id=$3`, owner, book, run).Scan(&revision)
	return revision, err
}
