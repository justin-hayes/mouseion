//go:build integration

package persistence

import (
	"context"
	"errors"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
)

func TestPrimaryGoalBackfillAndPersistence(t *testing.T) {
	ctx := context.Background()
	url, pool := testutil.Postgres(t, ctx, Migrate)
	store, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	alice, err := store.CreateUser(ctx, "goal-alice", false)
	if err != nil {
		t.Fatal(err)
	}
	bob, err := store.CreateUser(ctx, "goal-bob", false)
	if err != nil {
		t.Fatal(err)
	}
	carol, err := store.CreateUser(ctx, "goal-carol", false)
	if err != nil {
		t.Fatal(err)
	}
	dave, err := store.CreateUser(ctx, "goal-dave", false)
	if err != nil {
		t.Fatal(err)
	}
	erin, err := store.CreateUser(ctx, "goal-erin", false)
	if err != nil {
		t.Fatal(err)
	}

	aliceBook, aliceSource, alicePrep := createJourneyFixture(t, ctx, store, alice.ID, "goal-active")
	bobBook, bobSource, bobPrep := createJourneyFixture(t, ctx, store, bob.ID, "goal-queued")
	_, daveSource, davePrep := createJourneyFixture(t, ctx, store, dave.ID, "goal-complete")
	_, erinSource, erinPrep := createJourneyFixture(t, ctx, store, erin.ID, "goal-abandoned")

	insertJourneyCampaign(t, ctx, pool, alice.ID, aliceSource.ID, alicePrep.ID, "reading", "studying", aliceBook.CreatedAt)
	insertJourneyCampaign(t, ctx, pool, bob.ID, bobSource.ID, bobPrep.ID, "queued", "queued", bobBook.CreatedAt)
	insertJourneyCampaign(t, ctx, pool, dave.ID, daveSource.ID, davePrep.ID, "finished", "reviewed", daveSource.CreatedAt)
	insertJourneyCampaign(t, ctx, pool, erin.ID, erinSource.ID, erinPrep.ID, "abandoned", "queued", erinSource.CreatedAt)

	if goal, err := store.GetPrimaryGoal(ctx, carol.ID); err != nil || goal != (domain.PrimaryGoal{}) {
		t.Fatalf("legacy owner without active campaign goal=%+v err=%v", goal, err)
	}
	var beforeSource, beforePreparation string
	if err = pool.QueryRow(ctx, `SELECT source_material_id::text,deck_preparation_id::text FROM learning_campaigns WHERE owner_id=$1`, alice.ID).Scan(&beforeSource, &beforePreparation); err != nil {
		t.Fatal(err)
	}

	backfill := migrationSQL(t, "000041_primary_goals_backfill.up.sql")
	if _, err = pool.Exec(ctx, backfill); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, backfill); err != nil {
		t.Fatal(err)
	}

	goal, err := store.GetPrimaryGoal(ctx, alice.ID)
	if err != nil || goal.OwnerID != alice.ID || goal.BookID != aliceBook.ID {
		t.Fatalf("migrated active goal=%+v err=%v", goal, err)
	}
	var afterSource, afterPreparation string
	if err = pool.QueryRow(ctx, `SELECT source_material_id::text,deck_preparation_id::text FROM learning_campaigns WHERE owner_id=$1`, alice.ID).Scan(&afterSource, &afterPreparation); err != nil {
		t.Fatal(err)
	}
	if beforeSource != afterSource || beforePreparation != afterPreparation {
		t.Fatalf("migration changed campaign links: source %q -> %q, preparation %q -> %q", beforeSource, afterSource, beforePreparation, afterPreparation)
	}
	for _, owner := range []string{bob.ID, carol.ID, dave.ID, erin.ID} {
		if goal, err := store.GetPrimaryGoal(ctx, owner); err != nil || goal != (domain.PrimaryGoal{}) {
			t.Fatalf("non-active owner=%s goal=%+v err=%v", owner, goal, err)
		}
	}

	otherAliceBook, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Alice second goal book", MetadataProvenance: domain.MetadataProvenanceManualEntry, LanguageState: domain.LanguageUnknown})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreatePrimaryGoal(ctx, alice.ID, otherAliceBook.ID); !errors.Is(err, ErrGoalExists) {
		t.Fatalf("second goal error=%v, want ErrGoalExists", err)
	}

	noDeckBook, err := store.CreateBook(ctx, domain.Book{OwnerID: carol.ID, Title: "Carol reading-only book", MetadataProvenance: domain.MetadataProvenanceManualEntry, LanguageState: domain.LanguageUnknown})
	if err != nil {
		t.Fatal(err)
	}
	goal, err = store.CreatePrimaryGoal(ctx, carol.ID, noDeckBook.ID)
	if err != nil || goal.BookID != noDeckBook.ID {
		t.Fatalf("no-deck goal=%+v err=%v", goal, err)
	}
	if _, err = store.CreatePrimaryGoal(ctx, carol.ID, bobBook.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner create error=%v, want ErrNotFound", err)
	}

	changed, err := store.ChangePrimaryGoal(ctx, carol.ID, otherAliceBook.ID, noDeckBook.ID)
	if !errors.Is(err, ErrNotFound) || changed != (domain.PrimaryGoal{}) {
		t.Fatalf("cross-owner change goal=%+v err=%v, want ErrNotFound", changed, err)
	}
	replacementBook, err := store.CreateBook(ctx, domain.Book{OwnerID: carol.ID, Title: "Carol replacement book", MetadataProvenance: domain.MetadataProvenanceManualEntry, LanguageState: domain.LanguageUnknown})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.ChangePrimaryGoal(ctx, carol.ID, replacementBook.ID, "stale-book"); !errors.Is(err, ErrGoalStale) {
		t.Fatalf("stale change error=%v, want ErrGoalStale", err)
	}
	changed, err = store.ChangePrimaryGoal(ctx, carol.ID, replacementBook.ID, noDeckBook.ID)
	if err != nil || changed.BookID != replacementBook.ID {
		t.Fatalf("valid change goal=%+v err=%v", changed, err)
	}
	if err = store.ClearPrimaryGoal(ctx, carol.ID, noDeckBook.ID); !errors.Is(err, ErrGoalStale) {
		t.Fatalf("stale clear error=%v, want ErrGoalStale", err)
	}
	if err = store.ClearPrimaryGoal(ctx, carol.ID, replacementBook.ID); err != nil {
		t.Fatalf("valid clear: %v", err)
	}
	if err = store.ClearPrimaryGoal(ctx, carol.ID, replacementBook.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("clear absent error=%v, want ErrNotFound", err)
	}
	if err = store.ClearPrimaryGoal(ctx, bob.ID, aliceBook.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner clear error=%v, want ErrNotFound", err)
	}
	if goal, err = store.GetPrimaryGoal(ctx, bob.ID); err != nil || goal != (domain.PrimaryGoal{}) {
		t.Fatalf("cross-owner get goal=%+v err=%v", goal, err)
	}

	if _, err = pool.Exec(ctx, migrationSQL(t, "000041_primary_goals_backfill.down.sql")); err != nil {
		t.Fatal(err)
	}
	var goalCount int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM primary_goals`).Scan(&goalCount); err != nil {
		t.Fatal(err)
	}
	if goalCount != 0 {
		t.Fatalf("backfill down left %d goals", goalCount)
	}
	if _, err = pool.Exec(ctx, backfill); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, migrationSQL(t, "000040_primary_goals.down.sql")); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, migrationSQL(t, "000040_primary_goals.up.sql")); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, migrationSQL(t, "000042_primary_goal_reading_finished.up.sql")); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, backfill); err != nil {
		t.Fatal(err)
	}
	goal, err = store.GetPrimaryGoal(ctx, alice.ID)
	if err != nil || goal.BookID != aliceBook.ID {
		t.Fatalf("round-trip goal=%+v err=%v", goal, err)
	}
}

func TestPrimaryGoalReadingFinishIsGuardedPersistentAndIdempotent(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	owner, err := store.CreateUser(ctx, "goal-finish", false)
	if err != nil {
		t.Fatal(err)
	}
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "Finish this book", MetadataProvenance: domain.MetadataProvenanceManualEntry, LanguageState: domain.LanguageUnknown})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreatePrimaryGoal(ctx, owner.ID, book.ID); err != nil {
		t.Fatal(err)
	}

	result, err := store.FinishReadingPrimaryGoal(ctx, owner.ID, book.ID)
	if err != nil || result.Goal.ReadingFinishedAt == nil || result.Campaign != nil || len(result.Graduated) != 0 {
		t.Fatalf("reading-only finish=%+v err=%v", result, err)
	}
	finishedAt := *result.Goal.ReadingFinishedAt
	persisted, err := store.GetPrimaryGoal(ctx, owner.ID)
	if err != nil || persisted.ReadingFinishedAt == nil || !persisted.ReadingFinishedAt.Equal(finishedAt) {
		t.Fatalf("persisted finish=%+v err=%v", persisted, err)
	}

	repeated, err := store.FinishReadingPrimaryGoal(ctx, owner.ID, book.ID)
	if err != nil || repeated.Goal.ReadingFinishedAt == nil || !repeated.Goal.ReadingFinishedAt.Equal(finishedAt) {
		t.Fatalf("idempotent finish=%+v err=%v", repeated, err)
	}
	if _, err = store.FinishReadingPrimaryGoal(ctx, owner.ID, "stale-book"); !errors.Is(err, ErrGoalStale) {
		t.Fatalf("stale finish error=%v", err)
	}

	replacement, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "Next book", MetadataProvenance: domain.MetadataProvenanceManualEntry, LanguageState: domain.LanguageUnknown})
	if err != nil {
		t.Fatal(err)
	}
	if goal, err := store.CreatePrimaryGoal(ctx, owner.ID, replacement.ID); err != nil || goal.BookID != replacement.ID || goal.ReadingFinishedAt != nil {
		t.Fatalf("new Goal after finish=%+v err=%v", goal, err)
	}
}
