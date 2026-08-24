//go:build integration

package analysisinsights

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

func TestCoverageEndToEndOwnerIsolationAndLegacyReanalysis(t *testing.T) {
	ctx := context.Background()
	databaseURL := os.Getenv("MOUSEION_TEST_DATABASE_URL")
	if databaseURL == "" {
		databaseURL = "postgres://postgres@localhost:5432/mouseion_test?sslmode=disable"
	}
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	conn, err := admin.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	if _, err = conn.Exec(ctx, `SELECT pg_advisory_lock(90420199)`); err != nil {
		t.Fatal(err)
	}
	defer conn.Exec(context.Background(), `SELECT pg_advisory_unlock(90420199)`)
	if _, err = conn.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	if err = persistence.Migrate(databaseURL); err != nil {
		t.Fatal(err)
	}
	store, err := persistence.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	alice, _ := store.CreateUser(ctx, "insights-alice", false)
	bob, _ := store.CreateUser(ctx, "insights-bob", false)
	artifact := domain.NormalizedArtifact{ContentHash: "sha256:insights-e2e", Language: "de", SchemaVersion: "1", NormalizationProfile: "test", NormalizationVersion: "1", AnalyzerName: "test", AnalyzerVersion: "1"}
	if err = store.PutArtifact(ctx, artifact, []domain.SharedLemma{
		{CanonicalLemma: "eins", UPOS: "NOUN", Frequency: 70},
		{CanonicalLemma: "zwei", UPOS: "VERB", Frequency: 20},
		{CanonicalLemma: "drei", UPOS: "ADJ", Frequency: 10},
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
	if _, err = service.Coverage(ctx, bob.ID, corpus.ID); err == nil {
		t.Fatal("bob read Alice's analysis insights")
	}
}
