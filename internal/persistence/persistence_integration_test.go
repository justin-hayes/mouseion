//go:build integration

package persistence

import (
	"context"
	"errors"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
)

func integrationDatabase(t *testing.T, ctx context.Context) string {
	t.Helper()
	url, _ := testutil.Postgres(t, ctx, Migrate)
	return url
}

func TestPostgresOwnershipAndSharedArtifactBoundaries(t *testing.T) {
	ctx := context.Background()
	url := integrationDatabase(t, ctx)
	store, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	alice, err := store.CreateUser(ctx, "alice", false)
	if err != nil {
		t.Fatal(err)
	}
	bob, err := store.CreateUser(ctx, "bob", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.PutLanguageProfile(ctx, alice.ID, "de", "German"); err != nil {
		t.Fatal(err)
	}

	artifact := domain.NormalizedArtifact{ContentHash: "sha256:shared", Language: "de", SchemaVersion: "1.0.0", NormalizationProfile: "de-standard", NormalizationVersion: "1", AnalyzerName: "test", AnalyzerVersion: "1"}
	if err = store.PutArtifact(ctx, artifact, []domain.SharedLemma{{CanonicalLemma: "Haus", UPOS: "NOUN", Morphology: []byte(`{"Gender":"Neut"}`), Frequency: 3}}); err != nil {
		t.Fatal(err)
	}
	if _, lemmas, err := store.GetArtifact(ctx, artifact.ContentHash); err != nil || len(lemmas) != 1 {
		t.Fatalf("shared artifact: lemmas=%d err=%v", len(lemmas), err)
	}

	source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: alice.ID, Language: "de", SourceIdentifier: "book-1", Title: "Private", MediaType: "application/epub+zip", ContentHash: "sha256:shared", Content: []byte("epub"), FullText: "private sentence"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.GetSourceMaterial(ctx, bob.ID, source.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bob read alice source: %v", err)
	}
	library, err := store.ListSourceMaterials(ctx, alice.ID)
	if err != nil || len(library) != 1 || library[0].AnalysisStatus != "not analyzed" {
		t.Fatalf("alice library before analysis: books=%v err=%v", library, err)
	}
	bobLibrary, err := store.ListSourceMaterials(ctx, bob.ID)
	if err != nil || len(bobLibrary) != 0 {
		t.Fatalf("bob library leaked alice source: books=%v err=%v", bobLibrary, err)
	}
	if _, err = store.Pool().Exec(ctx, `INSERT INTO analysis_jobs(river_job_id,owner_id,source_material_id,content_hash) VALUES(84001,$1,$2,$3)`, alice.ID, source.ID, source.ContentHash); err != nil {
		t.Fatal(err)
	}
	library, err = store.ListSourceMaterials(ctx, alice.ID)
	if err != nil || len(library) != 1 || library[0].AnalysisStatus != "analyzing" || library[0].AnalysisJobID != 84001 {
		t.Fatalf("alice library during analysis: books=%v err=%v", library, err)
	}
	corpus, err := store.PutCorpus(ctx, alice.ID, source.ID, artifact.ContentHash)
	if err != nil {
		t.Fatal(err)
	}
	library, err = store.ListSourceMaterials(ctx, alice.ID)
	if err != nil || len(library) != 1 || library[0].AnalysisStatus != "analyzed" || library[0].CorpusID != corpus.ID {
		t.Fatalf("alice library after analysis: books=%v err=%v", library, err)
	}
	if _, err = store.GetCorpus(ctx, bob.ID, corpus.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bob read alice corpus: %v", err)
	}
	if _, err = store.PutExampleSentence(ctx, bob.ID, corpus.ID, "s1", "stolen", []byte(`{}`)); err == nil {
		t.Fatal("bob inserted a sentence into alice corpus")
	}

	state, err := store.PutVocabularyState(ctx, alice.ID, "de", "Haus", "NOUN", "accepted")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.GetVocabularyState(ctx, bob.ID, state.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bob read alice state: %v", err)
	}
	if err = store.DeleteVocabularyState(ctx, bob.ID, state.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bob deleted alice state: %v", err)
	}
	got, err := store.GetVocabularyState(ctx, alice.ID, state.ID)
	if err != nil || got.State != "accepted" {
		t.Fatalf("alice state changed: %+v %v", got, err)
	}

	known, err := store.PutKnownVocabulary(ctx, alice.ID, "de", "gehen", "VERB")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.GetKnownVocabulary(ctx, bob.ID, known.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("bob read alice known vocabulary: %v", err)
	}
	deck, err := store.PutDeck(ctx, alice.ID, "de", "Study")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.PutCard(ctx, domain.Card{OwnerID: bob.ID, DeckID: deck.ID, DedupKey: "x", CanonicalLemma: "Haus", UPOS: "NOUN", Front: "x", Back: "y"}); err == nil {
		t.Fatal("bob inserted a card into alice deck")
	}
}
