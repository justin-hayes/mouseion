//go:build integration

package persistence_test

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
)

func TestGetCoverageEntryForBookEncodesFirstEncounterAsBigint(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	owner, err := store.CreateUser(ctx, "coverage-entry-owner", false)
	if err != nil {
		t.Fatal(err)
	}
	artifact := domain.NormalizedArtifact{ContentHash: "coverage-entry-hash", Language: "de", SchemaVersion: "1", NormalizationProfile: "test", NormalizationVersion: "1", AnalyzerName: "test", AnalyzerVersion: "1"}
	if err = store.PutArtifact(ctx, artifact, []domain.SharedLemma{{CanonicalLemma: "Haus", UPOS: "NOUN", Morphology: []byte(`{}`), Frequency: 1}}); err != nil {
		t.Fatal(err)
	}
	book, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: "coverage-entry-book", Title: "Coverage Entry Book", MediaType: "text/plain", ContentHash: artifact.ContentHash, Content: []byte("Haus"), FullText: "Haus"})
	if err != nil {
		t.Fatal(err)
	}
	corpus, err := store.PutCorpus(ctx, owner.ID, book.ID, artifact.ContentHash)
	if err != nil {
		t.Fatal(err)
	}
	candidate := domain.SelectionCandidate{OwnerID: owner.ID, CorpusID: corpus.ID, Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN", OccurrenceCount: 1, ObservedForms: []byte(`["Haus"]`), SentenceReferences: []byte(`[]`), Provenance: []byte(`{}`), FirstEncounter: 59}
	if _, err = store.PutSelectionCandidate(ctx, candidate); err != nil {
		t.Fatal(err)
	}

	entry, err := store.GetCoverageEntryForBook(ctx, owner.ID, book.ID, candidate)
	if err != nil {
		t.Fatal(err)
	}
	if entry.FirstEncounter != candidate.FirstEncounter {
		t.Fatalf("first encounter = %d, want %d", entry.FirstEncounter, candidate.FirstEncounter)
	}
}
