//go:build integration

package testutil

import (
	"context"
	"encoding/json"
	"testing"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/checked"
	"github.com/stretchr/testify/require"
)

// SeedCorpusWords appends persisted analysis evidence to a complete corpus: one
// gate-passing sentence per occurrence (a one-token fragment when the word asks
// for one), after any evidence the corpus already has. The Reading freeze
// projects this evidence, so it is how a test gives an analyzed Book
// vocabulary.
func SeedCorpusWords(t *testing.T, ctx context.Context, pool *pgxpool.Pool, ownerID, corpusID, language string, words ...CorpusWord) {
	t.Helper()
	var runID string
	var ordinal, offset int64
	require.NoError(t, pool.QueryRow(ctx, `SELECT c.analysis_run_id::text,
		COALESCE((SELECT max(s.sentence_ordinal)+1 FROM corpus_sentences s WHERE s.owner_id=c.owner_id AND s.corpus_id=c.id), 0),
		COALESCE((SELECT max(s.end_offset)+1 FROM corpus_sentences s WHERE s.owner_id=c.owner_id AND s.corpus_id=c.id), 0)
		FROM corpora c WHERE c.owner_id=$1 AND c.id=$2`, ownerID, corpusID).Scan(&runID, &ordinal, &offset))
	for _, word := range words {
		sentence := CorpusSentence(language, word)
		for range word.Occurrences {
			ordinal, offset = SeedCorpusSentence(t, ctx, pool, ownerID, corpusID, language, runID, "seeded-unit", ordinal, offset, sentence)
		}
	}
}

// SeedCorpusSentence appends one seeded sentence and its dependency tokens to a
// corpus, returning the next free sentence ordinal and start offset. Token
// offsets in sentence are relative to the sentence's start; the persisted rows
// carry document-absolute offsets.
func SeedCorpusSentence(t *testing.T, ctx context.Context, pool *pgxpool.Pool, ownerID, corpusID, language, runID, unitID string, ordinal, offset int64, sentence SeededSentence) (int64, int64) {
	t.Helper()
	sentenceEnd := offset + int64(utf8.RuneCountInString(sentence.Text))
	_, err := pool.Exec(ctx, `INSERT INTO corpus_sentences(owner_id,analysis_run_id,corpus_id,unit_id,sentence_ordinal,sentence_text,start_offset,end_offset) VALUES($1,$2,$3,$4,$5,$6,$7,$8)`, ownerID, runID, corpusID, unitID, ordinal, sentence.Text, offset, sentenceEnd)
	require.NoError(t, err)
	for index, token := range sentence.Tokens {
		morphology, err := json.Marshal(token.Morphology)
		require.NoError(t, err)
		tokenStart, err := checked.Int64FromUint64(token.Location.StartOffset)
		require.NoError(t, err)
		tokenEnd, err := checked.Int64FromUint64(token.Location.EndOffset)
		require.NoError(t, err)
		_, err = pool.Exec(ctx, `INSERT INTO corpus_tokens(owner_id,language,analysis_run_id,corpus_id,sentence_ordinal,token_ordinal,surface,raw_lemma,canonical_lemma,upos,dependency,head,morphology,start_offset,end_offset,lexical_discriminator,headword) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17)`,
			ownerID, language, runID, corpusID, ordinal, index,
			token.Surface, token.RawLemma, token.CanonicalLemma, token.UPOS, token.Dependency, int64(token.Head), morphology,
			offset+tokenStart, offset+tokenEnd,
			token.LexicalDiscriminator, token.Headword)
		require.NoError(t, err)
	}
	return ordinal + 1, sentenceEnd + 1
}
