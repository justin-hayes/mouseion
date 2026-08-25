//go:build integration

package persistence

import (
	"context"
	"errors"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
)

func TestLearningCampaignLifecycleAndCompatibility(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, integrationDatabase(t, ctx))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	alice, _ := store.CreateUser(ctx, "campaign-alice", false)
	bob, _ := store.CreateUser(ctx, "campaign-bob", false)

	source, prep := readyCampaignFixture(t, ctx, store, alice.ID, "one")
	deck, err := store.PutDeck(ctx, alice.ID, "de", "Campaign one")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.RecordGeneratedVocabulary(ctx, domain.GeneratedVocabulary{OwnerID: alice.ID, Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN", FirstDeckID: deck.ID, FirstSourceMaterialID: &source.ID}); err != nil {
		t.Fatal(err)
	}
	// A legacy row without source provenance must remain unrelated and unknown.
	if _, err = store.RecordGeneratedVocabulary(ctx, domain.GeneratedVocabulary{OwnerID: alice.ID, Language: "de", CanonicalLemma: "Alt", UPOS: "ADJ", FirstDeckID: deck.ID}); err != nil {
		t.Fatal(err)
	}
	campaign, err := store.CreateLearningCampaign(ctx, alice.ID, source.ID, prep.ID)
	if err != nil || campaign.Status != domain.CampaignQueued {
		t.Fatalf("create campaign = %+v, %v", campaign, err)
	}
	if _, err = store.GetLearningCampaign(ctx, bob.ID, campaign.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("owner isolation error = %v", err)
	}
	var known int
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1`, alice.ID).Scan(&known); err != nil || known != 0 {
		t.Fatalf("campaign creation promoted generated history: count=%d err=%v", known, err)
	}

	campaign, err = store.UpdateLearningCampaignProgress(ctx, alice.ID, campaign.ID, domain.BookReading, domain.DeckQueued)
	if err != nil || campaign.Status != domain.CampaignActive || campaign.ActivatedAt == nil || campaign.BookFinishedAt != nil || campaign.DeckReviewedAt != nil {
		t.Fatalf("activate campaign = %+v, %v", campaign, err)
	}

	source2, prep2 := readyCampaignFixture(t, ctx, store, alice.ID, "two")
	queued2, err := store.CreateLearningCampaign(ctx, alice.ID, source2.ID, prep2.ID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.UpdateLearningCampaignProgress(ctx, alice.ID, queued2.ID, domain.BookQueued, domain.DeckStudying); !errors.Is(err, ErrActiveCampaign) {
		t.Fatalf("second active campaign error = %v", err)
	}

	campaign, err = store.UpdateLearningCampaignProgress(ctx, alice.ID, campaign.ID, domain.BookFinished, domain.DeckStudying)
	if err != nil || campaign.Status != domain.CampaignActive || campaign.BookFinishedAt == nil || campaign.DeckReviewedAt != nil {
		t.Fatalf("independent progress = %+v, %v", campaign, err)
	}
	campaign, err = store.UpdateLearningCampaignProgress(ctx, alice.ID, campaign.ID, domain.BookFinished, domain.DeckReviewed)
	if err != nil || campaign.Status != domain.CampaignComplete || campaign.CompletedAt == nil || campaign.DeckReviewedAt == nil || campaign.VocabularyGraduatedAt == nil {
		t.Fatalf("complete campaign = %+v, %v", campaign, err)
	}
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1`, alice.ID).Scan(&known); err != nil || known != 1 {
		t.Fatalf("graduated vocabulary count=%d err=%v", known, err)
	}
	completedAt, graduatedAt := *campaign.CompletedAt, *campaign.VocabularyGraduatedAt
	campaign, err = store.UpdateLearningCampaignProgress(ctx, alice.ID, campaign.ID, domain.BookFinished, domain.DeckReviewed)
	if err != nil || campaign.CompletedAt == nil || !campaign.CompletedAt.Equal(completedAt) || campaign.VocabularyGraduatedAt == nil || !campaign.VocabularyGraduatedAt.Equal(graduatedAt) {
		t.Fatalf("idempotent completion = %+v, %v", campaign, err)
	}
	var generated, graduated int
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM generated_vocabulary WHERE owner_id=$1 AND canonical_lemma='Haus'`, alice.ID).Scan(&generated); err != nil || generated != 1 {
		t.Fatalf("generated history count=%d err=%v", generated, err)
	}
	if err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM learning_campaign_vocabulary WHERE owner_id=$1 AND campaign_id=$2 AND canonical_lemma='Haus' AND graduated_at IS NOT NULL`, alice.ID, campaign.ID).Scan(&graduated); err != nil || graduated != 1 {
		t.Fatalf("graduation provenance count=%d err=%v", graduated, err)
	}
	var legacyKnown bool
	if err = store.Pool().QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM known_vocabulary WHERE owner_id=$1 AND canonical_lemma='Alt')`, alice.ID).Scan(&legacyKnown); err != nil || legacyKnown {
		t.Fatalf("legacy generated row was promoted: known=%v err=%v", legacyKnown, err)
	}
	if _, err = store.UpdateLearningCampaignProgress(ctx, alice.ID, campaign.ID, domain.BookReading, domain.DeckReviewed); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("regressive transition error = %v", err)
	}

	queued2, err = store.UpdateLearningCampaignProgress(ctx, alice.ID, queued2.ID, domain.BookReading, domain.DeckQueued)
	if err != nil || queued2.Status != domain.CampaignActive {
		t.Fatalf("activate next campaign = %+v, %v", queued2, err)
	}
}

func TestLearningCampaignRequiresMatchingReadyPreparation(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, integrationDatabase(t, ctx))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	owner, _ := store.CreateUser(ctx, "campaign-validation", false)
	source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: "pending", Title: "Pending", MediaType: "text/plain", ContentHash: "pending", Content: []byte("pending"), FullText: "pending"})
	if err != nil {
		t.Fatal(err)
	}
	prep, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: source.ID, Filename: "pending.apkg", DeckName: "Pending", ContentHash: source.ContentHash})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreateLearningCampaign(ctx, owner.ID, source.ID, prep.ID); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("pending preparation error = %v", err)
	}
}

func TestListUnassignedReadyDeckPreparationsIsOwnerScoped(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, integrationDatabase(t, ctx))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	alice, _ := store.CreateUser(ctx, "campaign-ready-alice", false)
	bob, _ := store.CreateUser(ctx, "campaign-ready-bob", false)
	aliceSource, aliceDeck := readyCampaignFixture(t, ctx, store, alice.ID, "available")
	_, assignedDeck := readyCampaignFixture(t, ctx, store, alice.ID, "assigned")
	_, _ = readyCampaignFixture(t, ctx, store, bob.ID, "private")
	if _, err = store.CreateLearningCampaign(ctx, alice.ID, assignedDeck.SourceMaterialID, assignedDeck.ID); err != nil {
		t.Fatal(err)
	}

	ready, err := store.ListUnassignedReadyDeckPreparations(ctx, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(ready) != 1 || ready[0].ID != aliceDeck.ID || ready[0].SourceMaterialID != aliceSource.ID {
		t.Fatalf("alice unassigned ready decks = %+v", ready)
	}
	if _, err = store.CreateLearningCampaign(ctx, alice.ID, aliceSource.ID, aliceDeck.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreateLearningCampaign(ctx, alice.ID, aliceSource.ID, aliceDeck.ID); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("duplicate campaign error = %v", err)
	}
	ready, err = store.ListUnassignedReadyDeckPreparations(ctx, alice.ID)
	if err != nil || len(ready) != 0 {
		t.Fatalf("ready decks after assignment = %+v, %v", ready, err)
	}
}

func TestAbandonedCampaignDoesNotGraduateVocabulary(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, integrationDatabase(t, ctx))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	owner, _ := store.CreateUser(ctx, "campaign-abandoned", false)
	source, prep := readyCampaignFixture(t, ctx, store, owner.ID, "abandoned")
	deck, _ := store.PutDeck(ctx, owner.ID, "de", "Abandoned")
	if _, err = store.RecordGeneratedVocabulary(ctx, domain.GeneratedVocabulary{OwnerID: owner.ID, Language: "de", CanonicalLemma: "Frei", UPOS: "ADJ", FirstDeckID: deck.ID, FirstSourceMaterialID: &source.ID}); err != nil {
		t.Fatal(err)
	}
	campaign, err := store.CreateLearningCampaign(ctx, owner.ID, source.ID, prep.ID)
	if err != nil {
		t.Fatal(err)
	}
	campaign, err = store.UpdateLearningCampaignProgress(ctx, owner.ID, campaign.ID, domain.BookAbandoned, domain.DeckQueued)
	if err != nil || campaign.Status != domain.CampaignAbandoned || campaign.AbandonedAt == nil || campaign.VocabularyGraduatedAt != nil {
		t.Fatalf("abandon campaign = %+v, %v", campaign, err)
	}
	var known bool
	if err = store.Pool().QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM known_vocabulary WHERE owner_id=$1 AND canonical_lemma='Frei')`, owner.ID).Scan(&known); err != nil || known {
		t.Fatalf("abandoned vocabulary graduated: known=%v err=%v", known, err)
	}
	if _, err = store.UpdateLearningCampaignProgress(ctx, owner.ID, campaign.ID, domain.BookAbandoned, domain.DeckStudying); !errors.Is(err, ErrInvalidTransition) {
		t.Fatalf("abandoned campaign transition error = %v", err)
	}
}

func readyCampaignFixture(t *testing.T, ctx context.Context, store *PostgresStore, owner, suffix string) (domain.SourceMaterial, domain.DeckPreparation) {
	t.Helper()
	source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: owner, Language: "de", SourceIdentifier: "campaign-" + suffix, Title: "Book " + suffix, MediaType: "text/plain", ContentHash: "campaign-" + suffix, Content: []byte(suffix), FullText: suffix})
	if err != nil {
		t.Fatal(err)
	}
	prep, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner, SourceMaterialID: source.ID, Filename: suffix + ".apkg", DeckName: "Deck " + suffix, ContentHash: source.ContentHash})
	if err != nil {
		t.Fatal(err)
	}
	if prep, err = store.ClaimDeckPreparation(ctx, owner, prep.ID); err != nil {
		t.Fatal(err)
	}
	prep, err = store.CompleteDeckPreparation(ctx, owner, prep.ID, domain.DeckPreparation{Artifact: []byte("apkg-" + suffix), Filename: suffix + ".apkg", DeckName: "Deck " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	return source, prep
}
