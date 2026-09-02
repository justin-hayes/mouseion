//go:build integration

package persistence

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/migrations"
)

func TestCreateFirstUserAndSessionIsAtomicAndOwnerReady(t *testing.T) {
	ctx := context.Background()
	store, err := Open(ctx, integrationDatabase(t, ctx))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	if exists, err := store.HasUsers(ctx); err != nil || exists {
		t.Fatalf("fresh users exists=%v err=%v", exists, err)
	}
	u, created, err := store.CreateFirstUserAndSession(ctx, "alice", "hash", "token-hash", time.Now().Add(time.Hour))
	if err != nil || !created || u.Username != "alice" {
		t.Fatalf("create first: %+v created=%v err=%v", u, created, err)
	}
	if _, _, err = store.GetSession(ctx, "token-hash"); err != nil {
		t.Fatalf("initial session: %v", err)
	}
	if _, err = store.PutSupportedLanguage(ctx, "de", "German"); err != nil {
		t.Fatalf("supported language: %v", err)
	}
	if _, err = store.PutLanguageProfile(ctx, u.ID, "de", "German"); err != nil {
		t.Fatalf("learner ownership: %v", err)
	}
	if _, created, err = store.CreateFirstUserAndSession(ctx, "bob", "hash", "other-token", time.Now().Add(time.Hour)); err != nil || created {
		t.Fatalf("second create created=%v err=%v", created, err)
	}
}

func integrationDatabase(t *testing.T, ctx context.Context) string {
	t.Helper()
	url, _ := testutil.Postgres(t, ctx, Migrate)
	return url
}

func TestRemoveAdminRoleMigrationPreservesAccounts(t *testing.T) {
	ctx := context.Background()
	url := integrationDatabase(t, ctx)
	store, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	var id string
	if err = store.Pool().QueryRow(ctx, `INSERT INTO users(username,password_hash,is_admin) VALUES('legacy-admin','hash',true) RETURNING id`).Scan(&id); err != nil {
		t.Fatal(err)
	}
	migration, err := migrations.FS.ReadFile("000017_remove_admin_role.up.sql")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Pool().Exec(ctx, string(migration)); err != nil {
		t.Fatal(err)
	}
	var username, passwordHash string
	var isAdmin bool
	if err = store.Pool().QueryRow(ctx, `SELECT username,password_hash,is_admin FROM users WHERE id=$1`, id).Scan(&username, &passwordHash, &isAdmin); err != nil {
		t.Fatal(err)
	}
	if username != "legacy-admin" || passwordHash != "hash" || isAdmin {
		t.Fatalf("migrated account username=%q hash=%q is_admin=%v", username, passwordHash, isAdmin)
	}
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
	if _, err = store.PutSupportedLanguage(ctx, "de", "German"); err != nil {
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
	if _, err = store.Pool().Exec(ctx, `INSERT INTO analysis_jobs(river_job_id,display_number,owner_id,source_material_id,content_hash) VALUES(84001,1,$1,$2,$3)`, alice.ID, source.ID, source.ContentHash); err != nil {
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
	legacyCorpus, err := store.GetCorpus(ctx, alice.ID, corpus.ID)
	if err != nil || legacyCorpus.Statistics != nil {
		t.Fatalf("legacy corpus statistics = %+v, err = %v; want unavailable", legacyCorpus.Statistics, err)
	}
	library, err = store.ListSourceMaterials(ctx, alice.ID)
	if err != nil || len(library) != 1 || library[0].AnalysisStatus != "analyzing" || library[0].AnalysisRunID != "" || library[0].CorpusID != "" {
		t.Fatalf("alice library after legacy corpus: books=%v err=%v", library, err)
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
