package persistence

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"
)

const vocabularyBrowseCountBuilderVersion = 1

// BuildVocabularyBrowseCountsTx replaces a Book's count projection for one
// completed analysis. Call it in the same transaction that publishes that
// analysis so readers never observe a pointer without matching ready counts.
func BuildVocabularyBrowseCountsTx(ctx context.Context, tx pgx.Tx, owner, book, source, run, corpus, language string) error {
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
       COALESCE(d.canonical_lemma,t.canonical_lemma),t.upos,count(*)::bigint,bool_or(d.canonical_lemma IS NOT NULL)
FROM corpus_tokens t
JOIN corpus_sentences s ON s.owner_id=t.owner_id AND s.analysis_run_id=t.analysis_run_id
 AND s.corpus_id=t.corpus_id AND s.sentence_ordinal=t.sentence_ordinal
JOIN source_material_units u ON u.owner_id=$1 AND u.source_material_id=$3
 AND u.snapshot_id=$7 AND u.unit_id=s.unit_id
LEFT JOIN occurrence_lemma_corrections d ON d.owner_id=$1 AND d.book_id=$2
 AND d.analysis_run_id=t.analysis_run_id AND d.corpus_id=t.corpus_id
 AND d.source_document_id=s.unit_id AND d.start_offset=t.start_offset AND d.end_offset=t.end_offset
WHERE t.owner_id=$1 AND t.analysis_run_id=$4 AND t.corpus_id=$5 AND t.language=$6
 AND t.upos IN ('NOUN','VERB','ADJ','ADV') AND t.dependency <> 'compound:prt'
 AND COALESCE(d.excluded,false)=false AND COALESCE(d.canonical_lemma,t.canonical_lemma) ~ '[[:alpha:]]'
GROUP BY t.owner_id,t.analysis_run_id,t.corpus_id,COALESCE(d.canonical_lemma,t.canonical_lemma),t.upos`,
		owner, book, source, run, corpus, language, snapshot); err != nil {
		return fmt.Errorf("build effective Browse counts: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO vocabulary_browse_count_readiness(owner_id,book_id,language,analysis_run_id,corpus_id,builder_version)
VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT(owner_id,book_id) DO UPDATE SET language=excluded.language,
analysis_run_id=excluded.analysis_run_id,corpus_id=excluded.corpus_id,builder_version=excluded.builder_version,ready_at=now()`,
		owner, book, language, run, corpus, vocabularyBrowseCountBuilderVersion); err != nil {
		return fmt.Errorf("publish Browse count readiness: %w", err)
	}
	return nil
}
