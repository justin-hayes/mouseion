//go:build integration

package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/migrations"
)

func TestBookVocabularyStudyBackfillCorrectionMapsHistoricalCampaignStates(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, integrationDatabase(t, ctx))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	owner, err := store.CreateUser(ctx, "migration-owner", false)
	if err != nil {
		t.Fatal(err)
	}
	deck, err := store.PutDeck(ctx, owner.ID, "de", "Migration deck")
	if err != nil {
		t.Fatal(err)
	}

	if _, err = store.Pool().Exec(ctx, `CREATE TABLE learning_campaigns (
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
	)`); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Pool().Exec(ctx, `CREATE TABLE learning_campaign_vocabulary (
		owner_id uuid NOT NULL,
		campaign_id uuid NOT NULL,
		language text NOT NULL,
		canonical_lemma text NOT NULL,
		upos text NOT NULL,
		generated_at timestamptz NOT NULL,
		graduated_at timestamptz
	)`); err != nil {
		t.Fatal(err)
	}
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
		if sourceErr != nil {
			t.Fatal(sourceErr)
		}
		prep, prepErr := store.CreateDeckPreparation(ctx, domain.DeckPreparation{
			OwnerID: owner.ID, SourceMaterialID: source.ID, Filename: want.name + ".apkg",
			DeckName: want.name, ContentHash: source.ContentHash,
		})
		if prepErr != nil {
			t.Fatal(prepErr)
		}
		if _, prepErr = store.ClaimDeckPreparation(ctx, owner.ID, prep.ID); prepErr != nil {
			t.Fatal(prepErr)
		}
		if _, prepErr = store.CompleteDeckPreparation(ctx, owner.ID, prep.ID, domain.DeckPreparation{Artifact: []byte("apkg"), Filename: prep.Filename, DeckName: prep.DeckName, TotalCards: 1}); prepErr != nil {
			t.Fatal(prepErr)
		}
		preparations[want.name] = prep

		lemma := "migration-" + want.name
		if _, prepErr = store.Pool().Exec(ctx, `INSERT INTO generated_vocabulary(owner_id,language,canonical_lemma,upos,first_deck_id,first_source_material_id) VALUES($1,'de',$2,'NOUN',$3,$4)`, owner.ID, lemma, deck.ID, source.ID); prepErr != nil {
			t.Fatal(prepErr)
		}
		if want.name == "complete" {
			if _, prepErr = store.Pool().Exec(ctx, `INSERT INTO deck_preparation_vocabulary(owner_id,deck_preparation_id,language,canonical_lemma,upos,generated_at) VALUES($1,$2,'de',$3,'NOUN','2000-01-01T00:00:00Z')`, owner.ID, prep.ID, lemma); prepErr != nil {
				t.Fatal(prepErr)
			}
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
		if _, prepErr = store.Pool().Exec(ctx, `INSERT INTO learning_campaigns(id,owner_id,deck_preparation_id,status,deck_status,created_at,updated_at,activated_at,deck_reviewed_at,completed_at,abandoned_at,vocabulary_graduated_at) VALUES($1,$2,$3,$4,$5,$6,$6,$7,$8,$9,$10,$11)`, campaignID, owner.ID, prep.ID, want.campaignStatus, want.deckStatus, now, activatedAt, reviewedAt, completedAt, abandonedAt, graduatedAt); prepErr != nil {
			t.Fatal(prepErr)
		}
		if _, prepErr = store.Pool().Exec(ctx, `INSERT INTO learning_campaign_vocabulary(owner_id,campaign_id,language,canonical_lemma,upos,generated_at,graduated_at) VALUES($1,$2,'de',$3,'NOUN',$4,$5)`, owner.ID, campaignID, lemma, now, graduatedAt); prepErr != nil {
			t.Fatal(prepErr)
		}
	}

	correction, err := migrations.FS.ReadFile("000060_book_vocabulary_study_backfill_correction.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Pool().Exec(ctx, string(correction)); err != nil {
		t.Fatalf("execute correction: %v", err)
	}
	if _, err = store.Pool().Exec(ctx, string(correction)); err != nil {
		t.Fatalf("execute correction twice: %v", err)
	}

	for _, want := range states {
		prep := preparations[want.name]
		var studying, reviewed, graduated, released bool
		if err = store.Pool().QueryRow(ctx, `SELECT studying_at IS NOT NULL,reviewed_at IS NOT NULL,graduated_at IS NOT NULL,released_at IS NOT NULL FROM deck_preparations WHERE owner_id=$1 AND id=$2`, owner.ID, prep.ID).Scan(&studying, &reviewed, &graduated, &released); err != nil {
			t.Fatal(err)
		}
		if studying != want.studying || reviewed != want.reviewed || graduated != want.graduated || released != want.released {
			t.Fatalf("%s state: studying=%v reviewed=%v graduated=%v released=%v", want.name, studying, reviewed, graduated, released)
		}
	}

	var studyingCount int
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM deck_preparations WHERE owner_id=$1 AND studying_at IS NOT NULL`, owner.ID).Scan(&studyingCount); err != nil {
		t.Fatal(err)
	}
	if studyingCount != 1 {
		t.Fatalf("owner-wide studying leases=%d", studyingCount)
	}
	var generatedAt time.Time
	if err = store.Pool().QueryRow(ctx, `SELECT generated_at FROM deck_preparation_vocabulary WHERE owner_id=$1 AND deck_preparation_id=$2`, owner.ID, preparations["complete"].ID).Scan(&generatedAt); err != nil {
		t.Fatal(err)
	}
	if !generatedAt.Equal(time.Date(2000, time.January, 1, 0, 0, 0, 0, time.UTC)) {
		t.Fatalf("snapshot provenance was rewritten: %s", generatedAt)
	}
	var knownCount int
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1`, owner.ID).Scan(&knownCount); err != nil {
		t.Fatal(err)
	}
	if knownCount != 0 {
		t.Fatalf("migration created known vocabulary: %d", knownCount)
	}

	drop, err := migrations.FS.ReadFile("000061_drop_learning_campaigns.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Pool().Exec(ctx, string(drop)); err != nil {
		t.Fatalf("execute campaign-table drop: %v", err)
	}
	for _, table := range []string{"learning_campaigns", "learning_campaign_vocabulary"} {
		var count int
		if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM information_schema.tables WHERE table_schema='public' AND table_name=$1`, table).Scan(&count); err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Fatalf("retired table remains: %s", table)
		}
	}
}
