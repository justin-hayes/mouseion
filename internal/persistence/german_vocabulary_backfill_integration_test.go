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
	store, err := Open(ctx, integrationDatabase(t, ctx))
	require.NoError(t, err)
	defer store.Close()
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
	_, err = store.Pool().Exec(ctx, `INSERT INTO deck_preparation_vocabulary(owner_id,deck_preparation_id,language,canonical_lemma,upos,generated_at) VALUES($1,$2,'de','haß','NOUN',$3),($1,$2,'de','hass','NOUN',$4)`, owner.ID, preparationID, oldTime, newTime)
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
}
