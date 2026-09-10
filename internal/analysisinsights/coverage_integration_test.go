//go:build integration

package analysisinsights

import (
	"context"
	"errors"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
)

func TestCoverageEndToEndOwnerIsolationAndLegacyReanalysis(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	alice, _ := store.CreateUser(ctx, "insights-alice", false)
	bob, _ := store.CreateUser(ctx, "insights-bob", false)
	artifact := domain.NormalizedArtifact{ContentHash: "sha256:insights-e2e", Language: "de", SchemaVersion: "1", NormalizationProfile: "test", NormalizationVersion: "1", AnalyzerName: "test", AnalyzerVersion: "1"}
	if err = store.PutArtifact(ctx, artifact, []domain.SharedLemma{
		{CanonicalLemma: "eins", UPOS: "NOUN", Morphology: []byte(`{}`), Frequency: 70},
		{CanonicalLemma: "zwei", UPOS: "VERB", Morphology: []byte(`{}`), Frequency: 20},
		{CanonicalLemma: "drei", UPOS: "ADJ", Morphology: []byte(`{}`), Frequency: 10},
	}); err != nil {
		t.Fatal(err)
	}
	source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: alice.ID, Language: "de", SourceIdentifier: "insights-e2e", Title: "Insights", MediaType: "text/plain", ContentHash: artifact.ContentHash, Content: []byte("eins zwei drei"), FullText: "eins zwei drei"})
	if err != nil {
		t.Fatal(err)
	}
	corpus, err := store.PutCorpus(ctx, alice.ID, source.ID, artifact.ContentHash)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(store)
	if _, err = service.Coverage(ctx, alice.ID, corpus.ID); !errors.Is(err, ErrStatisticsUnavailable) {
		t.Fatalf("legacy coverage error = %v", err)
	}
	if _, err = store.PutKnownVocabulary(ctx, alice.ID, "de", "eins", "NOUN"); err != nil {
		t.Fatal(err)
	}
	if _, err = store.PutKnownVocabulary(ctx, bob.ID, "de", "zwei", "VERB"); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Pool().Exec(ctx, `UPDATE corpora SET analyzable_token_count=100,distinct_lemma_count=3,sentence_count=4,normalized_token_count=120,empty_sentence_count=1,median_sentence_token_count=30,p90_sentence_token_count=40,long_sentence_count=1 WHERE id=$1`, corpus.ID); err != nil {
		t.Fatal(err)
	}

	got, err := service.Coverage(ctx, alice.ID, corpus.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.KnownTokenCount != 70 || got.UnknownTokenCount != 30 || got.KnownLemmaCount != 1 || got.UnknownLemmaCount != 2 {
		t.Fatalf("owner-scoped coverage = %+v", got)
	}
	if got.TextProfile == nil || got.TextProfile.NormalizedTokenCount != 120 || got.TextProfile.EmptySentenceCount != 1 {
		t.Fatalf("text profile = %+v", got.TextProfile)
	}
	if len(got.Thresholds) != 3 || got.Thresholds[0].LemmaCount != 2 || got.Thresholds[1].OccurrenceCount != 30 || got.Thresholds[2].LemmaCount != 2 {
		t.Fatalf("thresholds = %+v", got.Thresholds)
	}
	if len(got.TopUnknownLemmas) != 2 || got.TopUnknownLemmas[0].CanonicalLemma != "zwei" || got.UnknownConcentration.OccurrenceCount != 30 {
		t.Fatalf("unknown profile = top %+v, concentration %+v", got.TopUnknownLemmas, got.UnknownConcentration)
	}
	if len(got.Projections) != 3 || got.Projections[0].ProjectedTokenCount != 100 {
		t.Fatalf("projections = %+v", got.Projections)
	}
	var deckID string
	if err = store.Pool().QueryRow(ctx, `INSERT INTO decks(owner_id,language,name) VALUES($1,'de','insights-study') RETURNING id`, alice.ID).Scan(&deckID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Pool().Exec(ctx, `INSERT INTO generated_vocabulary(owner_id,language,canonical_lemma,upos,first_deck_id,first_source_material_id) VALUES($1,'de','zwei','VERB',$2,$3)`, alice.ID, deckID, source.ID); err != nil {
		t.Fatal(err)
	}
	preparation, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: alice.ID, SourceMaterialID: source.ID, Filename: "study.apkg", DeckName: "Study", ContentHash: "study-hash"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.ClaimDeckPreparation(ctx, alice.ID, preparation.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.CompleteDeckPreparation(ctx, alice.ID, preparation.ID, domain.DeckPreparation{Artifact: []byte("study"), Filename: "study.apkg", DeckName: "Study", TotalCards: 1}); err != nil {
		t.Fatal(err)
	}
	if _, err = store.StartDeckVocabularyStudy(ctx, alice.ID, preparation.ID); err != nil {
		t.Fatal(err)
	}
	reservedCoverage, err := service.Coverage(ctx, alice.ID, corpus.ID)
	if err != nil {
		t.Fatal(err)
	}
	if reservedCoverage.KnownTokenCount != 70 || reservedCoverage.ReservedTokenCount != 20 || reservedCoverage.ReservedLemmaCount != 1 {
		t.Fatalf("coverage while deck is reserved=%+v", reservedCoverage)
	}
	if _, err = store.ConfirmDeckVocabularyReview(ctx, alice.ID, preparation.ID); err != nil {
		t.Fatal(err)
	}
	graduatedCoverage, err := service.Coverage(ctx, alice.ID, corpus.ID)
	if err != nil {
		t.Fatal(err)
	}
	if graduatedCoverage.KnownTokenCount != 90 || graduatedCoverage.UnknownTokenCount != 10 {
		t.Fatalf("coverage after reviewed deck=%+v", graduatedCoverage)
	}
	if _, err = service.Coverage(ctx, bob.ID, corpus.ID); err == nil {
		t.Fatal("bob read Alice's analysis insights")
	}
}
