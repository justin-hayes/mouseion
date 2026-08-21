//go:build integration

package cardexport_test

import (
	"context"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/vocabulary"
)

func TestExportPersistsOwnerScopedCardsAndGeneratedStateIdempotently(t *testing.T) {
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
	alice, _ := store.CreateUser(ctx, "export-alice", false)
	bob, _ := store.CreateUser(ctx, "export-bob", false)
	seed := func(owner domain.User, hash, lemma, sentence string) {
		t.Helper()
		artifact := domain.NormalizedArtifact{ContentHash: hash, Language: "de", SchemaVersion: "1", NormalizationProfile: "test", NormalizationVersion: "1", AnalyzerName: "test", AnalyzerVersion: "1"}
		if err := store.PutArtifact(ctx, artifact, []domain.SharedLemma{{CanonicalLemma: lemma, UPOS: "NOUN", Morphology: []byte(`{"Gender":"Neut"}`), Frequency: 1}}); err != nil {
			t.Fatal(err)
		}
		source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: hash, Title: "Test Book", MediaType: "text/plain", ContentHash: hash, Content: []byte(sentence), FullText: sentence})
		if err != nil {
			t.Fatal(err)
		}
		corpus, err := store.PutCorpus(ctx, owner.ID, source.ID, hash)
		if err != nil {
			t.Fatal(err)
		}
		if err = store.ReplaceSelectedSentences(ctx, owner.ID, corpus.ID, "de", lemma, "NOUN", []domain.ExampleSentence{{SentenceKey: "s1", Text: sentence, SourceLocation: []byte(`{}`), SelectionReasons: []byte(`[]`), SelectionRank: 1, Chosen: true}}); err != nil {
			t.Fatal(err)
		}
		examples, err := store.ListReviewSentences(ctx, owner.ID, "de", lemma, "NOUN")
		if err != nil {
			t.Fatal(err)
		}
		if _, err = store.PutCuratedSentence(ctx, owner.ID, examples[0].ID, "de", lemma, "NOUN", "note"); err != nil {
			t.Fatal(err)
		}
		life := vocabulary.NewLifecycle(store)
		id := vocabulary.Identity{Language: "de", CanonicalLemma: lemma, UPOS: "NOUN"}
		if _, err = life.Transition(ctx, owner.ID, id, vocabulary.Candidate); err != nil {
			t.Fatal(err)
		}
		if _, err = life.Transition(ctx, owner.ID, id, vocabulary.Accepted); err != nil {
			t.Fatal(err)
		}
	}
	seed(alice, "export-a", "Haus", "Das Haus ist groß.")
	seed(bob, "export-b", "Baum", "Der Baum ist groß.")
	artifact, err := cardexport.NewService(store).Export(ctx, alice.ID, "German")
	if err != nil {
		t.Fatal(err)
	}
	if artifact.Count != 1 || !contains(artifact.TSV, "Haus") || contains(artifact.TSV, "Baum") {
		t.Fatalf("artifact=%+v", artifact)
	}
	var cards, decks, audits int
	var state string
	if err = conn.QueryRow(ctx, `SELECT count(*) FROM cards WHERE owner_id=$1`, alice.ID).Scan(&cards); err != nil {
		t.Fatal(err)
	}
	_ = conn.QueryRow(ctx, `SELECT count(*) FROM decks WHERE owner_id=$1`, alice.ID).Scan(&decks)
	_ = conn.QueryRow(ctx, `SELECT state FROM vocabulary_states WHERE owner_id=$1 AND canonical_lemma='Haus'`, alice.ID).Scan(&state)
	_ = conn.QueryRow(ctx, `SELECT count(*) FROM processing_history WHERE owner_id=$1 AND operation='vocabulary.transition' AND details->>'to'='generated'`, alice.ID).Scan(&audits)
	if cards != 1 || decks != 1 || state != "generated" || audits != 1 {
		t.Fatalf("cards=%d decks=%d state=%s audits=%d", cards, decks, state, audits)
	}
	again, err := cardexport.NewService(store).Export(ctx, alice.ID, "German")
	if err != nil || again.Count != 0 {
		t.Fatalf("again=%+v err=%v", again, err)
	}
	_ = conn.QueryRow(ctx, `SELECT count(*) FROM cards WHERE owner_id=$1`, alice.ID).Scan(&cards)
	_ = conn.QueryRow(ctx, `SELECT count(*) FROM processing_history WHERE owner_id=$1 AND operation='vocabulary.transition' AND details->>'to'='generated'`, alice.ID).Scan(&audits)
	if cards != 1 || audits != 1 {
		t.Fatalf("idempotence cards=%d audits=%d", cards, audits)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
