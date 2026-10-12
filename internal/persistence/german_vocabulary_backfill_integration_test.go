//go:build integration

package persistence

import (
	"context"
	"encoding/json"
	"fmt"
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
	assert.Positive(t, report.Updated)
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

func TestBackfillGermanVocabularyPreservesEarliestGeneratedProvenance(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "german-backfill-provenance", false)
	require.NoError(t, err)
	deck, err := store.PutDeck(ctx, owner.ID, "de", "Provenance")
	require.NoError(t, err)
	source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: "provenance", Title: "Provenance", MediaType: "text/plain", ContentHash: "provenance-hash", Content: []byte("Haß"), FullText: "Haß"})
	require.NoError(t, err)
	oldTime := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	newTime := oldTime.Add(time.Hour)
	_, err = store.Pool().Exec(ctx, `INSERT INTO generated_vocabulary(owner_id,language,canonical_lemma,upos,first_deck_id,first_source_material_id,first_generated_at) VALUES($1,'de','haß','NOUN',$2,$3,$4),($1,'de','hass','NOUN',$2,NULL,$5)`, owner.ID, deck.ID, source.ID, oldTime, newTime)
	require.NoError(t, err)

	report, err := store.BackfillGermanVocabulary(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1, report.Updated)
	assert.Equal(t, 1, report.Merged)
	var lemma, firstDeckID, firstSourceID string
	var firstGeneratedAt time.Time
	err = store.Pool().QueryRow(ctx, `SELECT canonical_lemma, first_deck_id::text, COALESCE(first_source_material_id::text,''), first_generated_at FROM generated_vocabulary WHERE owner_id=$1 AND language='de'`, owner.ID).Scan(&lemma, &firstDeckID, &firstSourceID, &firstGeneratedAt)
	require.NoError(t, err)
	assert.Equal(t, "hass", lemma)
	assert.Equal(t, deck.ID, firstDeckID)
	assert.Equal(t, source.ID, firstSourceID)
	assert.True(t, firstGeneratedAt.Equal(oldTime))
}

func TestBackfillGermanVocabularyBreaksGeneratedIdentityTiesDeterministically(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	tie := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)

	t.Run("smallest first deck wins", func(t *testing.T) {
		owner, err := store.CreateUser(ctx, "german-backfill-deck-tie", false)
		require.NoError(t, err)
		first, err := store.PutDeck(ctx, owner.ID, "de", "First")
		require.NoError(t, err)
		second, err := store.PutDeck(ctx, owner.ID, "de", "Second")
		require.NoError(t, err)
		_, err = store.Pool().Exec(ctx, `INSERT INTO generated_vocabulary(owner_id,language,canonical_lemma,upos,first_deck_id,first_generated_at) VALUES($1,'de','haß','NOUN',$2,$3),($1,'de','hass','NOUN',$4,$3)`, owner.ID, first.ID, tie, second.ID)
		require.NoError(t, err)

		report, err := store.BackfillGermanVocabulary(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, report.Updated)
		var winner string
		err = store.Pool().QueryRow(ctx, `SELECT first_deck_id::text FROM generated_vocabulary WHERE owner_id=$1 AND language='de' AND canonical_lemma='hass'`, owner.ID).Scan(&winner)
		require.NoError(t, err)
		assert.Equal(t, min(first.ID, second.ID), winner)
	})

	t.Run("smallest first source wins", func(t *testing.T) {
		owner, err := store.CreateUser(ctx, "german-backfill-source-tie", false)
		require.NoError(t, err)
		deck, err := store.PutDeck(ctx, owner.ID, "de", "Tie")
		require.NoError(t, err)
		first, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: "first", Title: "First", MediaType: "text/plain", ContentHash: "first-hash", Content: []byte("Haß"), FullText: "Haß"})
		require.NoError(t, err)
		second, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: "second", Title: "Second", MediaType: "text/plain", ContentHash: "second-hash", Content: []byte("Hass"), FullText: "Hass"})
		require.NoError(t, err)
		_, err = store.Pool().Exec(ctx, `INSERT INTO generated_vocabulary(owner_id,language,canonical_lemma,upos,first_deck_id,first_source_material_id,first_generated_at) VALUES($1,'de','haß','NOUN',$2,$3,$4),($1,'de','hass','NOUN',$2,$5,$4)`, owner.ID, deck.ID, first.ID, tie, second.ID)
		require.NoError(t, err)

		report, err := store.BackfillGermanVocabulary(ctx)
		require.NoError(t, err)
		assert.Equal(t, 1, report.Updated)
		var winner string
		err = store.Pool().QueryRow(ctx, `SELECT first_source_material_id::text FROM generated_vocabulary WHERE owner_id=$1 AND language='de' AND canonical_lemma='hass'`, owner.ID).Scan(&winner)
		require.NoError(t, err)
		assert.Equal(t, min(first.ID, second.ID), winner)
	})
}

// backfillTestID returns a deterministic, table-unique uuid for a case and row
// rank, so a backfill test can control id tie-breaks without collisions.
func backfillTestID(caseIndex, rank int) string {
	return fmt.Sprintf("%08x-0000-0000-0000-%012x", caseIndex+1, rank)
}

func TestBackfillGermanVocabularyChoosesKnownIdentityWinner(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	created := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	later := created.Add(time.Hour)

	cases := []struct {
		name    string
		entries []struct {
			rank      int
			lemma     string
			createdAt time.Time
		}
		wantWinnerRank int
		wantUpdated    int
	}{
		{
			name: "earliest creation wins over a later spelling",
			entries: []struct {
				rank      int
				lemma     string
				createdAt time.Time
			}{
				{1, "hass", created},
				{2, "haß", later},
			},
			wantWinnerRank: 1,
			wantUpdated:    0,
		},
		{
			name: "smallest id breaks an equal creation tie",
			entries: []struct {
				rank      int
				lemma     string
				createdAt time.Time
			}{
				{1, "haß", created},
				{2, "hass", created},
			},
			wantWinnerRank: 1,
			wantUpdated:    1,
		},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			owner, err := store.CreateUser(ctx, fmt.Sprintf("german-known-winner-%d", i), false)
			require.NoError(t, err)
			for _, entry := range tc.entries {
				_, err = store.Pool().Exec(ctx, `INSERT INTO known_vocabulary(id,owner_id,language,canonical_lemma,upos,created_at) VALUES($1,$2,'de',$3,'NOUN',$4)`, backfillTestID(i, entry.rank), owner.ID, entry.lemma, entry.createdAt)
				require.NoError(t, err)
			}

			report, err := store.BackfillGermanVocabulary(ctx)
			require.NoError(t, err)
			assert.Empty(t, report.Conflicts)
			assert.Equal(t, tc.wantUpdated, report.Updated)

			var count int
			err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1 AND language='de' AND upos='NOUN'`, owner.ID).Scan(&count)
			require.NoError(t, err)
			assert.Equal(t, 1, count)
			var winner, lemma string
			err = store.Pool().QueryRow(ctx, `SELECT id::text, canonical_lemma FROM known_vocabulary WHERE owner_id=$1 AND language='de' AND upos='NOUN'`, owner.ID).Scan(&winner, &lemma)
			require.NoError(t, err)
			assert.Equal(t, backfillTestID(i, tc.wantWinnerRank), winner)
			assert.Equal(t, "hass", lemma)
		})
	}
}

func TestBackfillGermanVocabularyChoosesVocabularyStateWinner(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	created := time.Date(2026, 5, 1, 0, 0, 0, 0, time.UTC)
	later := created.Add(time.Hour)

	cases := []struct {
		name    string
		entries []struct {
			rank      int
			lemma     string
			state     string
			updatedAt time.Time
		}
		wantWinnerRank int
		wantState      string
		wantUpdated    int
	}{
		{
			name: "higher state precedence wins over recency",
			entries: []struct {
				rank      int
				lemma     string
				state     string
				updatedAt time.Time
			}{
				{1, "hass", "candidate", later},
				{2, "haß", "known", created},
			},
			wantWinnerRank: 2,
			wantState:      "known",
			wantUpdated:    1,
		},
		{
			name: "latest update wins at equal precedence",
			entries: []struct {
				rank      int
				lemma     string
				state     string
				updatedAt time.Time
			}{
				{1, "haß", "candidate", created},
				{2, "hass", "candidate", later},
			},
			wantWinnerRank: 2,
			wantState:      "candidate",
			wantUpdated:    0,
		},
		{
			name: "smallest id breaks an equal precedence and update tie",
			entries: []struct {
				rank      int
				lemma     string
				state     string
				updatedAt time.Time
			}{
				{1, "haß", "candidate", created},
				{2, "hass", "candidate", created},
			},
			wantWinnerRank: 1,
			wantState:      "candidate",
			wantUpdated:    1,
		},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			owner, err := store.CreateUser(ctx, fmt.Sprintf("german-state-winner-%d", i), false)
			require.NoError(t, err)
			for _, entry := range tc.entries {
				_, err = store.Pool().Exec(ctx, `INSERT INTO vocabulary_states(id,owner_id,language,canonical_lemma,upos,state,updated_at) VALUES($1,$2,'de',$3,'NOUN',$4,$5)`, backfillTestID(i, entry.rank), owner.ID, entry.lemma, entry.state, entry.updatedAt)
				require.NoError(t, err)
			}

			report, err := store.BackfillGermanVocabulary(ctx)
			require.NoError(t, err)
			assert.Empty(t, report.Conflicts)
			assert.Equal(t, tc.wantUpdated, report.Updated)

			var count int
			err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM vocabulary_states WHERE owner_id=$1 AND language='de' AND upos='NOUN'`, owner.ID).Scan(&count)
			require.NoError(t, err)
			assert.Equal(t, 1, count)
			var winner, lemma, state string
			err = store.Pool().QueryRow(ctx, `SELECT id::text, canonical_lemma, state FROM vocabulary_states WHERE owner_id=$1 AND language='de' AND upos='NOUN'`, owner.ID).Scan(&winner, &lemma, &state)
			require.NoError(t, err)
			assert.Equal(t, backfillTestID(i, tc.wantWinnerRank), winner)
			assert.Equal(t, "hass", lemma)
			assert.Equal(t, tc.wantState, state)
		})
	}
}

func TestBackfillGermanVocabularyMergesSelectionCandidatesPerCorpus(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "german-selection-corpus", false)
	require.NoError(t, err)
	first := time.Date(2026, 6, 1, 0, 0, 0, 0, time.UTC)
	second := first.Add(time.Hour)
	_, err = store.Pool().Exec(ctx, `INSERT INTO selection_candidates(owner_id,corpus_id,language,canonical_lemma,upos,occurrence_count,observed_forms,eligible_sentence_refs,provenance,selected_at) VALUES
		($1,'alpha','de','haß','NOUN',2,'["Haß"]','["a1"]','{"min_occurrences":2}',$2),
		($1,'alpha','de','hass','NOUN',3,'["Hass"]','["a2"]','{"min_occurrences":1}',$3),
		($1,'beta','de','haß','NOUN',5,'["Haß"]','["b1"]','{"min_occurrences":3}',$2)`, owner.ID, first, second)
	require.NoError(t, err)

	report, err := store.BackfillGermanVocabulary(ctx)
	require.NoError(t, err)
	assert.Empty(t, report.Conflicts)
	assert.Equal(t, 2, report.Updated)
	assert.Equal(t, 1, report.Merged)

	cases := []struct {
		corpus          string
		wantOccurrences int
		wantForms       []string
		wantReasons     []string
		wantSelectedAt  time.Time
	}{
		{corpus: "alpha", wantOccurrences: 5, wantForms: []string{"Haß", "Hass"}, wantReasons: []string{"a1", "a2"}, wantSelectedAt: second},
		{corpus: "beta", wantOccurrences: 5, wantForms: []string{"Haß"}, wantReasons: []string{"b1"}, wantSelectedAt: first},
	}
	for _, tc := range cases {
		t.Run(tc.corpus, func(t *testing.T) {
			var count int
			err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM selection_candidates WHERE owner_id=$1 AND corpus_id=$2 AND language='de' AND canonical_lemma='hass' AND upos='NOUN'`, owner.ID, tc.corpus).Scan(&count)
			require.NoError(t, err)
			assert.Equal(t, 1, count)
			var occurrences int
			var forms, reasons []byte
			var selectedAt time.Time
			err = store.Pool().QueryRow(ctx, `SELECT occurrence_count, observed_forms, eligible_sentence_refs, selected_at FROM selection_candidates WHERE owner_id=$1 AND corpus_id=$2 AND language='de' AND canonical_lemma='hass' AND upos='NOUN'`, owner.ID, tc.corpus).Scan(&occurrences, &forms, &reasons, &selectedAt)
			require.NoError(t, err)
			assert.Equal(t, tc.wantOccurrences, occurrences)
			assert.True(t, selectedAt.Equal(tc.wantSelectedAt))
			var gotForms, gotReasons []string
			require.NoError(t, json.Unmarshal(forms, &gotForms))
			require.NoError(t, json.Unmarshal(reasons, &gotReasons))
			assert.ElementsMatch(t, tc.wantForms, gotForms)
			assert.ElementsMatch(t, tc.wantReasons, gotReasons)
		})
	}
}
