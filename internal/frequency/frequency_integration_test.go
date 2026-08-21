//go:build integration

package frequency

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/auth"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

func TestFrequencyAdminLifecycleLookupsRefreshAndRollback(t *testing.T) {
	ctx := context.Background()
	url := os.Getenv("MOUSEION_TEST_DATABASE_URL")
	if url == "" {
		url = "postgres://postgres@localhost:5432/mouseion_test?sslmode=disable"
	}
	lockPool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	lockConn, err := lockPool.Acquire(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = lockConn.Exec(ctx, `SELECT pg_advisory_lock(90420009)`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = lockConn.Exec(context.Background(), `SELECT pg_advisory_unlock(90420009)`)
		lockConn.Release()
		lockPool.Close()
	})
	if _, err = lockConn.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
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
	admin, err := store.CreateUser(ctx, "frequency-admin", true)
	if err != nil {
		t.Fatal(err)
	}
	user, err := store.CreateUser(ctx, "frequency-user", false)
	if err != nil {
		t.Fatal(err)
	}
	service := NewService(store)
	v1Input := "lemma,wortklasse,frequenzklasse\nHaus,Substantiv,6\ndaß,Konjunktion,2\nalt,Adjektiv,0\n"
	if _, _, err = service.Create(ctx, user.ID, "de", "2026-08-20 13:00:00 CEST", strings.NewReader(v1Input)); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("non-admin create error = %v", err)
	}
	v1, parsed, err := service.Create(ctx, admin.ID, "de", "2026-08-20 13:00:00 CEST", strings.NewReader(v1Input))
	if err != nil || len(parsed.Entries) != 3 {
		t.Fatalf("create v1: entries=%d err=%v", len(parsed.Entries), err)
	}
	if err = service.Activate(ctx, admin.ID, v1.ID); err != nil {
		t.Fatal(err)
	}
	percentile, found, err := service.FrequencyPercentile(ctx, "de", "HAUS", "noun")
	if err != nil || !found || percentile <= 0.5 {
		t.Fatalf("shared canonical lookup: percentile=%v found=%v err=%v", percentile, found, err)
	}
	if _, found, err = service.FrequencyPercentile(ctx, "fr", "haus", "NOUN"); err == nil || found {
		t.Fatalf("language scope: found=%v err=%v", found, err)
	}
	top, found, err := service.IsTopPercentile(ctx, "de", "Haus", "NOUN", percentile)
	if err != nil || !found || !top {
		t.Fatalf("membership: top=%v found=%v err=%v", top, found, err)
	}

	v2Input := "lemma,wortklasse,frequenzklasse\nHaus,Substantiv,0\nneu,Adjektiv,6\n"
	v2, _, err := service.Replace(ctx, admin.ID, "de", "2026-08-21 13:00:46 CEST", strings.NewReader(v2Input))
	if err != nil {
		t.Fatal(err)
	}
	active, err := service.GetActive(ctx, "de")
	if err != nil || active.ID != v2.ID {
		t.Fatalf("refresh active=%+v err=%v", active, err)
	}
	if _, found, err = service.FrequencyPercentile(ctx, "de", "dass", "CCONJ"); err != nil || found {
		t.Fatalf("old entry visible after refresh: found=%v err=%v", found, err)
	}
	if err = service.Activate(ctx, admin.ID, v1.ID); err != nil {
		t.Fatal(err)
	}
	if _, found, err = service.FrequencyPercentile(ctx, "de", "daß", "CCONJ"); err != nil || !found {
		t.Fatalf("rollback lookup: found=%v err=%v", found, err)
	}
	if err = service.Remove(ctx, user.ID, v2.ID); !errors.Is(err, auth.ErrForbidden) {
		t.Fatalf("non-admin remove error = %v", err)
	}
	if err = service.Remove(ctx, admin.ID, v2.ID); err != nil {
		t.Fatal(err)
	}
	datasets, err := service.List(ctx, "de")
	if err != nil || len(datasets) != 1 || datasets[0].ID != v1.ID {
		t.Fatalf("datasets=%+v err=%v", datasets, err)
	}
}
