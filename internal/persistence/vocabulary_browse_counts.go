package persistence

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// Version 2 normalizes identities like the shared selection projection: trimmed
// lemma and trimmed upper-case UPOS. Version 1 matched them verbatim.
const vocabularyBrowseCountBuilderVersion = 2

var ErrBrowseCountDecisionsChanged = errors.New("occurrence decisions changed during Browse count rebuild")

type browseCountReadiness struct {
	ready    bool
	owner    string
	book     string
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
	err := tx.QueryRow(ctx, `SELECT cai.analysis_run_id::text,cai.corpus_id::text,s.language,
		EXISTS(SELECT 1 FROM vocabulary_browse_count_readiness r WHERE r.owner_id=cai.owner_id AND r.book_id=cai.book_id
		  AND r.analysis_run_id=cai.analysis_run_id AND r.corpus_id=cai.corpus_id
		  AND r.language=s.language AND r.builder_version=$3)
		FROM current_analysis_identity cai JOIN source_materials s ON s.owner_id=cai.owner_id AND s.id=cai.source_material_id
		WHERE cai.owner_id=$1 AND cai.book_id=$2`, owner, book, vocabularyBrowseCountBuilderVersion).
		Scan(&result.run, &result.corpus, &result.language, &result.ready)
	if errors.Is(err, pgx.ErrNoRows) {
		return result, nil
	}
	if err != nil {
		return browseCountReadiness{}, fmt.Errorf("read exact Browse count readiness: %w", err)
	}
	return result, nil
}

// applyVocabularyBrowseDecisionDeltasTx keeps a ready projection in step with
// the decision rows in the same transaction. Its work is bounded by the
// selected occurrences, not by the full Book evidence scan.
func applyVocabularyBrowseDecisionDeltasTx(ctx context.Context, tx pgx.Tx, ready browseCountReadiness, decisions []domain.LemmaReviewDecision) error {
	deltas := make(map[browseCountIdentity]int64, len(decisions)*2)
	affected := make(map[browseCountIdentity]struct{}, len(decisions)*2)
	for _, decision := range decisions {
		o := decision.Occurrence
		oldLemma := o.CorrectedLemma
		if oldLemma == "" {
			oldLemma = o.CanonicalLemma
		}
		if !o.Excluded && browseCountIdentityEligible(oldLemma, o.UPOS) {
			identity := normalizedBrowseCountIdentity(oldLemma, o.UPOS)
			deltas[identity]--
			affected[identity] = struct{}{}
		}
		if !decision.Excluded && browseCountIdentityEligible(decision.CanonicalLemma, o.UPOS) {
			identity := normalizedBrowseCountIdentity(decision.CanonicalLemma, o.UPOS)
			deltas[identity]++
			affected[identity] = struct{}{}
		}
	}
	for identity := range affected {
		delta := deltas[identity]
		if delta < 0 {
			tag, err := tx.Exec(ctx, `DELETE FROM vocabulary_browse_counts
				WHERE owner_id=$1 AND book_id=$2 AND analysis_run_id=$3 AND corpus_id=$4 AND canonical_lemma=$5 AND upos=$7
				AND occurrence_count=-$6::bigint`, ready.owner, ready.book, ready.run, ready.corpus, identity.lemma, delta, identity.upos)
			if err != nil {
				return fmt.Errorf("remove empty effective Browse identity: %w", err)
			}
			if tag.RowsAffected() == 0 {
				tag, err = tx.Exec(ctx, `UPDATE vocabulary_browse_counts SET occurrence_count=occurrence_count+$6
					WHERE owner_id=$1 AND book_id=$2 AND analysis_run_id=$3 AND corpus_id=$4 AND canonical_lemma=$5 AND upos=$7
					AND occurrence_count>-$6::bigint`, ready.owner, ready.book, ready.run, ready.corpus, identity.lemma, delta, identity.upos)
				if err != nil {
					return fmt.Errorf("decrement effective Browse count: %w", err)
				}
				if tag.RowsAffected() != 1 {
					return fmt.Errorf("effective Browse count underflow for %s/%s", identity.lemma, identity.upos)
				}
			}
		} else if delta > 0 {
			if _, err := tx.Exec(ctx, `INSERT INTO vocabulary_browse_counts(owner_id,book_id,language,analysis_run_id,corpus_id,canonical_lemma,upos,occurrence_count)
				VALUES($1,$2,$3,$4,$5,$6,$7,$8)
				ON CONFLICT(owner_id,book_id,analysis_run_id,canonical_lemma,upos)
				DO UPDATE SET occurrence_count=vocabulary_browse_counts.occurrence_count+excluded.occurrence_count`,
				ready.owner, ready.book, ready.language, ready.run, ready.corpus, identity.lemma, identity.upos, delta); err != nil {
				return fmt.Errorf("increment effective Browse count: %w", err)
			}
		}
		var corrected bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS(
			SELECT 1 FROM corpus_tokens t
			JOIN corpus_sentences s ON s.owner_id=t.owner_id AND s.analysis_run_id=t.analysis_run_id AND s.corpus_id=t.corpus_id AND s.sentence_ordinal=t.sentence_ordinal
			JOIN source_material_units u ON u.owner_id=$1 AND u.source_material_id=(SELECT source_material_id FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2)
				AND u.snapshot_id=(SELECT snapshot_id FROM analysis_runs WHERE owner_id=$1 AND id=$3) AND u.unit_id=s.unit_id
			LEFT JOIN occurrence_lemma_corrections d ON d.owner_id=$1 AND d.book_id=$2 AND d.analysis_run_id=t.analysis_run_id AND d.corpus_id=t.corpus_id
				AND d.source_document_id=s.unit_id AND d.start_offset=t.start_offset AND d.end_offset=t.end_offset
			WHERE t.owner_id=$1 AND t.analysis_run_id=$3 AND t.corpus_id=$4 AND t.language=$5
				AND upper(btrim(t.upos))=$7 AND upper(btrim(t.upos)) IN ('NOUN','VERB','ADJ','ADV') AND t.dependency <> 'compound:prt'
				AND COALESCE(d.excluded,false)=false AND d.canonical_lemma IS NOT NULL
				AND btrim(COALESCE(d.canonical_lemma,t.canonical_lemma))=$6
				AND btrim(COALESCE(d.canonical_lemma,t.canonical_lemma)) ~ '[[:alpha:]]')`,
			ready.owner, ready.book, ready.run, ready.corpus, ready.language, identity.lemma, identity.upos).Scan(&corrected); err != nil {
			return fmt.Errorf("recompute corrected Browse identity state: %w", err)
		}
		if _, err := tx.Exec(ctx, `UPDATE vocabulary_browse_counts SET corrected=$6 WHERE owner_id=$1 AND book_id=$2 AND analysis_run_id=$3 AND corpus_id=$4 AND canonical_lemma=$5 AND upos=$7`,
			ready.owner, ready.book, ready.run, ready.corpus, identity.lemma, corrected, identity.upos); err != nil {
			return fmt.Errorf("update corrected Browse identity state: %w", err)
		}
	}
	return nil
}

func normalizedBrowseCountIdentity(lemma, upos string) browseCountIdentity {
	return browseCountIdentity{lemma: strings.TrimSpace(lemma), upos: strings.ToUpper(strings.TrimSpace(upos))}
}

func browseCountIdentityEligible(lemma, upos string) bool {
	switch strings.ToUpper(strings.TrimSpace(upos)) {
	case "NOUN", "VERB", "ADJ", "ADV":
		return strings.ContainsFunc(lemma, unicode.IsLetter)
	}
	return false
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
	if _, err := tx.Exec(ctx, `DELETE FROM vocabulary_browse_counts WHERE owner_id=$1 AND book_id=$2`, owner, book); err != nil {
		return fmt.Errorf("clear previous Browse counts: %w", err)
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO vocabulary_browse_counts(owner_id,book_id,language,analysis_run_id,corpus_id,canonical_lemma,upos,occurrence_count,corrected)
SELECT t.owner_id,$2,$6,t.analysis_run_id,t.corpus_id,
       btrim(COALESCE(d.canonical_lemma,t.canonical_lemma)),upper(btrim(t.upos)),count(*)::bigint,bool_or(d.canonical_lemma IS NOT NULL)
FROM corpus_tokens t
JOIN corpus_sentences s ON s.owner_id=t.owner_id AND s.analysis_run_id=t.analysis_run_id
 AND s.corpus_id=t.corpus_id AND s.sentence_ordinal=t.sentence_ordinal
JOIN source_material_units u ON u.owner_id=$1 AND u.source_material_id=$3
 AND u.snapshot_id=$7 AND u.unit_id=s.unit_id
LEFT JOIN occurrence_lemma_corrections d ON d.owner_id=$1 AND d.book_id=$2
 AND d.analysis_run_id=t.analysis_run_id AND d.corpus_id=t.corpus_id
 AND d.source_document_id=s.unit_id AND d.start_offset=t.start_offset AND d.end_offset=t.end_offset
WHERE t.owner_id=$1 AND t.analysis_run_id=$4 AND t.corpus_id=$5 AND t.language=$6
 AND upper(btrim(t.upos)) IN ('NOUN','VERB','ADJ','ADV') AND t.dependency <> 'compound:prt'
 AND COALESCE(d.excluded,false)=false AND btrim(COALESCE(d.canonical_lemma,t.canonical_lemma)) ~ '[[:alpha:]]'
GROUP BY t.owner_id,t.analysis_run_id,t.corpus_id,btrim(COALESCE(d.canonical_lemma,t.canonical_lemma)),upper(btrim(t.upos))`,
		owner, book, source, run, corpus, language, snapshot); err != nil {
		return fmt.Errorf("build effective Browse counts: %w", err)
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
