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

	aliceBook, _, _ := createJourneyFixture(t, ctx, store, alice.ID, "goal-active")
	bobBook, _, _ := createJourneyFixture(t, ctx, store, bob.ID, "goal-queued")
	_, _, _ = createJourneyFixture(t, ctx, store, dave.ID, "goal-complete")
	_, _, _ = createJourneyFixture(t, ctx, store, erin.ID, "goal-abandoned")

	if goal, err := store.GetPrimaryGoal(ctx, carol.ID, "de"); err != nil || goal != (domain.PrimaryGoal{}) {
		t.Fatalf("owner without a Goal=%+v err=%v", goal, err)
	}
	if _, err = pool.Exec(ctx, migrationSQL(t, "000057_primary_goals_language_constraint.down.sql")); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, migrationSQL(t, "000056_primary_goals_language_backfill.down.sql")); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, migrationSQL(t, "000055_primary_goals_language.down.sql")); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO primary_goals(owner_id,book_id) VALUES($1,$2)`, alice.ID, aliceBook.ID); err != nil {
		t.Fatal(err)
	}
	legacyUnknownBook, err := store.CreateBook(ctx, domain.Book{OwnerID: erin.ID, Title: "Legacy unknown-language goal", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO primary_goals(owner_id,book_id) VALUES($1,$2)`, erin.ID, legacyUnknownBook.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, migrationSQL(t, "000055_primary_goals_language.up.sql")); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, migrationSQL(t, "000056_primary_goals_language_backfill.up.sql")); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, migrationSQL(t, "000057_primary_goals_language_constraint.up.sql")); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, migrationSQL(t, "000056_primary_goals_language_backfill.up.sql")); err != nil {
		t.Fatalf("idempotent backfill: %v", err)
	}

	goal, err := store.GetPrimaryGoal(ctx, alice.ID, "de")
	if err != nil || goal.OwnerID != alice.ID || goal.BookID != aliceBook.ID {
		t.Fatalf("migrated active goal=%+v err=%v", goal, err)
	}
	italianBook, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Alice Italian goal book", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "it"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO primary_goals(owner_id,language,book_id) VALUES($1,$2,$3)`, alice.ID, "it", italianBook.ID); err != nil {
		t.Fatal(err)
	}
	if italianGoal, getErr := store.GetPrimaryGoal(ctx, alice.ID, "it"); getErr != nil || italianGoal.BookID != italianBook.ID {
		t.Fatalf("parallel Italian goal=%+v err=%v", italianGoal, getErr)
	}
	if err = store.ClearPrimaryGoal(ctx, alice.ID, "it", italianBook.ID); err != nil {
		t.Fatalf("clear parallel Italian goal: %v", err)
	}
	if germanGoal, getErr := store.GetPrimaryGoal(ctx, alice.ID, "de"); getErr != nil || germanGoal.BookID != aliceBook.ID {
		t.Fatalf("German goal after Italian clear=%+v err=%v", germanGoal, getErr)
	}
	for _, owner := range []string{bob.ID, carol.ID, dave.ID, erin.ID} {
		if goal, err := store.GetPrimaryGoal(ctx, owner, "de"); err != nil || goal != (domain.PrimaryGoal{}) {
			t.Fatalf("non-active owner=%s goal=%+v err=%v", owner, goal, err)
		}
	}

	otherAliceBook, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Alice second goal book", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreatePrimaryGoal(ctx, alice.ID, "de", otherAliceBook.ID); !errors.Is(err, ErrGoalExists) {
		t.Fatalf("second goal error=%v, want ErrGoalExists", err)
	}

	noDeckBook, err := store.CreateBook(ctx, domain.Book{OwnerID: carol.ID, Title: "Carol reading-only book", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreatePrimaryGoal(ctx, carol.ID, "de", noDeckBook.ID); !errors.Is(err, ErrGoalIneligible) {
		t.Fatalf("no-deck goal error=%v, want ErrGoalIneligible", err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO primary_goals(owner_id,language,book_id) VALUES($1,$2,$3)`, carol.ID, "de", noDeckBook.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreatePrimaryGoal(ctx, carol.ID, "de", bobBook.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner create error=%v, want ErrNotFound", err)
	}

	changed, err := store.ChangePrimaryGoal(ctx, carol.ID, "de", otherAliceBook.ID, noDeckBook.ID)
	if !errors.Is(err, ErrNotFound) || changed != (domain.PrimaryGoal{}) {
		t.Fatalf("cross-owner change goal=%+v err=%v, want ErrNotFound", changed, err)
	}
	replacementBook, replacementSource, _ := createJourneyFixture(t, ctx, store, carol.ID, "goal-replacement")
	makeJourneyMemberAnalyzed(t, ctx, store, replacementBook, replacementSource)
	if _, err = store.ChangePrimaryGoal(ctx, carol.ID, "de", replacementBook.ID, "stale-book"); !errors.Is(err, ErrGoalStale) {
		t.Fatalf("stale change error=%v, want ErrGoalStale", err)
	}
	changed, err = store.ChangePrimaryGoal(ctx, carol.ID, "de", replacementBook.ID, noDeckBook.ID)
	if err != nil || changed.BookID != replacementBook.ID {
		t.Fatalf("valid change goal=%+v err=%v", changed, err)
	}
	if err = store.ClearPrimaryGoal(ctx, carol.ID, "de", noDeckBook.ID); !errors.Is(err, ErrGoalStale) {
		t.Fatalf("stale clear error=%v, want ErrGoalStale", err)
	}
	if err = store.ClearPrimaryGoal(ctx, carol.ID, "de", replacementBook.ID); err != nil {
		t.Fatalf("valid clear: %v", err)
	}
	if err = store.ClearPrimaryGoal(ctx, carol.ID, "de", replacementBook.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("clear absent error=%v, want ErrNotFound", err)
	}
	if err = store.ClearPrimaryGoal(ctx, bob.ID, "de", aliceBook.ID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner clear error=%v, want ErrNotFound", err)
	}
	if goal, err = store.GetPrimaryGoal(ctx, bob.ID, "de"); err != nil || goal != (domain.PrimaryGoal{}) {
		t.Fatalf("cross-owner get goal=%+v err=%v", goal, err)
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
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "Finish this book", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Pool().Exec(ctx, `INSERT INTO primary_goals(owner_id,language,book_id) VALUES($1,$2,$3)`, owner.ID, "de", book.ID); err != nil {
		t.Fatal(err)
	}

	result, err := store.RecordReadingFinishedPrimaryGoal(ctx, owner.ID, "de", book.ID)
	if err != nil || result.Goal.ReadingFinishedAt == nil {
		t.Fatalf("reading-only finish=%+v err=%v", result, err)
	}
	finishedAt := *result.Goal.ReadingFinishedAt
	persisted, err := store.GetPrimaryGoal(ctx, owner.ID, "de")
	if err != nil || persisted.ReadingFinishedAt == nil || !persisted.ReadingFinishedAt.Equal(finishedAt) {
		t.Fatalf("persisted finish=%+v err=%v", persisted, err)
	}

	repeated, err := store.RecordReadingFinishedPrimaryGoal(ctx, owner.ID, "de", book.ID)
	if err != nil || repeated.Goal.ReadingFinishedAt == nil || !repeated.Goal.ReadingFinishedAt.Equal(finishedAt) {
		t.Fatalf("idempotent finish=%+v err=%v", repeated, err)
	}
	if _, err = store.RecordReadingFinishedPrimaryGoal(ctx, owner.ID, "de", "stale-book"); !errors.Is(err, ErrGoalStale) {
		t.Fatalf("stale finish error=%v", err)
	}

	replacement, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "Next book", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.CreatePrimaryGoal(ctx, owner.ID, "de", replacement.ID); !errors.Is(err, ErrGoalIneligible) {
		t.Fatalf("ineligible new Goal error=%v, want ErrGoalIneligible", err)
	}
}
