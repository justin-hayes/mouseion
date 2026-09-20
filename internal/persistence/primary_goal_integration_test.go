//go:build integration

package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrimaryGoalPersistence(t *testing.T) {
	ctx := context.Background()
	url, pool := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, url)

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
	assert.ErrorIs(t, err, ErrGoalExists) //nolint:testifylint // Duplicate-goal rejection is independent of later eligibility cases.

	noDeckBook, err := store.CreateBook(ctx, domain.Book{OwnerID: carol.ID, Title: "Carol reading-only book", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown})
	require.NoError(t, err)
	_, err = store.CreatePrimaryGoal(ctx, carol.ID, "de", noDeckBook.ID)
	assert.ErrorIs(t, err, ErrGoalIneligible) //nolint:testifylint // Eligibility rejection is an independent goal case.
	_, err = pool.Exec(ctx, `INSERT INTO primary_goals(owner_id,language,book_id) VALUES($1,$2,$3)`, carol.ID, "de", noDeckBook.ID)
	require.NoError(t, err)
	_, err = store.CreatePrimaryGoal(ctx, carol.ID, "de", bobBook.ID)
	assert.ErrorIs(t, err, ErrNotFound) //nolint:testifylint // Cross-owner goal rejection is independently asserted.

	changed, err := store.ChangePrimaryGoal(ctx, carol.ID, "de", otherAliceBook.ID, noDeckBook.ID)
	assert.ErrorIs(t, err, ErrNotFound) //nolint:testifylint // Stale-book change rejection is independent of the valid replacement.
	assert.Equal(t, domain.PrimaryGoal{}, changed, "cross-owner change goal")
	replacementBook, replacementSource, _ := createJourneyFixture(t, ctx, store, carol.ID, "goal-replacement")
	makeJourneyMemberAnalyzed(t, ctx, store, replacementBook, replacementSource)
	_, err = store.ChangePrimaryGoal(ctx, carol.ID, "de", replacementBook.ID, "stale-book")
	assert.ErrorIs(t, err, ErrGoalStale) //nolint:testifylint // Stale-book change rejection is independent of the valid replacement.
	changed, err = store.ChangePrimaryGoal(ctx, carol.ID, "de", replacementBook.ID, noDeckBook.ID)
	require.NoError(t, err)
	assert.Equal(t, replacementBook.ID, changed.BookID, "valid change goal")
	err = store.ClearPrimaryGoal(ctx, carol.ID, "de", noDeckBook.ID)
	assert.ErrorIs(t, err, ErrGoalStale) //nolint:testifylint // Stale clear rejection is independent of the valid clear.
	err = store.ClearPrimaryGoal(ctx, carol.ID, "de", replacementBook.ID)
	require.NoError(t, err, "valid clear")
	err = store.ClearPrimaryGoal(ctx, carol.ID, "de", replacementBook.ID)
	assert.ErrorIs(t, err, ErrNotFound) //nolint:testifylint // Repeated clear rejection is an independent idempotency case.
	err = store.ClearPrimaryGoal(ctx, bob.ID, "de", aliceBook.ID)
	assert.ErrorIs(t, err, ErrNotFound) //nolint:testifylint // Cross-owner clear rejection is independent of the final owner lookup.
	goal, err = store.GetPrimaryGoal(ctx, bob.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, domain.PrimaryGoal{}, goal, "cross-owner get goal")

}

func TestPrimaryGoalReadingFinishIsGuardedPersistentAndIdempotent(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)

	owner, err := store.CreateUser(ctx, "goal-finish", false)
	require.NoError(t, err)
	book, bookSource, _ := createJourneyFixture(t, ctx, store, owner.ID, "finish")
	replacement, replacementSource, _ := createJourneyFixture(t, ctx, store, owner.ID, "finish-replacement")
	makeJourneyMemberAnalyzed(t, ctx, store, book, bookSource)
	makeJourneyMemberAnalyzed(t, ctx, store, replacement, replacementSource)
	journey, err := store.GetReadingJourney(ctx, owner.ID, "de")
	require.NoError(t, err)
	_, err = store.AddToReadingJourney(ctx, owner.ID, "de", book.ID, journey.Revision)
	require.NoError(t, err)
	journey, err = store.GetReadingJourney(ctx, owner.ID, "de")
	require.NoError(t, err)
	_, err = store.AddToReadingJourney(ctx, owner.ID, "de", replacement.ID, journey.Revision)
	require.NoError(t, err)
	goal, err := store.CreatePrimaryGoal(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `
CREATE FUNCTION test_reading_completion_failure() RETURNS trigger
LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'forced reading completion failure'; END; $$;
CREATE TRIGGER test_reading_completion_failure
BEFORE DELETE ON reading_journey_membership
FOR EACH ROW EXECUTE FUNCTION test_reading_completion_failure();`)
	require.NoError(t, err)
	_, err = store.RecordReadingFinishedPrimaryGoal(ctx, owner.ID, "de", book.ID, goal.SnapshotID)
	require.Error(t, err)
	var historyCount int
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM reading_history WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&historyCount)
	require.NoError(t, err)
	assert.Zero(t, historyCount, "failed completion left durable history")
	goalAfterRollback, err := store.GetPrimaryGoal(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, book.ID, goalAfterRollback.BookID, "failed completion cleared Goal")
	journeyAfterRollback, err := store.GetReadingJourney(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Contains(t, journeyBookIDs(journeyAfterRollback), book.ID, "failed completion removed Book from Journey")
	knownAfterRollback, err := store.ListKnownVocabulary(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Empty(t, knownAfterRollback, "failed completion changed Known vocabulary")
	_, err = store.Pool().Exec(ctx, `DROP TRIGGER test_reading_completion_failure ON reading_journey_membership; DROP FUNCTION test_reading_completion_failure();`)
	require.NoError(t, err)

	result, err := store.RecordReadingFinishedPrimaryGoal(ctx, owner.ID, "de", book.ID, goal.SnapshotID)
	require.NoError(t, err)
	assert.Equal(t, book.ID, result.Completion.BookID, "reading completion")
	persisted, err := store.GetPrimaryGoal(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Empty(t, persisted.BookID, "completed Goal cleared")
	var historyBook, historyLanguage string
	var completedAt time.Time
	err = store.Pool().QueryRow(ctx, `SELECT book_id::text, language, completed_at FROM reading_history WHERE owner_id=$1`, owner.ID).Scan(&historyBook, &historyLanguage, &completedAt)
	require.NoError(t, err)
	assert.Equal(t, book.ID, historyBook)
	assert.Equal(t, "de", historyLanguage)
	assert.False(t, completedAt.IsZero())
	journey, err = store.GetReadingJourney(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.NotContains(t, journeyBookIDs(journey), book.ID, "completed Book removed from Journey")

	repeated, err := store.RecordReadingFinishedPrimaryGoal(ctx, owner.ID, "de", book.ID, goal.SnapshotID)
	require.NoError(t, err)
	assert.Equal(t, result.Completion, repeated.Completion, "idempotent finish")
	_, err = store.CreatePrimaryGoal(ctx, owner.ID, "de", replacement.ID)
	require.NoError(t, err)
	_, err = store.RecordReadingFinishedPrimaryGoal(ctx, owner.ID, "de", book.ID, goal.SnapshotID)
	require.ErrorIs(t, err, ErrGoalStale)
	goalAfterStale, err := store.GetPrimaryGoal(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, replacement.ID, goalAfterStale.BookID, "stale completion changed replacement Goal")
	err = store.ClearPrimaryGoal(ctx, owner.ID, "de", replacement.ID)
	require.NoError(t, err)
	journey, err = store.GetReadingJourney(ctx, owner.ID, "de")
	require.NoError(t, err)
	_, err = store.AddToReadingJourney(ctx, owner.ID, "de", book.ID, journey.Revision)
	require.NoError(t, err)
	secondGoal, err := store.CreatePrimaryGoal(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	second, err := store.RecordReadingFinishedPrimaryGoal(ctx, owner.ID, "de", book.ID, secondGoal.SnapshotID)
	require.NoError(t, err)
	assert.NotEqual(t, result.Completion.CompletedAt, second.Completion.CompletedAt)
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM reading_history WHERE owner_id=$1 AND language='de' AND book_id=$2`, owner.ID, book.ID).Scan(&historyCount)
	require.NoError(t, err)
	assert.Equal(t, 2, historyCount)
	oldRetry, err := store.RecordReadingFinishedPrimaryGoal(ctx, owner.ID, "de", book.ID, goal.SnapshotID)
	require.NoError(t, err)
	assert.Equal(t, result.Completion, oldRetry.Completion)
}

func TestPrimaryGoalReadingFinishConcurrentRequestsTransitionOnce(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "goal-finish-concurrent", false)
	require.NoError(t, err)
	book, source, _ := createJourneyFixture(t, ctx, store, owner.ID, "finish-concurrent")
	makeJourneyMemberAnalyzed(t, ctx, store, book, source)
	var corpusID string
	err = store.Pool().QueryRow(ctx, `SELECT corpus_id::text FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&corpusID)
	require.NoError(t, err)
	_, err = store.PutSelectionCandidate(ctx, domain.SelectionCandidate{
		OwnerID: owner.ID, CorpusID: corpusID, Language: "de", CanonicalLemma: "concurrent", UPOS: "NOUN",
		OccurrenceCount: 5, ObservedForms: []byte(`[]`), SentenceReferences: []byte(`[]`), Provenance: []byte(`{}`),
	})
	require.NoError(t, err)
	_, err = store.PutSelectionCandidate(ctx, domain.SelectionCandidate{
		OwnerID: owner.ID, CorpusID: corpusID, Language: "de", CanonicalLemma: "legacy-concurrent", UPOS: "VERB",
		OccurrenceCount: 5, ObservedForms: []byte(`[]`), SentenceReferences: []byte(`[]`), Provenance: []byte(`{}`),
	})
	require.NoError(t, err)
	goal, err := store.CreatePrimaryGoal(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	_, err = store.PutVocabularyState(ctx, owner.ID, "de", "legacy-concurrent", "VERB", "known")
	require.NoError(t, err)

	start := make(chan struct{})
	results := make(chan ReadingFinishResult, 2)
	errors := make(chan error, 2)
	for range 2 {
		go func() {
			<-start
			result, finishErr := store.RecordReadingFinishedPrimaryGoal(ctx, owner.ID, "de", book.ID, goal.SnapshotID)
			results <- result
			errors <- finishErr
		}()
	}
	close(start)
	for range 2 {
		require.NoError(t, <-errors)
	}
	first, second := <-results, <-results
	assert.Equal(t, first.Completion, second.Completion)

	var historyCount int
	err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM reading_history WHERE owner_id=$1 AND language='de' AND book_id=$2`, owner.ID, book.ID).Scan(&historyCount)
	require.NoError(t, err)
	assert.Equal(t, 1, historyCount)
	known, err := store.ListKnownVocabulary(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Len(t, known, 1)
	assert.Equal(t, 2, first.Completion.SnapshotVocabularyCount)
	assert.Equal(t, 1, first.Completion.EligibleVocabularyCount)
	assert.Equal(t, 1, first.Completion.GraduatedVocabularyCount)
	assert.Equal(t, 1, first.Completion.AlreadyKnownVocabularyCount)
	persisted, err := store.GetPrimaryGoal(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Empty(t, persisted.BookID)
}

func journeyBookIDs(journey domain.ReadingJourney) []string {
	ids := make([]string, 0, len(journey.Entries))
	for _, entry := range journey.Entries {
		ids = append(ids, entry.BookID)
	}
	return ids
}
