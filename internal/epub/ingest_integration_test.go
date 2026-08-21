//go:build integration

package epub

import (
	"context"
	"errors"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

func TestImportPostgresOwnerIsolationHistoryAndDeletion(t *testing.T) {
	ctx := context.Background()
	url := os.Getenv("MOUSEION_TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://postgres@localhost:5432/mouseion_test?sslmode=disable"
	}
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	conn, err := admin.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Release()
	if _, err = conn.Exec(ctx, `SELECT pg_advisory_lock(90420009)`); err != nil {
		t.Fatal(err)
	}
	defer conn.Exec(context.Background(), `SELECT pg_advisory_unlock(90420009)`)
	if _, err = admin.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
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
	alice, err := store.CreateUser(ctx, "epub-alice", false)
	if err != nil {
		t.Fatal(err)
	}
	bob, err := store.CreateUser(ctx, "epub-bob", false)
	if err != nil {
		t.Fatal(err)
	}
	result, err := NewService(store).Import(ctx, alice.ID, "de", fixture(t))
	if err != nil {
		t.Fatal(err)
	}
	got, err := store.GetSourceMaterial(ctx, alice.ID, result.Source.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got.OwnerID != alice.ID || got.FullText != result.Book.FullText || len(got.Content) == 0 {
		t.Fatalf("stored source: %+v", got)
	}
	if _, err = store.GetSourceMaterial(ctx, bob.ID, result.Source.ID); !errors.Is(err, persistence.ErrNotFound) {
		t.Fatalf("bob read alice source: %v", err)
	}
	var historyCount int
	if err = admin.QueryRow(ctx, `SELECT count(*) FROM processing_history WHERE owner_id=$1 AND operation='epub.import' AND status='complete' AND details->>'source_material_id'=$2`, alice.ID, result.Source.ID).Scan(&historyCount); err != nil || historyCount != 1 {
		t.Fatalf("history count=%d err=%v", historyCount, err)
	}
	if _, err = admin.Exec(ctx, `DELETE FROM users WHERE id=$1`, alice.ID); err != nil {
		t.Fatal(err)
	}
	var sourceCount, remainingHistory int
	if err = admin.QueryRow(ctx, `SELECT count(*) FROM source_materials WHERE id=$1`, result.Source.ID).Scan(&sourceCount); err != nil {
		t.Fatal(err)
	}
	if err = admin.QueryRow(ctx, `SELECT count(*) FROM processing_history WHERE owner_id=$1`, alice.ID).Scan(&remainingHistory); err != nil {
		t.Fatal(err)
	}
	if sourceCount != 0 || remainingHistory != 0 {
		t.Fatalf("private artifacts survived deletion: sources=%d history=%d", sourceCount, remainingHistory)
	}
}
