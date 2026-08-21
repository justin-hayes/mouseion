//go:build integration

package ranking

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/frequency"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/selection"
)

func TestRankingPersistsOwnerAndLanguageScopedComponents(t *testing.T) {
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
	admin, _ := store.CreateUser(ctx, "ranking-admin", true)
	alice, _ := store.CreateUser(ctx, "ranking-alice", false)
	bob, _ := store.CreateUser(ctx, "ranking-bob", false)
	freq := frequency.NewService(store)
	input := "lemma,wortklasse,frequenzklasse\nHaus,Substantiv,6\ncasa,Substantiv,0\n"
	dataset, _, err := freq.Create(ctx, admin.ID, "de", "ranking", strings.NewReader(input))
	if err != nil {
		t.Fatal(err)
	}
	if err = freq.Activate(ctx, admin.ID, dataset.ID); err != nil {
		t.Fatal(err)
	}
	for _, owner := range []string{alice.ID, bob.ID} {
		for _, language := range []string{"de", "it"} {
			_, err = store.PutSelectionCandidate(ctx, domain.SelectionCandidate{OwnerID: owner, CorpusID: "book", Language: language, CanonicalLemma: "Haus", UPOS: "NOUN", OccurrenceCount: 1, ObservedForms: []byte(`[]`), SentenceReferences: []byte(`[]`), Provenance: []byte(`{}`)})
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	cfg := DefaultConfig("book")
	cfg.GlobalFrequencyCutoff = 0
	got, err := NewService(store, freq).Rank(ctx, alice.ID, []selection.Candidate{candidate("de", "Haus", "NOUN", 1, false, "book")}, cfg)
	if err != nil || got[0].Components.GlobalPercentile == 0 {
		t.Fatalf("got=%+v err=%v", got, err)
	}
	var aliceDE, aliceIT, bobDE *float64
	var global, corpus float64
	var priority bool
	var crossText int
	if err = conn.QueryRow(ctx, `SELECT ranking_score,ranking_global_pct,ranking_corpus_pct,ranking_priority,ranking_cross_text FROM selection_candidates WHERE owner_id=$1 AND language='de'`, alice.ID).Scan(&aliceDE, &global, &corpus, &priority, &crossText); err != nil {
		t.Fatal(err)
	}
	if err = conn.QueryRow(ctx, `SELECT ranking_score FROM selection_candidates WHERE owner_id=$1 AND language='it'`, alice.ID).Scan(&aliceIT); err != nil {
		t.Fatal(err)
	}
	if err = conn.QueryRow(ctx, `SELECT ranking_score FROM selection_candidates WHERE owner_id=$1 AND language='de'`, bob.ID).Scan(&bobDE); err != nil {
		t.Fatal(err)
	}
	if aliceDE == nil || global == 0 || corpus != 1 || priority || crossText != 1 || aliceIT != nil || bobDE != nil {
		t.Fatalf("alice components score=%v global=%v corpus=%v priority=%v cross=%v; isolated alice-it=%v bob-de=%v", aliceDE, global, corpus, priority, crossText, aliceIT, bobDE)
	}
}
