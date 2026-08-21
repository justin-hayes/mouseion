//go:build integration

package sentences

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

func TestSelectionPersistenceIsOwnerScoped(t *testing.T) {
	ctx := context.Background()
	url := os.Getenv("MOUSEION_TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://postgres@localhost:5432/mouseion_test?sslmode=disable"
	}
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	conn, err := pool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = conn.Exec(ctx, `SELECT pg_advisory_lock(90420009)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock(90420009)`)
		conn.Release()
		pool.Close()
	})
	if _, err = conn.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	if err = persistence.Migrate(url); err != nil {
		t.Fatal(err)
	}
	store, err := persistence.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	alice, err := store.CreateUser(ctx, "sentence-alice", false)
	if err != nil {
		t.Fatal(err)
	}
	bob, err := store.CreateUser(ctx, "sentence-bob", false)
	if err != nil {
		t.Fatal(err)
	}
	artifact := domain.NormalizedArtifact{ContentHash: "sentence-artifact", Language: "de", SchemaVersion: "1", NormalizationProfile: "test", NormalizationVersion: "1", AnalyzerName: "test", AnalyzerVersion: "1"}
	if err = store.PutArtifact(ctx, artifact, nil); err != nil {
		t.Fatal(err)
	}
	source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: alice.ID, Language: "de", SourceIdentifier: "alice-book", Title: "Private", MediaType: "text/plain", ContentHash: artifact.ContentHash, Content: []byte("private"), FullText: "Vor dem kleinen Haus spielt heute ein fröhliches Kind."})
	if err != nil {
		t.Fatal(err)
	}
	corpus, err := store.PutCorpus(ctx, alice.ID, source.ID, artifact.ContentHash)
	if err != nil {
		t.Fatal(err)
	}
	private := ref(0, "Vor dem kleinen Haus spielt heute ein fröhliches Kind.")
	result, err := NewService(store).Select(ctx, alice.ID, corpus.ID, candidate(private), DefaultConfig())
	if err != nil || result.Chosen == nil {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	stored, err := store.ListSelectedSentences(ctx, alice.ID, corpus.ID, "de", "Haus", "NOUN")
	if err != nil || len(stored) != 1 || !stored[0].Chosen || stored[0].SelectionScore == 0 {
		t.Fatalf("stored=%+v err=%v", stored, err)
	}
	bobView, err := store.ListSelectedSentences(ctx, bob.ID, corpus.ID, "de", "Haus", "NOUN")
	if err != nil || len(bobView) != 0 {
		t.Fatalf("bob read alice sentence: %+v err=%v", bobView, err)
	}
	_, err = NewService(store).Select(ctx, bob.ID, corpus.ID, candidate(private), DefaultConfig())
	if !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("bob persisted against alice corpus: %v", err)
	}
}
