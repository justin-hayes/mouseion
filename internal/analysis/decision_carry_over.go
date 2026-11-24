package analysis

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
)

// carryDecisionsOntoIdenticalTokens copies the Book's Lemma corrections and
// Occurrence exclusions from the run being replaced onto the new run, wherever
// the new run holds an identical token (ADR 0126). It runs in the publishing
// transaction, after the eligibility check and before the Browse projection, so
// the projection already reflects the copied decisions.
//
// Nothing is copied unless the Book has a previous current run that differs from
// the new run and shares its content revision and its extracted-unit snapshot.
// A Source structure update (ADR 0103) adopts a new snapshot under the same
// content revision, and a changed source has a new revision; both carry nothing.
// The previous run's decisions are never changed.
//
// A token is identical when its source document, offset span, surface and
// source (raw) lemma all match; UPOS and morphology are deliberately not
// compared. A correction whose lemma the new token already has is not copied but
// counts as carried over ("absorbed"). The outcome is recorded in
// analysis_decision_carry_overs, including 0 of 0, with the River job that
// published the run (zero when none did).
func carryDecisionsOntoIdenticalTokens(ctx context.Context, tx pgx.Tx, owner, book, newRun, corpus string, riverJobID int64) error {
	var previousRun string
	err := tx.QueryRow(ctx, `
		SELECT p.analysis_run_id::text
		FROM book_current_analyses p
		JOIN analysis_runs previous ON previous.owner_id=p.owner_id AND previous.id=p.analysis_run_id
		JOIN analysis_runs next ON next.owner_id=p.owner_id AND next.id=$3
		WHERE p.owner_id=$1 AND p.book_id=$2
		  AND p.analysis_run_id<>next.id
		  AND previous.content_revision_id=next.content_revision_id
		  AND previous.snapshot_id=next.snapshot_id`, owner, book, newRun).Scan(&previousRun)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("find analysis run whose decisions carry over: %w", err)
	}
	var copied, absorbed, total int
	if err := tx.QueryRow(ctx, `
		WITH matched AS (
			SELECT DISTINCT ON (o.source_document_id, o.start_offset, o.end_offset)
			       o.source_document_id, o.start_offset, o.end_offset, o.excluded, o.canonical_lemma,
			       o.normalization_profile, o.normalization_version,
			       nt.canonical_lemma AS new_canonical_lemma
			FROM occurrence_lemma_corrections o
			JOIN corpus_sentences os ON os.owner_id=o.owner_id AND os.analysis_run_id=o.analysis_run_id AND os.unit_id=o.source_document_id
			JOIN corpus_tokens ot ON ot.owner_id=os.owner_id AND ot.analysis_run_id=os.analysis_run_id AND ot.sentence_ordinal=os.sentence_ordinal
			 AND ot.start_offset=o.start_offset AND ot.end_offset=o.end_offset
			JOIN corpus_sentences ns ON ns.owner_id=o.owner_id AND ns.analysis_run_id=$4 AND ns.unit_id=o.source_document_id
			JOIN corpus_tokens nt ON nt.owner_id=ns.owner_id AND nt.analysis_run_id=ns.analysis_run_id AND nt.sentence_ordinal=ns.sentence_ordinal
			 AND nt.start_offset=o.start_offset AND nt.end_offset=o.end_offset
			 AND nt.surface=ot.surface AND nt.raw_lemma=ot.raw_lemma
			WHERE o.owner_id=$1 AND o.book_id=$2 AND o.analysis_run_id=$3
			ORDER BY o.source_document_id, o.start_offset, o.end_offset, nt.sentence_ordinal, nt.token_ordinal
		), inserted AS (
			INSERT INTO occurrence_lemma_corrections(owner_id,book_id,corpus_id,analysis_run_id,source_document_id,start_offset,end_offset,canonical_lemma,normalization_profile,normalization_version,excluded)
			SELECT $1,$2,$5,$4,source_document_id,start_offset,end_offset,canonical_lemma,normalization_profile,normalization_version,excluded
			FROM matched
			WHERE excluded OR canonical_lemma IS DISTINCT FROM new_canonical_lemma
			ON CONFLICT DO NOTHING
			RETURNING 1
		)
		SELECT (SELECT count(*) FROM inserted),
		       (SELECT count(*) FROM matched WHERE NOT excluded AND canonical_lemma IS NOT DISTINCT FROM new_canonical_lemma),
		       (SELECT count(*) FROM occurrence_lemma_corrections WHERE owner_id=$1 AND book_id=$2 AND analysis_run_id=$3)`,
		owner, book, previousRun, newRun, corpus).Scan(&copied, &absorbed, &total); err != nil {
		return fmt.Errorf("carry decisions onto identical tokens: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO analysis_decision_carry_overs(owner_id,book_id,analysis_run_id,previous_analysis_run_id,river_job_id,carried,total)
		VALUES($1,$2,$3,$4,NULLIF($5::bigint,0),$6,$7) ON CONFLICT(owner_id,analysis_run_id) DO NOTHING`,
		owner, book, newRun, previousRun, riverJobID, copied+absorbed, total); err != nil {
		return fmt.Errorf("record carried-over decisions: %w", err)
	}
	return nil
}
