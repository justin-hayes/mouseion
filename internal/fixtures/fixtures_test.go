package fixtures

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStoreMoveReadingJourneyEntryMutatesAndProtectsRevision(t *testing.T) {
	store := NewStore()
	ctx := context.Background()
	journey, err := store.GetReadingJourney(ctx, OwnerID, "it")
	require.NoError(t, err)
	revision, err := store.MoveReadingJourneyEntry(ctx, OwnerID, "it", edgeBookID, 1, journey.Revision)
	require.NoError(t, err, "move revision=%d err=%v", revision, err)
	assert.Equal(t, journey.Revision+1, revision, "move revision=%d err=%v", revision, err)
	journey, err = store.GetReadingJourney(ctx, OwnerID, "it")
	require.NoError(t, err)
	require.Len(t, journey.Entries, 3, "reordered journey=%+v", journey.Entries)
	assert.Equal(t, edgeBookID, journey.Entries[0].BookID, "reordered journey=%+v", journey.Entries)
	assert.Equal(t, 1, journey.Entries[0].Position, "reordered journey=%+v", journey.Entries)
	assert.Equal(t, 2, journey.Entries[1].Position, "reordered journey=%+v", journey.Entries)
	_, err = store.MoveReadingJourneyEntry(ctx, OwnerID, "it", "fixture-empty", 1, revision-1)
	assert.ErrorIs(t, err, persistence.ErrJourneyStale, "stale move error=%v", err) //nolint:testifylint // Stale-write classification and the following clamped move are independent.
	unchanged, err := store.MoveReadingJourneyEntry(ctx, OwnerID, "it", edgeBookID, 0, revision)
	require.NoError(t, err, "clamped move revision=%d err=%v", unchanged, err)
	assert.Equal(t, revision, unchanged, "clamped move revision=%d err=%v", unchanged, err)
}

func TestFixtureGetBookDetailResolvesBookAndSourceIDs(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	store.books = append(store.books, domain.SourceMaterialSummary{
		Source:         domain.SourceMaterial{ID: "fixture-detail-source", OwnerID: OwnerID, Language: "de", Title: "Distinct fixture identities"},
		BookID:         "fixture-detail-book",
		AnalysisStatus: "not analyzed",
	})

	for _, id := range []string{BookID, SourceID, "fixture-metadata-only", "fixture-detail-book", "fixture-detail-source"} {
		detail, err := store.GetBookDetail(ctx, OwnerID, id)
		require.NoError(t, err, "GetBookDetail(%q): %v", id, err)
		if id == SourceID || id == "fixture-detail-source" {
			wantBookID := BookID
			if id == "fixture-detail-source" {
				wantBookID = "fixture-detail-book"
			}
			assert.Equal(t, wantBookID, detail.Book.ID, "source resolution=%+v", detail)
			require.NotNil(t, detail.Acquired, "source resolution=%+v", detail)
		} else {
			assert.Equal(t, id, detail.Book.ID, "GetBookDetail(%q) book=%q", id, detail.Book.ID)
		}
	}

	detail, err := store.GetBookDetail(ctx, OwnerID, BookID)
	require.NoError(t, err, "acquired detail=%+v err=%v", detail, err)
	require.NotNil(t, detail.Acquired, "acquired detail=%+v err=%v", detail, err)
	assert.Equal(t, domain.BookAnalyzed, detail.EvidenceState(), "acquired detail=%+v err=%v", detail, err)
	_, err = store.GetBookDetail(ctx, "other-owner", BookID)
	assert.ErrorIs(t, err, persistence.ErrNotFound, "cross-owner detail error=%v", err) //nolint:testifylint // Cross-owner and unknown-detail lookups are independent cases.
	_, err = store.GetBookDetail(ctx, OwnerID, "unknown")
	assert.ErrorIs(t, err, persistence.ErrNotFound, "unknown detail error=%v", err)
}

func TestFixtureJourneyProjectionCoversComparisonStatesDeterministically(t *testing.T) {
	store := NewStore()
	result, err := (Insights{JourneyStore: store}).JourneyProjection(context.Background(), OwnerID, "de")
	require.NoError(t, err)
	wantLearner := []string{BookID, "fixture-failed", routeMatchBookID, routeDiffersBookID, routeTieABookID, routeTieBBookID, routeUnavailableBookID}
	wantAdvisory := []string{BookID, "fixture-failed", routeMatchBookID, routeTieABookID, routeTieBBookID, routeDiffersBookID, routeUnavailableBookID}
	require.Len(t, result.LearnerOrder, len(wantLearner))
	for i, want := range wantLearner {
		assert.Equal(t, want, result.LearnerOrder[i].BookID, "learner order[%d]=%q, want %q", i, result.LearnerOrder[i].BookID, want)
	}
	require.Len(t, result.AdvisoryOrder, len(wantAdvisory))
	for i, want := range wantAdvisory {
		assert.Equal(t, want, result.AdvisoryOrder[i].BookID, "advisory order[%d]=%q, want %q", i, result.AdvisoryOrder[i].BookID, want)
	}
	assert.Equal(t, 4, result.ComparableCount, "comparison counts=%d/%d", result.ComparableCount, result.IncomparableCount)
	assert.Equal(t, 3, result.IncomparableCount, "comparison counts=%d/%d", result.ComparableCount, result.IncomparableCount)
	require.Len(t, result.AdvisoryOrder, 7)
	assert.Nil(t, result.AdvisoryOrder[0].Rank, "incomparable ranks=%+v", result.AdvisoryOrder)
	assert.Nil(t, result.AdvisoryOrder[6].Rank, "incomparable ranks=%+v", result.AdvisoryOrder)
	require.NotNil(t, result.AdvisoryOrder[3].Coverage)
	require.NotNil(t, result.AdvisoryOrder[4].Coverage)
	assert.Equal(t, result.AdvisoryOrder[3].Coverage.KnownTokenCount, result.AdvisoryOrder[4].Coverage.KnownTokenCount, "tie coverage=%+v", result.AdvisoryOrder)
	require.Len(t, result.ConditionalAdvisoryOrder, len(result.AdvisoryOrder), "conditional order=%+v", result.ConditionalAdvisoryOrder)
	assert.Equal(t, routeDiffersBookID, result.ConditionalAdvisoryOrder[2].BookID, "conditional order did not differ=%+v", result.ConditionalAdvisoryOrder)

	journey, err := store.GetReadingJourney(context.Background(), OwnerID, "de")
	require.NoError(t, err)
	_, err = store.MoveReadingJourneyEntry(context.Background(), OwnerID, "de", routeDiffersBookID, 1, journey.Revision)
	require.NoError(t, err)
	result, err = (Insights{JourneyStore: store}).JourneyProjection(context.Background(), OwnerID, "de")
	require.NoError(t, err, "projection did not follow canonical fixture order=%+v err=%v", result.LearnerOrder, err)
	require.Len(t, result.LearnerOrder, len(wantLearner))
	assert.Equal(t, routeDiffersBookID, result.LearnerOrder[1].BookID, "projection did not follow canonical fixture order=%+v err=%v", result.LearnerOrder, err)
}

func TestFixtureItalianJourneyProjectionUsesItalianEvidenceIdentity(t *testing.T) {
	result, err := (Insights{JourneyStore: NewStore()}).JourneyProjection(context.Background(), OwnerID, "it")
	require.NoError(t, err)
	assert.Equal(t, "it", result.Language, "Italian projection language=%q", result.Language)
	require.Len(t, result.LearnerOrder, 3, "Italian projection order=%+v", result.LearnerOrder)
	for _, book := range result.LearnerOrder {
		assert.Equal(t, "it", book.Language, "Italian projection book=%+v", book)
		assert.NotContains(t, book.IncomparableReason, "different study language", "Italian projection retained cross-language reason=%q", book.IncomparableReason)
	}
}

func TestStoreStudyLanguagesDeriveFromFixtureBooks(t *testing.T) {
	store := NewStore()
	store.supported = []domain.SupportedLanguage{{Language: " DE ", DisplayName: "German"}}
	store.books = []domain.SourceMaterialSummary{
		{Source: domain.SourceMaterial{OwnerID: OwnerID, Language: " DE "}},
		{Source: domain.SourceMaterial{OwnerID: OwnerID, Language: "it_IT"}},
		{Source: domain.SourceMaterial{OwnerID: "another-owner", Language: "fr"}},
	}
	store.myBooks = []domain.MyBook{
		{Book: domain.Book{OwnerID: OwnerID, LanguageState: domain.LanguageChosen, LanguageTag: "pt-BR"}},
		{Book: domain.Book{OwnerID: OwnerID, LanguageState: domain.LanguageUnknown, LanguageTag: ""}},
		{Book: domain.Book{OwnerID: "another-owner", LanguageState: domain.LanguageChosen, LanguageTag: "nl"}},
	}

	languages, err := store.ListStudyLanguages(context.Background(), OwnerID)
	require.NoError(t, err)
	want := []domain.StudyLanguage{
		{Language: "de", DisplayName: "German"},
		{Language: "it", DisplayName: "it"},
		{Language: "pt", DisplayName: "pt"},
	}
	require.Len(t, languages, len(want), "study languages=%+v, want %+v", languages, want)
	for i := range want {
		assert.Equal(t, want[i], languages[i], "study language[%d]=%+v, want %+v", i, languages[i], want[i])
	}
}

func TestStoreActiveStudyLanguageIsStoredAndNullable(t *testing.T) {
	store := NewStore()
	ctx := context.Background()
	got, err := store.GetStoredActiveStudyLanguage(ctx, OwnerID)
	require.NoError(t, err, "initial active language=%q err=%v", got, err)
	assert.Equal(t, "de", got, "initial active language=%q err=%v", got, err)
	err = store.SetActiveStudyLanguage(ctx, OwnerID, "IT_it")
	require.NoError(t, err)
	got, err = store.GetStoredActiveStudyLanguage(ctx, OwnerID)
	require.NoError(t, err, "stored active language=%q err=%v", got, err)
	assert.Equal(t, "it", got, "stored active language=%q err=%v", got, err)
	err = store.SetActiveStudyLanguage(ctx, OwnerID, "")
	require.NoError(t, err)
	got, err = store.GetStoredActiveStudyLanguage(ctx, OwnerID)
	require.NoError(t, err, "cleared active language=%q err=%v", got, err)
	assert.Empty(t, got, "cleared active language=%q err=%v", got, err)
}

func TestFixtureCatalogueSyncAddsPassiveStudyLanguageArrival(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	sync := NewCatalogueSync(store)

	got, err := store.GetStoredActiveStudyLanguage(ctx, OwnerID)
	require.NoError(t, err, "initial active language=%q err=%v", got, err)
	assert.Equal(t, "de", got, "initial active language=%q err=%v", got, err)
	recent, err := store.MostRecentlyActivatedStudyLanguage(ctx, OwnerID)
	require.NoError(t, err, "initial recent language=%q err=%v", recent, err)
	assert.Equal(t, "it", recent, "initial recent language=%q err=%v", recent, err)

	_, err = sync.Enqueue(ctx, OwnerID, "fixture-connection")
	require.NoError(t, err)

	languages, err := store.ListStudyLanguages(ctx, OwnerID)
	require.NoError(t, err)
	var foundSpanish bool
	for _, language := range languages {
		if language.Language == "es" {
			foundSpanish = true
			break
		}
	}
	assert.True(t, foundSpanish, "study languages after sync=%+v, want Spanish arrival", languages)
	recent, err = store.MostRecentlyActivatedStudyLanguage(ctx, OwnerID)
	require.NoError(t, err, "recent language after sync=%q err=%v", recent, err)
	assert.Equal(t, "es", recent, "recent language after sync=%q err=%v", recent, err)
	got, err = store.GetStoredActiveStudyLanguage(ctx, OwnerID)
	require.NoError(t, err, "active language changed after sync=%q err=%v", got, err)
	assert.Equal(t, "de", got, "active language changed after sync=%q err=%v", got, err)
}

func TestStoreKnownVocabularyOnlyLanguageRemainsViewable(t *testing.T) {
	store := NewStore()
	err := store.SetActiveStudyLanguage(context.Background(), OwnerID, "fr")
	require.NoError(t, err)
	known, err := store.ListKnownVocabulary(context.Background(), OwnerID, "fr")
	require.NoError(t, err, "known-only vocabulary=%+v err=%v", known, err)
	require.Len(t, known, 1, "known-only vocabulary=%+v err=%v", known, err)
	assert.Equal(t, "bonjour", known[0].CanonicalLemma, "known-only vocabulary=%+v err=%v", known, err)
	languages, err := store.ListStudyLanguages(context.Background(), OwnerID)
	require.NoError(t, err, "study languages=%+v err=%v", languages, err)
	assert.Len(t, languages, 2, "study languages=%+v err=%v", languages, err)
}

func TestStoreMyBooksBrowseUsesCanonicalLanguageIdentity(t *testing.T) {
	store := NewStore()
	store.books = nil
	store.myBooks = []domain.MyBook{
		{Book: domain.Book{ID: "regional", OwnerID: OwnerID, Title: "Regional", LanguageState: domain.LanguageChosen, LanguageTag: "de-DE"}},
	}
	result, err := store.ListMyBooksBrowse(context.Background(), OwnerID, "", "DE_de", 0, 10)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Total, "canonical browse result=%+v", result)
	require.Len(t, result.Items, 1, "canonical browse result=%+v", result)
	require.Len(t, result.Counts, 1, "canonical browse result=%+v", result)
	assert.Equal(t, "de", result.Counts[0].Tag, "canonical browse result=%+v", result)
}

func TestStoreConcurrentJourneyMovesAcceptOnlyOneRevision(t *testing.T) {
	store := NewStore()
	ctx := context.Background()
	journey, err := store.GetReadingJourney(ctx, OwnerID, "it")
	require.NoError(t, err)
	var wait sync.WaitGroup
	results := make(chan error, 2)
	wait.Add(2)
	for range 2 {
		go func() {
			defer wait.Done()
			_, moveErr := store.MoveReadingJourneyEntry(ctx, OwnerID, "it", edgeBookID, 1, journey.Revision)
			results <- moveErr
		}()
	}
	wait.Wait()
	close(results)
	var successes, stale int
	for moveErr := range results {
		switch {
		case moveErr == nil:
			successes++
		case errors.Is(moveErr, persistence.ErrJourneyStale):
			stale++
		default:
			require.Failf(t, "concurrent move produced an unexpected error", "concurrent move error=%v", moveErr)
		}
	}
	assert.Equal(t, 1, successes, "concurrent move results successes=%d stale=%d", successes, stale)
	assert.Equal(t, 1, stale, "concurrent move results successes=%d stale=%d", successes, stale)
}

func TestStorePrimaryGoalsAreIndependentByLanguageAndJourneyRemovalClearsOnlyThatLanguage(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	deGoal, err := store.GetPrimaryGoal(ctx, OwnerID, "de")
	require.NoError(t, err, "German Goal=%+v err=%v", deGoal, err)
	assert.Equal(t, BookID, deGoal.BookID, "German Goal=%+v err=%v", deGoal, err)
	itGoal, err := store.GetPrimaryGoal(ctx, OwnerID, "it")
	require.NoError(t, err, "Italian Goal=%+v err=%v", itGoal, err)
	assert.Equal(t, ItalianGoalBookID, itGoal.BookID, "Italian Goal=%+v err=%v", itGoal, err)

	err = store.ClearPrimaryGoal(ctx, OwnerID, "it", ItalianGoalBookID)
	require.NoError(t, err, "clear Italian Goal: %v", err)
	deGoal, err = store.GetPrimaryGoal(ctx, OwnerID, "de")
	require.NoError(t, err, "German Goal after Italian clear=%+v err=%v", deGoal, err)
	assert.Equal(t, BookID, deGoal.BookID, "German Goal after Italian clear=%+v err=%v", deGoal, err)
	itGoal, err = store.GetPrimaryGoal(ctx, OwnerID, "it")
	require.NoError(t, err, "Italian Goal after clear=%+v err=%v", itGoal, err)
	assert.Empty(t, itGoal.BookID, "Italian Goal after clear=%+v err=%v", itGoal, err)

	_, err = store.CreatePrimaryGoal(ctx, OwnerID, "it", ItalianGoalBookID)
	require.NoError(t, err, "recreate Italian Goal: %v", err)
	journey, err := store.GetReadingJourney(ctx, OwnerID, "it")
	require.NoError(t, err)
	_, err = store.RemoveFromReadingJourney(ctx, OwnerID, "it", ItalianGoalBookID, journey.Revision)
	require.NoError(t, err, "remove Italian Goal member: %v", err)
	itGoal, err = store.GetPrimaryGoal(ctx, OwnerID, "it")
	require.NoError(t, err, "Italian Goal after Journey removal=%+v err=%v", itGoal, err)
	assert.Empty(t, itGoal.BookID, "Italian Goal after Journey removal=%+v err=%v", itGoal, err)
	deGoal, err = store.GetPrimaryGoal(ctx, OwnerID, "de")
	require.NoError(t, err, "German Goal after Italian Journey removal=%+v err=%v", deGoal, err)
	assert.Equal(t, BookID, deGoal.BookID, "German Goal after Italian Journey removal=%+v err=%v", deGoal, err)
}

func TestStoreRetaggingGoalBookClearsItsLanguageGoal(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	_, err := store.UpdateBookMetadata(ctx, OwnerID, ItalianGoalBookID, "Una meta", domain.LanguageUnknown, "")
	require.NoError(t, err, "retag Italian Goal book: %v", err)
	goal, err := store.GetPrimaryGoal(ctx, OwnerID, "it")
	require.NoError(t, err, "Italian Goal after retag=%+v err=%v", goal, err)
	assert.Empty(t, goal.BookID, "Italian Goal after retag=%+v err=%v", goal, err)
	goal, err = store.GetPrimaryGoal(ctx, OwnerID, "de")
	require.NoError(t, err, "German Goal after Italian retag=%+v err=%v", goal, err)
	assert.Equal(t, BookID, goal.BookID, "German Goal after Italian retag=%+v err=%v", goal, err)
}

func TestFixtureReadModelKeepsCanonicalTitleSeparateFromAcquisitionTitle(t *testing.T) {
	store := NewStore()
	store.books[0].Source.Title = "Acquisition-internal title"
	store.books[0].BookTitle = "Refreshed catalogue title"

	books, err := store.ListMyBooksWithEvidence(context.Background(), OwnerID)
	require.NoError(t, err)
	for _, book := range books {
		if book.Book.ID != BookID {
			continue
		}
		assert.Equal(t, "Refreshed catalogue title", book.Book.Title, "fixture read model=%+v", book)
		require.NotNil(t, book.Acquired, "fixture read model=%+v", book)
		assert.Equal(t, "Refreshed catalogue title", book.Acquired.BookTitle, "fixture read model=%+v", book)
		assert.Equal(t, "Acquisition-internal title", book.Acquired.Source.Title, "fixture read model=%+v", book)
		return
	}
	require.Failf(t, "fixture book not found", "fixture book %q not found", BookID)
}

func TestMigrationFixturesPinLegacyAndKnownVocabularyCategories(t *testing.T) {
	store := NewStore()
	ctx := context.Background()

	legacy, err := store.ListUnattachedGeneratedVocabulary(ctx, OwnerID, "de")
	require.NoError(t, err, "legacy generated fixture=%+v err=%v", legacy, err)
	require.Len(t, legacy, 1, "legacy generated fixture=%+v err=%v", legacy, err)
	assert.Equal(t, LegacyGeneratedLemma, legacy[0].CanonicalLemma, "legacy generated fixture=%+v err=%v", legacy, err)
	assert.Nil(t, legacy[0].FirstSourceMaterialID, "legacy generated fixture=%+v err=%v", legacy, err)
	known, err := store.ListKnownVocabulary(ctx, OwnerID, "de")
	require.NoError(t, err)
	provenance := map[string]string{}
	for _, item := range known {
		provenance[item.CanonicalLemma] = item.Provenance
	}
	assert.Equal(t, "Explicitly recorded", provenance[IndependentKnownLemma], "known vocabulary provenance=%v", provenance)
	assert.Equal(t, "Graduated from reviewed deck", provenance[GraduatedKnownLemma], "known vocabulary provenance=%v", provenance)
	books, err := store.ListMyBooksWithEvidence(ctx, OwnerID)
	require.NoError(t, err)
	for _, book := range books {
		if book.Book.ID != "fixture-metadata-only" {
			continue
		}
		assert.Nil(t, book.Acquired, "metadata-only fixture acquired evidence=%+v", book)
		assert.Equal(t, domain.BookNotAcquired, book.EvidenceState(), "metadata-only fixture acquired evidence=%+v", book)
	}
}

func TestFixtureCatalogueAliasScopesRefreshAndAcquisition(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	sync := NewCatalogueSync(store)

	alias, err := store.GetBookCatalogEntryAlias(ctx, OwnerID, "fixture-metadata-only")
	require.NoError(t, err, "catalogue alias=%+v err=%v", alias, err)
	assert.Equal(t, "fixture-connection", alias.ConnectionID, "catalogue alias=%+v err=%v", alias, err)
	assert.Equal(t, "fixture-entry", alias.Value, "catalogue alias=%+v err=%v", alias, err)
	target, err := sync.FindAcquisitionTarget(ctx, OwnerID, "fixture-metadata-only")
	require.NoError(t, err)
	assert.Equal(t, alias.ConnectionID, target.ConnectionID, "acquisition target=%+v alias=%+v", target, alias)
	assert.Equal(t, alias.Value, target.Entry.ID, "acquisition target=%+v alias=%+v", target, alias)
	store.myBooks[0].Book.Title = "Stale metadata"
	result, err := sync.RefreshEntry(ctx, OwnerID, "fixture-metadata-only")
	require.NoError(t, err, "refresh result=%+v err=%v", result, err)
	assert.True(t, result.Updated, "refresh result=%+v err=%v", result, err)
	assert.Equal(t, "Metadata-only migration book", result.Book.Title, "refresh result=%+v err=%v", result, err)

	store.aliases[0].ConnectionID = "fixture-failed-connection"
	_, err = sync.FindAcquisitionTarget(ctx, OwnerID, "fixture-metadata-only")
	assert.ErrorIs(t, err, cataloguesync.ErrNotFound, "wrong connection acquisition error=%v", err) //nolint:testifylint // Acquisition failure and refresh result are independent checks.
	result, err = sync.RefreshEntry(ctx, OwnerID, "fixture-metadata-only")
	require.NoError(t, err, "wrong connection refresh=%+v err=%v", result, err)
	assert.True(t, result.Missing, "wrong connection refresh=%+v err=%v", result, err)
	assert.False(t, result.Failed, "wrong connection refresh=%+v err=%v", result, err)
}

func TestFixtureNeedsLanguageBookCannotJoinJourney(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	journey, err := store.GetReadingJourney(ctx, OwnerID, "de")
	require.NoError(t, err)
	_, err = store.AddToReadingJourney(ctx, OwnerID, "de", "fixture-metadata-only", journey.Revision)
	assert.ErrorIs(t, err, persistence.ErrBookLanguageRequired, "unknown-language Journey add error=%v", err) //nolint:testifylint // Rejection and unchanged-Journey lookup are independent expectations.
	unchanged, err := store.GetReadingJourney(ctx, OwnerID, "de")
	require.NoError(t, err, "unknown-language add changed Journey=%+v err=%v", unchanged, err)
	assert.Equal(t, journey.Revision, unchanged.Revision, "unknown-language add changed Journey=%+v err=%v", unchanged, err)
}

func TestFixtureReservedVocabularyFollowsPrimaryGoalSnapshot(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	reserved, err := store.ListReservedVocabulary(ctx, OwnerID, "de")
	require.NoError(t, err, "reserved before study=%+v err=%v", reserved, err)
	assert.Len(t, reserved, 2, "reserved from active Goal=%+v err=%v", reserved, err)
	_, err = store.StartDeckVocabularyStudy(ctx, OwnerID, PrepID)
	require.NoError(t, err)
	reserved, err = store.ListReservedVocabulary(ctx, OwnerID, "de")
	require.NoError(t, err, "reserved during study=%+v err=%v", reserved, err)
	require.Len(t, reserved, 2, "reserved during study=%+v err=%v", reserved, err)
	got := map[string]bool{}
	for _, item := range reserved {
		got[item.CanonicalLemma] = true
	}
	assert.True(t, got["gehen"], "reserved identities=%v", got)
	assert.True(t, got["Weg"], "reserved identities=%v", got)
	_, err = store.ConfirmDeckVocabularyReview(ctx, OwnerID, PrepID)
	require.NoError(t, err)
	released, err := store.ListReservedVocabulary(ctx, OwnerID, "de")
	require.NoError(t, err, "reserved after review=%+v err=%v", released, err)
	assert.Len(t, released, 2, "active Goal snapshot survived deck review=%+v err=%v", released, err)
	err = store.ClearPrimaryGoal(ctx, OwnerID, "de", BookID)
	require.NoError(t, err)
	released, err = store.ListReservedVocabulary(ctx, OwnerID, "de")
	require.NoError(t, err)
	assert.Empty(t, released, "reserved after Goal clear=%+v err=%v", released, err)
}

func TestFixtureCatalogueSyncAdmitsNeedsLanguageBook(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	sync := NewCatalogueSync(store)
	before, err := store.ListMyBooksBrowse(ctx, OwnerID, "", domain.LanguageUnknown, 0, 25)
	require.NoError(t, err, "initial needs-language browse=%+v err=%v", before, err)
	assert.GreaterOrEqual(t, before.Total, 1, "initial needs-language browse=%+v err=%v", before, err)
	_, err = sync.Enqueue(ctx, OwnerID, "fixture-browser-sync-connection")
	require.NoError(t, err)
	afterUnknown, err := store.ListMyBooksBrowse(ctx, OwnerID, "", domain.LanguageUnknown, 0, 25)
	require.NoError(t, err, "needs-language book remained after sync=%+v err=%v", afterUnknown, err)
	assert.Equal(t, before.Total-1, afterUnknown.Total, "needs-language book remained after sync=%+v err=%v", afterUnknown, err)
	afterGerman, err := store.ListMyBooksBrowse(ctx, OwnerID, "", "de", 0, 25)
	require.NoError(t, err, "re-synced book missing from German browse=%+v err=%v", afterGerman, err)
	assert.NotZero(t, afterGerman.Total, "re-synced book missing from German browse=%+v err=%v", afterGerman, err)
	found := false
	for _, book := range afterGerman.Items {
		if book.Book.ID == BrowserSyncBookID {
			found = true
			break
		}
	}
	assert.True(t, found, "re-synced metadata book missing from German page=%+v", afterGerman.Items)
}
