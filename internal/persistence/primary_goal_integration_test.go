//go:build integration

package persistence

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrimaryGoalPersistence(t *testing.T) {
	ctx := context.Background()
	url, pool := testutil.Postgres(t, ctx, Migrate)
	store, err := Open(ctx, url)
	require.NoError(t, err)
	defer store.Close()

	alice, err := store.CreateUser(ctx, "goal-alice", false)
	require.NoError(t, err)
	bob, err := store.CreateUser(ctx, "goal-bob", false)
	require.NoError(t, err)
	carol, err := store.CreateUser(ctx, "goal-carol", false)
	require.NoError(t, err)
	dave, err := store.CreateUser(ctx, "goal-dave", false)
	require.NoError(t, err)
	erin, err := store.CreateUser(ctx, "goal-erin", false)
	require.NoError(t, err)

	aliceBook, _, _ := createJourneyFixture(t, ctx, store, alice.ID, "goal-active")
	bobBook, _, _ := createJourneyFixture(t, ctx, store, bob.ID, "goal-queued")
	_, _, _ = createJourneyFixture(t, ctx, store, dave.ID, "goal-complete")
	_, _, _ = createJourneyFixture(t, ctx, store, erin.ID, "goal-abandoned")

	goal, err := store.GetPrimaryGoal(ctx, carol.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, domain.PrimaryGoal{}, goal, "owner without a Goal")
	// Model a legacy goal directly against the baseline. The historical
	// migration that added language is retired, but current goal behavior still
	// needs coverage for persisted language-scoped rows.
	_, err = pool.Exec(ctx, `INSERT INTO primary_goals(owner_id,language,book_id) VALUES($1,'de',$2)`, alice.ID, aliceBook.ID)
	require.NoError(t, err)
	legacyUnknownBook, err := store.CreateBook(ctx, domain.Book{OwnerID: erin.ID, Title: "Legacy unknown-language goal", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown})
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO primary_goals(owner_id,language,book_id) VALUES($1,'und',$2)`, erin.ID, legacyUnknownBook.ID)
	require.NoError(t, err)

	goal, err = store.GetPrimaryGoal(ctx, alice.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, alice.ID, goal.OwnerID, "migrated active goal")
	assert.Equal(t, aliceBook.ID, goal.BookID, "migrated active goal")
	italianBook, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Alice Italian goal book", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "it"})
	require.NoError(t, err)
	_, err = pool.Exec(ctx, `INSERT INTO primary_goals(owner_id,language,book_id) VALUES($1,$2,$3)`, alice.ID, "it", italianBook.ID)
	require.NoError(t, err)
	italianGoal, getErr := store.GetPrimaryGoal(ctx, alice.ID, "it")
	require.NoError(t, getErr)
	assert.Equal(t, italianBook.ID, italianGoal.BookID, "parallel Italian goal")
	err = store.ClearPrimaryGoal(ctx, alice.ID, "it", italianBook.ID)
	require.NoError(t, err, "clear parallel Italian goal")
	germanGoal, getErr := store.GetPrimaryGoal(ctx, alice.ID, "de")
	require.NoError(t, getErr)
	assert.Equal(t, aliceBook.ID, germanGoal.BookID, "German goal after Italian clear")
	for _, owner := range []string{bob.ID, carol.ID, dave.ID, erin.ID} {
		goal, err := store.GetPrimaryGoal(ctx, owner, "de")
		require.NoError(t, err)
		assert.Equal(t, domain.PrimaryGoal{}, goal, "non-active owner=%s", owner)
	}

	otherAliceBook, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Alice second goal book", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown})
	require.NoError(t, err)
	_, err = store.CreatePrimaryGoal(ctx, alice.ID, "de", otherAliceBook.ID)
	assert.ErrorIs(t, err, ErrGoalExists)

	noDeckBook, err := store.CreateBook(ctx, domain.Book{OwnerID: carol.ID, Title: "Carol reading-only book", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown})
	require.NoError(t, err)
	_, err = store.CreatePrimaryGoal(ctx, carol.ID, "de", noDeckBook.ID)
	assert.ErrorIs(t, err, ErrGoalIneligible)
	_, err = pool.Exec(ctx, `INSERT INTO primary_goals(owner_id,language,book_id) VALUES($1,$2,$3)`, carol.ID, "de", noDeckBook.ID)
	require.NoError(t, err)
	_, err = store.CreatePrimaryGoal(ctx, carol.ID, "de", bobBook.ID)
	assert.ErrorIs(t, err, ErrNotFound)

	changed, err := store.ChangePrimaryGoal(ctx, carol.ID, "de", otherAliceBook.ID, noDeckBook.ID)
	assert.ErrorIs(t, err, ErrNotFound)
	assert.Equal(t, domain.PrimaryGoal{}, changed, "cross-owner change goal")
	replacementBook, replacementSource, _ := createJourneyFixture(t, ctx, store, carol.ID, "goal-replacement")
	makeJourneyMemberAnalyzed(t, ctx, store, replacementBook, replacementSource)
	_, err = store.ChangePrimaryGoal(ctx, carol.ID, "de", replacementBook.ID, "stale-book")
	assert.ErrorIs(t, err, ErrGoalStale)
	changed, err = store.ChangePrimaryGoal(ctx, carol.ID, "de", replacementBook.ID, noDeckBook.ID)
	require.NoError(t, err)
	assert.Equal(t, replacementBook.ID, changed.BookID, "valid change goal")
	err = store.ClearPrimaryGoal(ctx, carol.ID, "de", noDeckBook.ID)
	assert.ErrorIs(t, err, ErrGoalStale)
	err = store.ClearPrimaryGoal(ctx, carol.ID, "de", replacementBook.ID)
	require.NoError(t, err, "valid clear")
	err = store.ClearPrimaryGoal(ctx, carol.ID, "de", replacementBook.ID)
	assert.ErrorIs(t, err, ErrNotFound)
	err = store.ClearPrimaryGoal(ctx, bob.ID, "de", aliceBook.ID)
	assert.ErrorIs(t, err, ErrNotFound)
	goal, err = store.GetPrimaryGoal(ctx, bob.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, domain.PrimaryGoal{}, goal, "cross-owner get goal")

}

func TestPrimaryGoalReadingFinishIsGuardedPersistentAndIdempotent(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store, err := Open(ctx, databaseURL)
	require.NoError(t, err)
	defer store.Close()

	owner, err := store.CreateUser(ctx, "goal-finish", false)
	require.NoError(t, err)
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "Finish this book", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown})
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO primary_goals(owner_id,language,book_id) VALUES($1,$2,$3)`, owner.ID, "de", book.ID)
	require.NoError(t, err)

	result, err := store.RecordReadingFinishedPrimaryGoal(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	require.NotNil(t, result.Goal.ReadingFinishedAt, "reading-only finish")
	finishedAt := *result.Goal.ReadingFinishedAt
	persisted, err := store.GetPrimaryGoal(ctx, owner.ID, "de")
	require.NoError(t, err)
	require.NotNil(t, persisted.ReadingFinishedAt, "persisted finish")
	assert.True(t, persisted.ReadingFinishedAt.Equal(finishedAt), "persisted finish")

	repeated, err := store.RecordReadingFinishedPrimaryGoal(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	require.NotNil(t, repeated.Goal.ReadingFinishedAt, "idempotent finish")
	assert.True(t, repeated.Goal.ReadingFinishedAt.Equal(finishedAt), "idempotent finish")
	_, err = store.RecordReadingFinishedPrimaryGoal(ctx, owner.ID, "de", "stale-book")
	assert.ErrorIs(t, err, ErrGoalStale)

	replacement, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "Next book", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown})
	require.NoError(t, err)
	_, err = store.CreatePrimaryGoal(ctx, owner.ID, "de", replacement.ID)
	assert.ErrorIs(t, err, ErrGoalIneligible)
}
