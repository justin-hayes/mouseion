//go:build integration

package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBackfillGermanVocabularyMergesMutableIdentityProjections(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "german-backfill", false)
	require.NoError(t, err)
	deckID := ""
	err = store.Pool().QueryRow(ctx, `INSERT INTO decks(owner_id,language,name) VALUES($1,'de','backfill') RETURNING id::text`, owner.ID).Scan(&deckID)
	require.NoError(t, err)
	source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: "backfill", Title: "Backfill", MediaType: "text/plain", ContentHash: "backfill-hash", Content: []byte("Haß"), FullText: "Haß"})
	require.NoError(t, err)
	preparationID := ""
	err = store.Pool().QueryRow(ctx, `INSERT INTO deck_preparations(owner_id,source_material_id,filename,deck_name,content_hash) VALUES($1,$2,'backfill.apkg','Backfill','backfill-hash') RETURNING id::text`, owner.ID, source.ID).Scan(&preparationID)
	require.NoError(t, err)
	oldTime := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	newTime := oldTime.Add(time.Hour)
	_, err = store.Pool().Exec(ctx, `INSERT INTO known_vocabulary(owner_id,language,canonical_lemma,upos,created_at) VALUES($1,'de','haß','NOUN',$2),($1,'de','hass','NOUN',$3)`, owner.ID, oldTime, newTime)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO vocabulary_states(owner_id,language,canonical_lemma,upos,state,updated_at) VALUES($1,'de','haß','NOUN','known',$2),($1,'de','hass','NOUN','candidate',$3)`, owner.ID, oldTime, newTime)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO generated_vocabulary(owner_id,language,canonical_lemma,upos,first_deck_id,first_generated_at) VALUES($1,'de','haß','NOUN',$2,$3),($1,'de','hass','NOUN',$2,$4)`, owner.ID, deckID, oldTime, newTime)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO deck_preparation_vocabulary(owner_id,deck_preparation_id,language,canonical_lemma,upos,generated_at,graduated_at) VALUES($1,$2,'de','haß','NOUN',$3,NULL),($1,$2,'de','hass','NOUN',$4,$4)`, owner.ID, preparationID, oldTime, newTime)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO selection_candidates(owner_id,corpus_id,language,canonical_lemma,upos,occurrence_count,observed_forms,eligible_sentence_refs,provenance) VALUES($1,'corpus','de','haß','NOUN',2,'["Haß"]','[]','{"min_occurrences":1}'),($1,'corpus','de','hass','NOUN',3,'["Hass"]','[]','{"min_occurrences":1}')`, owner.ID)
	require.NoError(t, err)

	report, err := store.BackfillGermanVocabulary(ctx)
	require.NoError(t, err)
	assert.Empty(t, report.Conflicts)
	assert.Greater(t, report.Updated, 0)
	var count int
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1 AND language='de' AND canonical_lemma='hass' AND upos='NOUN'`, owner.ID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	var state string
	err = store.Pool().QueryRow(ctx, `SELECT state FROM vocabulary_states WHERE owner_id=$1 AND language='de' AND canonical_lemma='hass' AND upos='NOUN'`, owner.ID).Scan(&state)
	require.NoError(t, err)
	assert.Equal(t, "known", state)
	var occurrences int
	err = store.Pool().QueryRow(ctx, `SELECT occurrence_count FROM selection_candidates WHERE owner_id=$1 AND corpus_id='corpus' AND language='de' AND canonical_lemma='hass' AND upos='NOUN'`, owner.ID).Scan(&occurrences)
	require.NoError(t, err)
	assert.Equal(t, 5, occurrences)
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM deck_preparation_vocabulary WHERE owner_id=$1 AND canonical_lemma='hass'`, owner.ID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	var graduated bool
	err = store.Pool().QueryRow(ctx, `SELECT graduated_at IS NOT NULL FROM deck_preparation_vocabulary WHERE owner_id=$1 AND canonical_lemma='hass'`, owner.ID).Scan(&graduated)
	require.NoError(t, err)
	assert.True(t, graduated)
	second, err := store.BackfillGermanVocabulary(ctx)
	require.NoError(t, err)
	assert.Empty(t, second.Conflicts)
	assert.Zero(t, second.Updated)
	assert.Zero(t, second.Merged)
}

func TestBackfillGermanVocabularyReconcilesSelectedExampleSentences(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "german-backfill-sentences", false)
	require.NoError(t, err)
	source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: "sentences", Title: "Sentences", MediaType: "text/plain", ContentHash: "sentences-hash", Content: []byte("Haß"), FullText: "Haß"})
	require.NoError(t, err)
	err = store.PutArtifact(ctx, domain.NormalizedArtifact{ContentHash: "sentences-artifact", Language: "de", SchemaVersion: "1", NormalizationProfile: "test", NormalizationVersion: "1", AnalyzerName: "test", AnalyzerVersion: "1"}, nil)
	require.NoError(t, err)
	corpus, err := store.PutCorpus(ctx, owner.ID, source.ID, "sentences-artifact")
	require.NoError(t, err)
	oldExample, err := store.PutExampleSentence(ctx, owner.ID, corpus.ID, "old", "Haß", []byte(`{"source":"old"}`))
	require.NoError(t, err)
	modernExample, err := store.PutExampleSentence(ctx, owner.ID, corpus.ID, "modern", "Hass", []byte(`{"source":"modern"}`))
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE example_sentences SET language='de',canonical_lemma=$2,upos='NOUN',selection_rank=2,selection_score=3,selection_reasons='["old"]',is_chosen=false WHERE owner_id=$1 AND id=$3`, owner.ID, "haß", oldExample.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE example_sentences SET language='de',canonical_lemma=$2,upos='NOUN',selection_rank=1,selection_score=8,selection_reasons='["modern"]',is_chosen=true WHERE owner_id=$1 AND id=$3`, owner.ID, "hass", modernExample.ID)
	require.NoError(t, err)

	report, err := store.BackfillGermanVocabulary(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, report.Updated)
	assert.Equal(t, 1, report.Merged)
	var rows []struct {
		ID      string
		Lemma   string
		Rank    int
		Score   int
		Reasons []byte
		Chosen  bool
	}
	queryRows, err := store.Pool().Query(ctx, `SELECT id::text, canonical_lemma, selection_rank, selection_score, selection_reasons, is_chosen FROM example_sentences WHERE owner_id=$1 AND corpus_id=$2 ORDER BY selection_rank`, owner.ID, corpus.ID)
	require.NoError(t, err)
	defer queryRows.Close()
	for queryRows.Next() {
		var row struct {
			ID      string
			Lemma   string
			Rank    int
			Score   int
			Reasons []byte
			Chosen  bool
		}
		require.NoError(t, queryRows.Scan(&row.ID, &row.Lemma, &row.Rank, &row.Score, &row.Reasons, &row.Chosen))
		rows = append(rows, row)
	}
	require.NoError(t, queryRows.Err())
	require.Len(t, rows, 2)
	assert.Equal(t, []int{1, 2}, []int{rows[0].Rank, rows[1].Rank})
	assert.Equal(t, []string{"hass", "hass"}, []string{rows[0].Lemma, rows[1].Lemma})
	assert.Equal(t, []int{8, 3}, []int{rows[0].Score, rows[1].Score})
	assert.Equal(t, [][]byte{[]byte(`["modern"]`), []byte(`["old"]`)}, [][]byte{rows[0].Reasons, rows[1].Reasons})
	assert.True(t, rows[0].Chosen)
	assert.False(t, rows[1].Chosen)

	second, err := store.BackfillGermanVocabulary(ctx)
	require.NoError(t, err)
	assert.Zero(t, second.Updated)
	assert.Zero(t, second.Merged)
}

func TestBackfillGermanVocabularyRollsBackCuratedConflict(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "german-backfill-conflict", false)
	require.NoError(t, err)
	source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: "conflict", Title: "Conflict", MediaType: "text/plain", ContentHash: "conflict-hash", Content: []byte("Haß"), FullText: "Haß"})
	require.NoError(t, err)
	err = store.PutArtifact(ctx, domain.NormalizedArtifact{ContentHash: "conflict-artifact", Language: "de", SchemaVersion: "1", NormalizationProfile: "test", NormalizationVersion: "1", AnalyzerName: "test", AnalyzerVersion: "1"}, nil)
	require.NoError(t, err)
	corpus, err := store.PutCorpus(ctx, owner.ID, source.ID, "conflict-artifact")
	require.NoError(t, err)
	oldExample, err := store.PutExampleSentence(ctx, owner.ID, corpus.ID, "old", "Haß", []byte(`{}`))
	require.NoError(t, err)
	newExample, err := store.PutExampleSentence(ctx, owner.ID, corpus.ID, "new", "Hass", []byte(`{}`))
	require.NoError(t, err)
	for _, example := range []struct {
		id, lemma string
	}{
		{oldExample.ID, "haß"}, {newExample.ID, "hass"},
	} {
		_, err = store.Pool().Exec(ctx, `UPDATE example_sentences SET language='de',canonical_lemma=$2,upos='NOUN',selection_rank=1,selection_score=1,selection_reasons='[]',is_chosen=true WHERE owner_id=$1 AND id=$3`, owner.ID, example.lemma, example.id)
		require.NoError(t, err)
	}
	_, err = store.Pool().Exec(ctx, `INSERT INTO curated_sentences(owner_id,example_sentence_id,language,canonical_lemma,upos,notes) VALUES($1,$2,'de','haß','NOUN','old'),($1,$3,'de','hass','NOUN','modern')`, owner.ID, oldExample.ID, newExample.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO known_vocabulary(owner_id,language,canonical_lemma,upos) VALUES($1,'de','haß','NOUN')`, owner.ID)
	require.NoError(t, err)

	report, err := store.BackfillGermanVocabulary(ctx)
	require.NoError(t, err)
	require.Len(t, report.Conflicts, 1)
	assert.Equal(t, "curated_sentences", report.Conflicts[0].Table)
	var count int
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1 AND canonical_lemma='haß'`, owner.ID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count)
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1 AND canonical_lemma='hass'`, owner.ID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}
