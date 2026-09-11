//go:build integration

package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/migrations"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBookVocabularyStudyBackfillCorrectionMapsHistoricalCampaignStates(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, integrationDatabase(t, ctx))
	require.NoError(t, err)
	defer store.Close()

	owner, err := store.CreateUser(ctx, "migration-owner", false)
	require.NoError(t, err)
	deck, err := store.PutDeck(ctx, owner.ID, "de", "Migration deck")
	require.NoError(t, err)

	_, err = store.Pool().Exec(ctx, `CREATE TABLE learning_campaigns (
		id uuid PRIMARY KEY,
		owner_id uuid NOT NULL,
		deck_preparation_id uuid NOT NULL,
		status text NOT NULL,
		deck_status text NOT NULL,
		created_at timestamptz NOT NULL,
		updated_at timestamptz NOT NULL,
		activated_at timestamptz,
		deck_reviewed_at timestamptz,
		completed_at timestamptz,
		abandoned_at timestamptz,
		vocabulary_graduated_at timestamptz
	)`)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `CREATE TABLE learning_campaign_vocabulary (
		owner_id uuid NOT NULL,
		campaign_id uuid NOT NULL,
		language text NOT NULL,
		canonical_lemma text NOT NULL,
		upos text NOT NULL,
		generated_at timestamptz NOT NULL,
		graduated_at timestamptz
	)`)
	require.NoError(t, err)
	defer func() {
		_, _ = store.Pool().Exec(ctx, `DROP TABLE IF EXISTS learning_campaign_vocabulary`)
		_, _ = store.Pool().Exec(ctx, `DROP TABLE IF EXISTS learning_campaigns`)
	}()

	type state struct {
		name, campaignStatus, deckStatus        string
		studying, reviewed, graduated, released bool
	}
	states := []state{
		{name: "complete", campaignStatus: "complete", deckStatus: "reviewed", reviewed: true, graduated: true},
		{name: "active-studying", campaignStatus: "active", deckStatus: "studying", studying: true},
		{name: "active-reviewed", campaignStatus: "active", deckStatus: "reviewed", reviewed: true, graduated: true},
		{name: "queued", campaignStatus: "queued", deckStatus: "queued", released: true},
		{name: "abandoned", campaignStatus: "abandoned", deckStatus: "abandoned", released: true},
	}
	preparations := make(map[string]domain.DeckPreparation, len(states))
	for _, want := range states {
		source, sourceErr := store.PutSourceMaterial(ctx, domain.SourceMaterial{
			OwnerID: owner.ID, Language: "de", SourceIdentifier: "migration-" + want.name,
			Title: want.name, MediaType: "text/plain", ContentHash: "migration-" + want.name,
			Content: []byte(want.name), FullText: want.name,
		})
		require.NoError(t, sourceErr)
		prep, prepErr := store.CreateDeckPreparation(ctx, domain.DeckPreparation{
			OwnerID: owner.ID, SourceMaterialID: source.ID, Filename: want.name + ".apkg",
			DeckName: want.name, ContentHash: source.ContentHash,
		})
		require.NoError(t, prepErr)
		_, prepErr = store.ClaimDeckPreparation(ctx, owner.ID, prep.ID)
		require.NoError(t, prepErr)
		_, prepErr = store.CompleteDeckPreparation(ctx, owner.ID, prep.ID, domain.DeckPreparation{Artifact: []byte("apkg"), Filename: prep.Filename, DeckName: prep.DeckName, TotalCards: 1})
		require.NoError(t, prepErr)
		preparations[want.name] = prep

		lemma := "migration-" + want.name
		_, prepErr = store.Pool().Exec(ctx, `INSERT INTO generated_vocabulary(owner_id,language,canonical_lemma,upos,first_deck_id,first_source_material_id) VALUES($1,'de',$2,'NOUN',$3,$4)`, owner.ID, lemma, deck.ID, source.ID)
		require.NoError(t, prepErr)
		if want.name == "complete" {
			_, prepErr = store.Pool().Exec(ctx, `INSERT INTO deck_preparation_vocabulary(owner_id,deck_preparation_id,language,canonical_lemma,upos,generated_at) VALUES($1,$2,'de',$3,'NOUN','2000-01-01T00:00:00Z')`, owner.ID, prep.ID, lemma)
			require.NoError(t, prepErr)
		}

		campaignID := uuid.NewString()
		now := time.Now().UTC()
		var activatedAt, reviewedAt, completedAt, abandonedAt, graduatedAt any
		if want.campaignStatus == "active" {
			activatedAt = now
		}
		if want.deckStatus == "reviewed" {
			reviewedAt = now
		}
		if want.campaignStatus == "complete" {
			completedAt = now
			graduatedAt = now
		}
		if want.campaignStatus == "abandoned" {
			abandonedAt = now
		}
		_, prepErr = store.Pool().Exec(ctx, `INSERT INTO learning_campaigns(id,owner_id,deck_preparation_id,status,deck_status,created_at,updated_at,activated_at,deck_reviewed_at,completed_at,abandoned_at,vocabulary_graduated_at) VALUES($1,$2,$3,$4,$5,$6,$6,$7,$8,$9,$10,$11)`, campaignID, owner.ID, prep.ID, want.campaignStatus, want.deckStatus, now, activatedAt, reviewedAt, completedAt, abandonedAt, graduatedAt)
		require.NoError(t, prepErr)
		_, prepErr = store.Pool().Exec(ctx, `INSERT INTO learning_campaign_vocabulary(owner_id,campaign_id,language,canonical_lemma,upos,generated_at,graduated_at) VALUES($1,$2,'de',$3,'NOUN',$4,$5)`, owner.ID, campaignID, lemma, now, graduatedAt)
		require.NoError(t, prepErr)
	}

	correction, err := migrations.FS.ReadFile("000060_book_vocabulary_study_backfill_correction.up.sql")
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, string(correction))
	require.NoError(t, err, "execute correction")
	_, err = store.Pool().Exec(ctx, string(correction))
	require.NoError(t, err, "execute correction twice")

	for _, want := range states {
		prep := preparations[want.name]
		var studying, reviewed, graduated, released bool
		err = store.Pool().QueryRow(ctx, `SELECT studying_at IS NOT NULL,reviewed_at IS NOT NULL,graduated_at IS NOT NULL,released_at IS NOT NULL FROM deck_preparations WHERE owner_id=$1 AND id=$2`, owner.ID, prep.ID).Scan(&studying, &reviewed, &graduated, &released)
		require.NoError(t, err)
		assert.Equal(t, want.studying, studying, "%s state", want.name)
		assert.Equal(t, want.reviewed, reviewed, "%s state", want.name)
		assert.Equal(t, want.graduated, graduated, "%s state", want.name)
		assert.Equal(t, want.released, released, "%s state", want.name)
	}

	var studyingCount int
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM deck_preparations WHERE owner_id=$1 AND studying_at IS NOT NULL`, owner.ID).Scan(&studyingCount)
	require.NoError(t, err)
	assert.Equal(t, 1, studyingCount, "owner-wide studying leases")
	var generatedAt time.Time
	err = store.Pool().QueryRow(ctx, `SELECT generated_at FROM deck_preparation_vocabulary WHERE owner_id=$1 AND deck_preparation_id=$2`, owner.ID, preparations["complete"].ID).Scan(&generatedAt)
	require.NoError(t, err)
	assert.True(t, generatedAt.Equal(time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)), "snapshot provenance was rewritten: %s", generatedAt)
	var knownCount int
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1`, owner.ID).Scan(&knownCount)
	require.NoError(t, err)
	assert.Zero(t, knownCount, "migration created known vocabulary")

	drop, err := migrations.FS.ReadFile("000061_drop_learning_campaigns.up.sql")
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, string(drop))
	require.NoError(t, err, "execute campaign-table drop")
	for _, table := range []string{"learning_campaigns", "learning_campaign_vocabulary"} {
		var count int
		err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name=$1`, table).Scan(&count)
		require.NoError(t, err)
		assert.Zero(t, count, "retired table remains: %s", table)
	}
}
