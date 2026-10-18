//go:build integration

package testutil

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/stretchr/testify/require"
)

// CorpusWord is one vocabulary identity seeded into a corpus's analysis
// evidence. Occurrences is how many tokens carry it; Sentence is each
// occurrence's sentence text, the lemma when empty.
type CorpusWord struct {
	Lemma                string
	UPOS                 string
	LexicalDiscriminator string
	Headword             string
	Sentence             string
	Occurrences          int
}

// SeedCorpusWords appends persisted analysis evidence to a complete corpus:
// one single-token sentence per occurrence, after any evidence the corpus
// already has. The Reading freeze projects this evidence, so it is how a test
// gives an analyzed Book vocabulary.
func SeedCorpusWords(t *testing.T, ctx context.Context, pool *pgxpool.Pool, ownerID, corpusID, language string, words ...CorpusWord) {
	t.Helper()
	var runID string
	var ordinal, offset int64
	require.NoError(t, pool.QueryRow(ctx, `SELECT c.analysis_run_id::text,
		COALESCE((SELECT max(s.sentence_ordinal)+1 FROM corpus_sentences s WHERE s.owner_id=c.owner_id AND s.corpus_id=c.id), 0),
		COALESCE((SELECT max(s.end_offset)+1 FROM corpus_sentences s WHERE s.owner_id=c.owner_id AND s.corpus_id=c.id), 0)
		FROM corpora c WHERE c.owner_id=$1 AND c.id=$2`, ownerID, corpusID).Scan(&runID, &ordinal, &offset))
	for _, word := range words {
		sentence := word.Sentence
		if sentence == "" {
			sentence = word.Lemma
		}
		for range word.Occurrences {
			end := offset + int64(len(word.Lemma))
			_, err := pool.Exec(ctx, `INSERT INTO corpus_sentences(owner_id,analysis_run_id,corpus_id,unit_id,sentence_ordinal,sentence_text,start_offset,end_offset) VALUES($1,$2,$3,'seeded-unit',$4,$5,$6,$7)`, ownerID, runID, corpusID, ordinal, sentence, offset, end)
			require.NoError(t, err)
			_, err = pool.Exec(ctx, `INSERT INTO corpus_tokens(owner_id,language,analysis_run_id,corpus_id,sentence_ordinal,token_ordinal,surface,raw_lemma,canonical_lemma,upos,dependency,head,morphology,start_offset,end_offset,lexical_discriminator,headword) VALUES($1,$2,$3,$4,$5,0,$6,$6,$6,$7,'root',0,'{}',$8,$9,$10,$11)`, ownerID, language, runID, corpusID, ordinal, word.Lemma, word.UPOS, offset, end, word.LexicalDiscriminator, word.Headword)
			require.NoError(t, err)
			ordinal, offset = ordinal+1, end+1
		}
	}
}
