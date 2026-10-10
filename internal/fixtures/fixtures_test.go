package fixtures

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFixtureGetBookDetailResolvesBookAndSourceIDs(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	store.books = append(store.books, domain.SourceMaterialSummary{
		Source:  domain.SourceMaterial{ID: "fixture-detail-source", OwnerID: OwnerID, Language: "de", Title: "Distinct fixture identities"},
		BookID:  "fixture-detail-book",
		Signals: notAnalyzedSignals,
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
	result, err := store.ListMyBooksBrowse(context.Background(), OwnerID, "", "DE_de", "", false, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Total, "canonical browse result=%+v", result)
	require.Len(t, result.Items, 1, "canonical browse result=%+v", result)
	require.Len(t, result.Counts, 1, "canonical browse result=%+v", result)
	assert.Equal(t, "de", result.Counts[0].Tag, "canonical browse result=%+v", result)
}

func TestStoreMyBooksBrowseUsesCurrentReadingWorkflowBucket(t *testing.T) {
	store := NewStore()
	toRead, err := store.ListMyBooksBrowse(context.Background(), OwnerID, "", "de", string(domain.BookDispositionToRead), false, 0, 25)
	require.NoError(t, err)
	assert.Contains(t, fixtureBookIDs(toRead.Items), BookID, "current reading belongs in To Read")
	// Nine German To Read Books plus the five Analysis evidence situations.
	assert.Equal(t, 14, dispositionCount(toRead.DispositionCounts, domain.BookDispositionToRead))
	all, err := store.ListMyBooksBrowse(context.Background(), OwnerID, "", "de", "", false, 0, 25)
	require.NoError(t, err)
	var current domain.MyBook
	for _, book := range all.Items {
		if book.Book.ID == BookID {
			current = book
		}
	}
	assert.True(t, current.IsCurrentReading)
	assert.Equal(t, domain.MyBookBucketCurrentReading, current.WorkflowBucket())
}

func TestUnresolvedLemmaReviewFlagBlocksStartingFixtureBook(t *testing.T) {
	store := NewStore()
	current, err := store.GetCurrentReading(t.Context(), OwnerID, "de")
	require.NoError(t, err)
	require.NoError(t, store.EndCurrentReading(t.Context(), OwnerID, "de", current.BookID, current.SnapshotID))
	store.dispositions[fixtureDispositionKey(OwnerID, LemmaFlagBookID)] = domain.BookDispositionToRead
	_, err = store.StartCurrentReading(t.Context(), OwnerID, "de", LemmaFlagBookID)
	assert.ErrorIs(t, err, persistence.ErrUnresolvedLemmaReviewFlags)
}

func TestLemmaDecisionProposalIsRejectedForTheCurrentReadingBook(t *testing.T) {
	store := NewStore()
	current, err := store.GetCurrentReading(t.Context(), OwnerID, "de")
	require.NoError(t, err)
	require.Equal(t, BookID, current.BookID, "the fixture seeds BookID as the German current reading")
	occurrences, err := store.ListLemmaReviewOccurrences(t.Context(), OwnerID, BookID, "Weg")
	require.NoError(t, err)
	decisions := []domain.LemmaReviewDecision{{Occurrence: occurrences[0], CanonicalLemma: "pfad", NormalizationProfile: "german-post-1996", NormalizationVersion: "6"}}
	err = store.PutLemmaDecisionProposal(t.Context(), decisions, "Weg", "de", nil, "any-fingerprint")
	require.ErrorIs(t, err, persistence.ErrLemmaDecisionCurrentReading, "the gate runs before the preview fingerprint")
	err = store.PutLemmaDecisions(t.Context(), decisions)
	require.ErrorIs(t, err, persistence.ErrNotFound, "the single-decision path keeps its not-found contract")
	unchanged, err := store.ListLemmaReviewOccurrences(t.Context(), OwnerID, BookID, "Weg")
	require.NoError(t, err)
	assert.Empty(t, unchanged[0].CorrectedLemma, "a rejected decision persists nothing")
}

func TestStartingFixtureBookRejectsMalformedAndUnknownIDsAsNotFound(t *testing.T) {
	store := NewStore()
	for _, bookID := range []string{"not-a-uuid", "00000000-0000-0000-0000-000000000000", "' OR 1=1 --"} {
		_, err := store.StartCurrentReading(t.Context(), OwnerID, "de", bookID)
		require.ErrorIs(t, err, persistence.ErrNotFound, bookID)
	}
	_, err := store.StartCurrentReading(t.Context(), "not-a-uuid", "de", BookID)
	assert.ErrorIs(t, err, persistence.ErrNotFound, "malformed owner")
}

func TestStartingFixtureBookReportsAlreadyCurrentBeforeLemmaReviewFlags(t *testing.T) {
	store := NewStore()
	current, err := store.GetCurrentReading(t.Context(), OwnerID, "de")
	require.NoError(t, err)
	require.True(t, current.IsActive(), "the fixture seeds a German current reading")
	_, err = store.StartCurrentReading(t.Context(), OwnerID, "de", LemmaFlagBookID)
	assert.ErrorIs(t, err, persistence.ErrCurrentReadingExists)
}

func fixtureBookIDs(books []domain.MyBook) []string {
	ids := make([]string, 0, len(books))
	for _, book := range books {
		ids = append(ids, book.Book.ID)
	}
	return ids
}

func dispositionCount(counts []persistence.DispositionCount, disposition domain.BookDisposition) int {
	for _, count := range counts {
		if count.Disposition == disposition {
			return count.Count
		}
	}
	return 0
}

func TestStoreCurrentReadingIsIndependentByLanguage(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	deGoal, err := store.GetCurrentReading(ctx, OwnerID, "de")
	require.NoError(t, err, "German Goal=%+v err=%v", deGoal, err)
	assert.Equal(t, BookID, deGoal.BookID, "German Goal=%+v err=%v", deGoal, err)
	itGoal, err := store.GetCurrentReading(ctx, OwnerID, "it")
	require.NoError(t, err, "Italian Goal=%+v err=%v", itGoal, err)
	assert.Equal(t, ItalianGoalBookID, itGoal.BookID, "Italian Goal=%+v err=%v", itGoal, err)

	italianReading, err := store.GetCurrentReading(ctx, OwnerID, "it")
	require.NoError(t, err)
	err = store.EndCurrentReading(ctx, OwnerID, "it", ItalianGoalBookID, italianReading.SnapshotID)
	require.NoError(t, err, "end Italian reading: %v", err)
	deGoal, err = store.GetCurrentReading(ctx, OwnerID, "de")
	require.NoError(t, err, "German Goal after Italian clear=%+v err=%v", deGoal, err)
	assert.Equal(t, BookID, deGoal.BookID, "German Goal after Italian clear=%+v err=%v", deGoal, err)
	itGoal, err = store.GetCurrentReading(ctx, OwnerID, "it")
	require.NoError(t, err, "Italian Goal after clear=%+v err=%v", itGoal, err)
	assert.Empty(t, itGoal.BookID, "Italian Goal after clear=%+v err=%v", itGoal, err)

}

func TestStoreCurrentReadingLifecycleUsesExistingGoalBehavior(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	reading, err := store.GetCurrentReading(ctx, OwnerID, "de")
	require.NoError(t, err)
	assert.Equal(t, BookID, reading.BookID)
	assert.Equal(t, "fixture-de-goal-snapshot", reading.SnapshotID)
	reserved, err := store.ListReservedVocabulary(ctx, OwnerID, "de")
	require.NoError(t, err)
	assert.Len(t, reserved, 2)

	_, err = store.SwitchCurrentReading(ctx, OwnerID, "de", ItalianGoalBookID, "stale-book", reading.SnapshotID)
	require.ErrorIs(t, err, persistence.ErrCurrentReadingStale)
	require.NoError(t, store.EndCurrentReading(ctx, OwnerID, "de", BookID, reading.SnapshotID))
	reserved, err = store.ListReservedVocabulary(ctx, OwnerID, "de")
	require.NoError(t, err)
	assert.Empty(t, reserved)
	restarted, err := store.StartCurrentReading(ctx, OwnerID, "de", BookID)
	require.NoError(t, err)
	assert.NotEqual(t, reading.SnapshotID, restarted.SnapshotID, "starting Reading again freezes a fresh fixture snapshot")
	reserved, err = store.ListReservedVocabulary(ctx, OwnerID, "de")
	require.NoError(t, err)
	assert.Len(t, reserved, 2)

	reading, err = store.GetCurrentReading(ctx, OwnerID, "it")
	require.NoError(t, err)
	require.NoError(t, store.EndCurrentReading(ctx, OwnerID, "it", ItalianGoalBookID, reading.SnapshotID))
	reading, err = store.GetCurrentReading(ctx, OwnerID, "it")
	require.NoError(t, err)
	assert.False(t, reading.IsActive())

	reading, err = store.StartCurrentReading(ctx, OwnerID, "it", ItalianGoalBookID)
	require.NoError(t, err)
	assert.Equal(t, ItalianGoalBookID, reading.BookID)

	_, err = store.FinishCurrentReading(ctx, OwnerID, "it", "wrong-book", reading.SnapshotID)
	require.ErrorIs(t, err, persistence.ErrCurrentReadingStale)
	finished, err := store.FinishCurrentReading(ctx, OwnerID, "it", reading.BookID, reading.SnapshotID)
	require.NoError(t, err)
	replayed, err := store.FinishCurrentReading(ctx, OwnerID, "it", reading.BookID, reading.SnapshotID)
	require.NoError(t, err)
	assert.Equal(t, finished, replayed)
}

func TestFixtureToReadBooksExposeDeterministicCoverStates(t *testing.T) {
	store := NewStore()
	books, err := store.ListMyBooksWithEvidence(context.Background(), OwnerID)
	require.NoError(t, err)

	want := map[string]string{
		BookID:             domain.BookCoverAvailable,
		"fixture-failed":   domain.BookCoverPending,
		routeMatchBookID:   domain.BookCoverAvailable,
		routeDiffersBookID: domain.BookCoverNone,
		routeTieABookID:    domain.BookCoverUnavailable,
		routeTieBBookID:    domain.BookCoverPending,
		ItalianGoalBookID:  domain.BookCoverAvailable,
		"fixture-empty":    domain.BookCoverNone,
	}
	for _, book := range books {
		if expected, ok := want[book.Book.ID]; ok {
			assert.Equal(t, expected, book.Cover.State, book.Book.ID)
		}
	}
}

func TestFixtureGoalPreparationUsesExactSnapshotAndEmptyGoalsNeedNoDeck(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	goal, err := store.GetCurrentReading(ctx, OwnerID, "de")
	require.NoError(t, err)
	assert.Equal(t, 2, goal.SnapshotSize)
	preparation, err := store.GetDeckPreparationForSnapshot(ctx, OwnerID, goal.SnapshotID)
	require.NoError(t, err)
	assert.Equal(t, goal.SnapshotID, preparation.SnapshotID)

	emptyGoal, err := store.GetCurrentReading(ctx, OwnerID, "it")
	require.NoError(t, err)
	assert.Zero(t, emptyGoal.SnapshotSize)
	_, err = store.GetDeckPreparationForSnapshot(ctx, OwnerID, emptyGoal.SnapshotID)
	require.ErrorIs(t, err, persistence.ErrNotFound)

	handle, err := (PreparedDeck{Store: store}).SubmitForGoal(ctx, OwnerID, ResultRunID, "fresh-goal-snapshot")
	require.NoError(t, err)
	assert.Equal(t, "fresh-goal-snapshot", handle.Preparation.SnapshotID)
	preparation, err = store.GetDeckPreparationForSnapshot(ctx, OwnerID, "fresh-goal-snapshot")
	require.NoError(t, err)
	assert.Equal(t, handle.Preparation.ID, preparation.ID)
}

func TestStoreReadingCompletionHistoryAllowsFutureCompletionOfSameBook(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	firstGoal, err := store.GetCurrentReading(ctx, OwnerID, "de")
	require.NoError(t, err)
	first, err := store.FinishCurrentReading(ctx, OwnerID, "de", BookID, firstGoal.SnapshotID)
	require.NoError(t, err)

	require.NoError(t, store.SetBookDisposition(ctx, OwnerID, BookID, domain.BookDispositionToRead))
	secondGoal, err := store.StartCurrentReading(ctx, OwnerID, "de", BookID)
	require.NoError(t, err)
	assert.NotEqual(t, firstGoal.SnapshotID, secondGoal.SnapshotID)
	second, err := store.FinishCurrentReading(ctx, OwnerID, "de", BookID, secondGoal.SnapshotID)
	require.NoError(t, err)
	assert.NotEqual(t, first.Completion.CompletedAt, second.Completion.CompletedAt)

	retry, err := store.FinishCurrentReading(ctx, OwnerID, "de", BookID, firstGoal.SnapshotID)
	require.NoError(t, err)
	assert.Equal(t, first.Completion, retry.Completion)
}

func TestStoreRetaggingGoalBookClearsItsLanguageGoal(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	_, err := store.UpdateBookMetadata(ctx, OwnerID, ItalianGoalBookID, "Una meta", "", domain.LanguageUnknown, "")
	require.NoError(t, err, "retag Italian Goal book: %v", err)
	goal, err := store.GetCurrentReading(ctx, OwnerID, "it")
	require.NoError(t, err, "Italian Goal after retag=%+v err=%v", goal, err)
	assert.Empty(t, goal.BookID, "Italian Goal after retag=%+v err=%v", goal, err)
	goal, err = store.GetCurrentReading(ctx, OwnerID, "de")
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
	knownLemmas := map[string]bool{}
	for _, item := range known {
		knownLemmas[item.CanonicalLemma] = true
	}
	assert.True(t, knownLemmas[IndependentKnownLemma], "known vocabulary=%v", known)
	assert.True(t, knownLemmas[GraduatedKnownLemma], "known vocabulary=%v", known)
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

func TestFixtureReservedVocabularyFollowsCurrentReadingSnapshot(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	reserved, err := store.ListReservedVocabulary(ctx, OwnerID, "de")
	require.NoError(t, err, "reserved from active Goal=%+v err=%v", reserved, err)
	require.Len(t, reserved, 2, "reserved from active Goal=%+v err=%v", reserved, err)
	got := map[string]bool{}
	for _, item := range reserved {
		got[item.CanonicalLemma] = true
	}
	assert.True(t, got["gehen"], "reserved identities=%v", got)
	assert.True(t, got["Weg"], "reserved identities=%v", got)
	current, err := store.GetCurrentReading(ctx, OwnerID, "de")
	require.NoError(t, err)
	err = store.EndCurrentReading(ctx, OwnerID, "de", BookID, current.SnapshotID)
	require.NoError(t, err)
	released, err := store.ListReservedVocabulary(ctx, OwnerID, "de")
	require.NoError(t, err)
	assert.Empty(t, released, "reserved after Goal clear=%+v err=%v", released, err)
}

func TestFixtureCatalogueSyncAdmitsNeedsLanguageBook(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	sync := NewCatalogueSync(store)
	before, err := store.ListMyBooksBrowse(ctx, OwnerID, "", domain.LanguageUnknown, "", false, 0, 25)
	require.NoError(t, err, "initial needs-language browse=%+v err=%v", before, err)
	assert.GreaterOrEqual(t, before.Total, 1, "initial needs-language browse=%+v err=%v", before, err)
	_, err = sync.Enqueue(ctx, OwnerID, "fixture-browser-sync-connection")
	require.NoError(t, err)
	afterUnknown, err := store.ListMyBooksBrowse(ctx, OwnerID, "", domain.LanguageUnknown, "", false, 0, 25)
	require.NoError(t, err, "needs-language book remained after sync=%+v err=%v", afterUnknown, err)
	assert.Equal(t, before.Total-1, afterUnknown.Total, "needs-language book remained after sync=%+v err=%v", afterUnknown, err)
	afterGerman, err := store.ListMyBooksBrowse(ctx, OwnerID, "", "de", "", false, 0, 25)
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

func TestVocabularyBrowseScenariosSetTheWorkingDeskStates(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	query := domain.VocabularyBrowseQuery{Language: "de", CurrentBookID: BookID, ReadingBookID: BookID, Page: 1}
	browse := func(t *testing.T, query domain.VocabularyBrowseQuery) domain.VocabularyBrowsePage {
		t.Helper()
		page, err := store.ListVocabularyBrowsePage(ctx, OwnerID, "de", query)
		require.NoError(t, err)
		return page
	}

	page := browse(t, query)
	assert.Equal(t, int64(1), page.Total, "the default fixture still hides accounted-for identities")

	require.NoError(t, store.SetVocabularyBrowseScenario(VocabularyBrowseScenarioAccounted))
	page = browse(t, query)
	assert.Zero(t, page.Total)
	assert.Equal(t, int64(2), page.ScopedInventoryTotal, "accounted-for identities are scoped, not absent")
	assert.Empty(t, page.Rows)
	query.IncludeAll = true
	page = browse(t, query)
	require.Len(t, page.Rows, 2)
	assert.True(t, page.Rows[0].Known && page.Rows[0].InBookDeck, "haus is Known and in a Book deck")
	assert.True(t, page.Rows[1].Known && page.Rows[1].Reserved, "lesen is Known and Reserved")
	query.IncludeAll = false

	require.NoError(t, store.SetVocabularyBrowseScenario(VocabularyBrowseScenarioNoAnalysis))
	page = browse(t, query)
	require.Len(t, page.Books, 1)
	assert.False(t, page.Books[0].HasCurrentAnalysis)
	assert.Empty(t, page.Rows)

	require.NoError(t, store.SetVocabularyBrowseScenario(VocabularyBrowseScenarioNoVocabulary))
	page = browse(t, query)
	assert.True(t, page.Books[0].HasCurrentAnalysis)
	assert.Zero(t, page.InventoryTotal)

	require.NoError(t, store.SetVocabularyBrowseScenario(VocabularyBrowseScenarioCountsUpdating))
	assert.True(t, browse(t, query).BrowseCountsUpdating)

	require.NoError(t, store.SetVocabularyBrowseScenario(VocabularyBrowseScenarioCountsUnavailable))
	page = browse(t, query)
	assert.True(t, page.BrowseCountsUnavailable)
	assert.Empty(t, page.Rows, "unavailable counts must not leak rows")

	require.Error(t, store.SetVocabularyBrowseScenario("unlisted"))
	require.NoError(t, store.SetVocabularyBrowseScenario(VocabularyBrowseScenarioDefault))
	assert.Equal(t, int64(1), browse(t, query).Total, "the empty scenario restores the default fixture")
}

func TestStoreSwitchCurrentReadingDecidesFromSnapshotLifecycleAndFlags(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	store.dispositions[fixtureDispositionKey(OwnerID, LemmaFlagBookID)] = domain.BookDispositionToRead
	store.dispositions[fixtureDispositionKey(OwnerID, routeMatchBookID)] = domain.BookDispositionToRead

	reading, err := store.GetCurrentReading(ctx, OwnerID, "de")
	require.NoError(t, err)

	_, err = store.SwitchCurrentReading(ctx, OwnerID, "de", LemmaFlagBookID, reading.BookID, reading.SnapshotID)
	require.ErrorIs(t, err, persistence.ErrUnresolvedLemmaReviewFlags, "Switch blocks on unresolved flags as Postgres does")
	_, err = store.SwitchCurrentReading(ctx, OwnerID, "de", ItalianGoalBookID, reading.BookID, reading.SnapshotID)
	var ineligible persistence.CurrentReadingIneligibleError
	require.ErrorAs(t, err, &ineligible)
	assert.Equal(t, domain.CurrentReadingOtherLanguage, ineligible.Reason)

	switched, err := store.SwitchCurrentReading(ctx, OwnerID, "de", routeMatchBookID, reading.BookID, reading.SnapshotID)
	require.NoError(t, err)
	assert.Equal(t, routeMatchBookID, switched.BookID)
	replayed, err := store.SwitchCurrentReading(ctx, OwnerID, "de", routeMatchBookID, reading.BookID, reading.SnapshotID)
	require.NoError(t, err, "the switch that created the current snapshot replays")
	assert.Equal(t, switched.SnapshotID, replayed.SnapshotID)

	require.NoError(t, store.EndCurrentReading(ctx, OwnerID, "de", switched.BookID, switched.SnapshotID))
	require.NoError(t, store.EndCurrentReading(ctx, OwnerID, "de", switched.BookID, switched.SnapshotID), "End replays from the released snapshot")
	require.ErrorIs(t, store.EndCurrentReading(ctx, OwnerID, "de", switched.BookID, reading.SnapshotID), persistence.ErrCurrentReadingStale, "a released snapshot is no End replay for another Book")

	restarted, err := store.StartCurrentReading(ctx, OwnerID, "de", switched.BookID)
	require.NoError(t, err)
	finished, err := store.FinishCurrentReading(ctx, OwnerID, "de", restarted.BookID, restarted.SnapshotID)
	require.NoError(t, err)
	assert.Equal(t, restarted.SnapshotID, finished.Completion.SnapshotID)
	require.ErrorIs(t, store.EndCurrentReading(ctx, OwnerID, "de", restarted.BookID, restarted.SnapshotID), persistence.ErrCurrentReadingStale, "a completed snapshot is never an End replay")
}
