//go:build integration

package review

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/vocabulary"
)

func TestAcceptForBookPersistsAnalysisExampleWhenNoneWasRanked(t *testing.T) {
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
	if _, err = conn.Exec(ctx, `SELECT pg_advisory_lock(90420111)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = conn.Exec(context.Background(), `SELECT pg_advisory_unlock(90420111)`)
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
	owner, err := store.CreateUser(ctx, "review-fallback", false)
	if err != nil {
		t.Fatal(err)
	}
	artifact := domain.NormalizedArtifact{ContentHash: "review-fallback-artifact", Language: "de", SchemaVersion: "1", NormalizationProfile: "test", NormalizationVersion: "1", AnalyzerName: "test", AnalyzerVersion: "1"}
	if err = store.PutArtifact(ctx, artifact, nil); err != nil {
		t.Fatal(err)
	}
	source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: "review-fallback-book", Title: "Review fallback", MediaType: "text/plain", ContentHash: artifact.ContentHash, Content: []byte("private"), FullText: "Das Dach ist neu."})
	if err != nil {
		t.Fatal(err)
	}
	corpus, err := store.PutCorpus(ctx, owner.ID, source.ID, artifact.ContentHash)
	if err != nil {
		t.Fatal(err)
	}
	id := vocabulary.Identity{Language: "de", CanonicalLemma: "Dach", UPOS: "NOUN"}
	kept, err := store.PutSelectionCandidate(ctx, domain.SelectionCandidate{OwnerID: owner.ID, CorpusID: corpus.ID, Language: id.Language, CanonicalLemma: id.CanonicalLemma, UPOS: id.UPOS, OccurrenceCount: 1, ObservedForms: []byte(`["Dach"]`), SentenceReferences: []byte(`[{"sentence_index":0,"text":"Das Dach ist neu.","location":{"start_offset":4,"end_offset":8}}]`), Provenance: []byte(`{"occurrence_count":1}`)})
	if err != nil || !kept {
		t.Fatalf("put candidate kept=%v err=%v", kept, err)
	}

	state, err := NewService(vocabulary.NewLifecycle(store), store).AcceptForBook(ctx, owner.ID, source.ID, id)
	if err != nil || state.State != "accepted" {
		t.Fatalf("accept=%+v err=%v", state, err)
	}
	var text string
	if err = conn.QueryRow(ctx, `SELECT e.sentence_text FROM curated_sentences c JOIN example_sentences e ON e.owner_id=c.owner_id AND e.id=c.example_sentence_id WHERE c.owner_id=$1 AND c.language=$2 AND c.canonical_lemma=$3 AND c.upos=$4`, owner.ID, id.Language, id.CanonicalLemma, id.UPOS).Scan(&text); err != nil || text != "Das Dach ist neu." {
		t.Fatalf("curated text=%q err=%v", text, err)
	}
}

func TestReviewPersistsOwnerScopedLifecycleAndCuration(t *testing.T) {
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
	alice, err := store.CreateUser(ctx, "review-alice", false)
	if err != nil {
		t.Fatal(err)
	}
	bob, err := store.CreateUser(ctx, "review-bob", false)
	if err != nil {
		t.Fatal(err)
	}
	artifact := domain.NormalizedArtifact{ContentHash: "review-artifact", Language: "de", SchemaVersion: "1", NormalizationProfile: "test", NormalizationVersion: "1", AnalyzerName: "test", AnalyzerVersion: "1"}
	if err = store.PutArtifact(ctx, artifact, nil); err != nil {
		t.Fatal(err)
	}
	source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: alice.ID, Language: "de", SourceIdentifier: "review-book", Title: "Review", MediaType: "text/plain", ContentHash: artifact.ContentHash, Content: []byte("private"), FullText: "Das Haus ist alt. Ein Haus steht dort."})
	if err != nil {
		t.Fatal(err)
	}
	corpus, err := store.PutCorpus(ctx, alice.ID, source.ID, artifact.ContentHash)
	if err != nil {
		t.Fatal(err)
	}
	id := vocabulary.Identity{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN"}
	if err = store.ReplaceSelectedSentences(ctx, alice.ID, corpus.ID, id.Language, id.CanonicalLemma, id.UPOS, []domain.ExampleSentence{
		{SentenceKey: "chosen", Text: "Das Haus ist alt.", SourceLocation: []byte(`{}`), SelectionReasons: []byte(`[]`), SelectionRank: 1, SelectionScore: 75, Chosen: true},
		{SentenceKey: "alternate", Text: "Ein Haus steht dort.", SourceLocation: []byte(`{}`), SelectionReasons: []byte(`[]`), SelectionRank: 2, SelectionScore: 70},
	}); err != nil {
		t.Fatal(err)
	}
	lifecycle := vocabulary.NewLifecycle(store)
	if _, err = lifecycle.Transition(ctx, alice.ID, id, vocabulary.Candidate); err != nil {
		t.Fatal(err)
	}
	service := NewService(lifecycle, store)
	if state, err := service.Accept(ctx, alice.ID, id); err != nil || state.State != "accepted" {
		t.Fatalf("accept=%+v err=%v", state, err)
	}
	if _, err = service.EditExample(ctx, alice.ID, id, "Das Haus wurde von mir bearbeitet."); err != nil {
		t.Fatal(err)
	}
	chosen, err := service.ChooseAlternate(ctx, alice.ID, id, 0)
	if err != nil {
		t.Fatal(err)
	}
	var text, curatedExample, notes string
	if err = conn.QueryRow(ctx, `SELECT e.sentence_text,c.example_sentence_id::text,c.notes FROM curated_sentences c JOIN example_sentences e ON e.id=c.example_sentence_id AND e.owner_id=c.owner_id WHERE c.owner_id=$1 AND c.language=$2 AND c.canonical_lemma=$3 AND c.upos=$4`, alice.ID, id.Language, id.CanonicalLemma, id.UPOS).Scan(&text, &curatedExample, &notes); err != nil {
		t.Fatal(err)
	}
	if curatedExample != chosen.ExampleSentenceID || text != "Ein Haus steht dort." || notes != "alternate:0" {
		t.Fatalf("curated example=%s text=%q notes=%q", curatedExample, text, notes)
	}
	var auditCount int
	if err = conn.QueryRow(ctx, `SELECT count(*) FROM processing_history WHERE owner_id=$1 AND operation IN ('review.edit_example','review.choose_alternate') AND completed_at IS NOT NULL`, alice.ID).Scan(&auditCount); err != nil || auditCount != 2 {
		t.Fatalf("audit count=%d err=%v", auditCount, err)
	}
	if _, err = service.EditExample(ctx, bob.ID, id, "stolen"); !errors.Is(err, ErrNoExample) {
		t.Fatalf("bob edited alice sentence: %v", err)
	}
	if _, err = service.Ignore(ctx, alice.ID, id); err != nil {
		t.Fatal(err)
	}
	if items, err := service.Present(ctx, alice.ID, []Input{aggregate(id)}); err != nil || len(items) != 0 {
		t.Fatalf("ignored item reappeared: %+v err=%v", items, err)
	}
	if _, err = service.Reset(ctx, alice.ID, id); err != nil {
		t.Fatal(err)
	}
	if items, err := service.Present(ctx, alice.ID, []Input{aggregate(id)}); err != nil || len(items) != 1 || items[0].State != vocabulary.Candidate {
		t.Fatalf("reset item=%+v err=%v", items, err)
	}
	if _, err = service.MarkKnown(ctx, alice.ID, id); err != nil {
		t.Fatal(err)
	}
	aliceState, err := store.GetVocabularyStateByIdentity(ctx, alice.ID, id.Language, id.CanonicalLemma, id.UPOS)
	if err != nil || aliceState.State != "known" {
		t.Fatalf("alice state=%+v err=%v", aliceState, err)
	}
	if _, err = store.GetVocabularyStateByIdentity(ctx, bob.ID, id.Language, id.CanonicalLemma, id.UPOS); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("bob state leaked: %v", err)
	}
}
