package fixtures

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/cataloguesync"
	"github.com/justin-hayes/mouseion/internal/domain"
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
	store.dispositions[fixtureDispositionKey(OwnerID, "fixture-detail-book")] = domain.BookDispositionToRead

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
	missingDetails := []struct {
		name   string
		owner  string
		bookID string
	}{
		{"cross-owner detail is not found", "other-owner", BookID},
		{"unknown detail is not found", OwnerID, "unknown"},
	}
	for _, tc := range missingDetails {
		t.Run(tc.name, func(t *testing.T) {
			_, err := store.GetBookDetail(ctx, tc.owner, tc.bookID)
			require.ErrorIs(t, err, domain.ErrNotFound)
		})
	}
}

func TestFixtureConcordanceLemmaMatchSkipsOtherLemmasAndExcludedRows(t *testing.T) {
	store := NewStore()
	result, err := store.ListVocabularyConcordance(context.Background(), OwnerID, "de", domain.ConcordanceLookup{Term: "Haus", Page: 1})
	require.NoError(t, err)
	assert.Equal(t, domain.ConcordanceMatchLemma, result.Match)
	// Of the 27 fixture rows, one resolves to another lemma and one is excluded,
	// leaving 25 occurrences that fit on the first page.
	assert.Len(t, result.Occurrences, 25)
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

func TestStoreStudyProgressOnlyLanguageRemainsSelectable(t *testing.T) {
	store := NewStore()
	progress, err := store.ListStudyProgressLanguages(context.Background(), OwnerID)
	require.NoError(t, err)
	require.Len(t, progress, 1, "study progress languages=%+v", progress)
	assert.Equal(t, "fr", progress[0].Language)
	languages, err := store.ListStudyLanguages(context.Background(), OwnerID)
	require.NoError(t, err)
	for _, language := range languages {
		assert.NotEqual(t, "fr", language.Language, "French has no Book")
	}
}

func TestStoreMyBooksBrowseUsesCanonicalLanguageIdentity(t *testing.T) {
	store := NewStore()
	store.books = nil
	store.myBooks = []domain.MyBook{
		{Book: domain.Book{ID: "regional", OwnerID: OwnerID, Title: "Regional", LanguageState: domain.LanguageChosen, LanguageTag: "de-DE"}, Disposition: domain.BookDispositionToRead},
	}
	store.dispositions[fixtureDispositionKey(OwnerID, "regional")] = domain.BookDispositionToRead
	result, err := store.ListMyBooksBrowse(context.Background(), OwnerID, "", "DE_de", "", false, 0, 10)
	require.NoError(t, err)
	assert.Equal(t, 1, result.Total, "canonical browse result=%+v", result)
	require.Len(t, result.Items, 1, "canonical browse result=%+v", result)
	assert.Equal(t, "de", normalizeFixtureLanguage(result.Items[0].Book.LanguageTag), "canonical browse result=%+v", result)
}

func TestStoreMyBooksBrowseUsesCurrentReadingWorkflowBucket(t *testing.T) {
	store := NewStore()
	toRead, err := store.ListMyBooksBrowse(context.Background(), OwnerID, "", "de", string(domain.BookDispositionToRead), false, 0, 25)
	require.NoError(t, err)
	assert.Contains(t, fixtureBookIDs(toRead.Items), BookID, "current reading belongs in To Read")
	// The To Read projection counts the active German Books that are not Read.
	assert.Equal(t, 16, dispositionCount(toRead.DispositionCounts, domain.BookDispositionToRead))
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

// TestStoreMyBooksBrowseSearchesTitlesAndPages holds the search and pagination
// steps: a trimmed case-insensitive title substring, clamped caller-supplied
// paging, and the same overflow guard the PostgreSQL store keeps.
func TestStoreMyBooksBrowseSearchesTitlesAndPages(t *testing.T) {
	store := NewStore()
	store.books = nil
	store.myBooks = []domain.MyBook{
		{Book: domain.Book{ID: "browse-alpha", OwnerID: OwnerID, Title: "Alpha", LanguageState: domain.LanguageChosen, LanguageTag: "de"}},
		{Book: domain.Book{ID: "browse-alpha-two", OwnerID: OwnerID, Title: "ALPHA two", LanguageState: domain.LanguageChosen, LanguageTag: "de"}},
		{Book: domain.Book{ID: "browse-beta", OwnerID: OwnerID, Title: "Beta", LanguageState: domain.LanguageChosen, LanguageTag: "de"}},
		{Book: domain.Book{ID: "browse-alphabet", OwnerID: OwnerID, Title: "alphabet", LanguageState: domain.LanguageChosen, LanguageTag: "de"}},
	}
	for _, book := range store.myBooks {
		store.dispositions[fixtureDispositionKey(OwnerID, book.Book.ID)] = domain.BookDispositionToRead
	}

	search, err := store.ListMyBooksBrowse(t.Context(), OwnerID, "  ALPHA ", "de", "", false, 0, 25)
	require.NoError(t, err)
	t.Run("search is a trimmed case-insensitive title substring", func(t *testing.T) {
		assert.Equal(t, 3, search.Total)
		assert.Equal(t, []string{"browse-alpha", "browse-alpha-two", "browse-alphabet"}, fixtureBookIDs(search.Items))
	})

	first, err := store.ListMyBooksBrowse(t.Context(), OwnerID, "", "de", "", false, 0, 2)
	require.NoError(t, err)
	t.Run("the first page takes the leading matches", func(t *testing.T) {
		assert.Equal(t, 4, first.Total)
		assert.Equal(t, []string{"browse-alpha", "browse-alpha-two"}, fixtureBookIDs(first.Items))
	})

	second, err := store.ListMyBooksBrowse(t.Context(), OwnerID, "", "de", "", false, 2, 2)
	require.NoError(t, err)
	t.Run("the offset selects the next page", func(t *testing.T) {
		assert.Equal(t, 4, second.Total)
		assert.Equal(t, []string{"browse-alphabet", "browse-beta"}, fixtureBookIDs(second.Items))
	})

	beyond, err := store.ListMyBooksBrowse(t.Context(), OwnerID, "", "de", "", false, 10, 2)
	require.NoError(t, err)
	t.Run("an offset beyond the collection is empty", func(t *testing.T) {
		assert.Equal(t, 4, beyond.Total)
		assert.Empty(t, beyond.Items)
	})

	clamped, err := store.ListMyBooksBrowse(t.Context(), OwnerID, "", "de", "", false, -3, -1)
	require.NoError(t, err)
	t.Run("negative offset and limit clamp to an empty page", func(t *testing.T) {
		assert.Equal(t, 4, clamped.Total)
		assert.Empty(t, clamped.Items)
	})

	overflow, err := store.ListMyBooksBrowse(t.Context(), OwnerID, "", "de", "", false, 1, math.MaxInt)
	require.NoError(t, err)
	t.Run("an overflowing page end falls back to the collection end", func(t *testing.T) {
		assert.Equal(t, []string{"browse-alpha-two", "browse-alphabet", "browse-beta"}, fixtureBookIDs(overflow.Items))
	})
}

// TestStoreMyBooksBrowseCountsSkipUnchosenMembers pins that a legacy member
// with no chosen workflow role is absent from the list and from every count.
func TestStoreMyBooksBrowseCountsSkipUnchosenMembers(t *testing.T) {
	store := NewStore()
	store.books = nil
	store.myBooks = []domain.MyBook{
		{Book: domain.Book{ID: "unchosen-de", OwnerID: OwnerID, Title: "Unchosen", LanguageState: domain.LanguageChosen, LanguageTag: "de"}},
		{Book: domain.Book{ID: "unchosen-it", OwnerID: OwnerID, Title: "Unchosen Italian", LanguageState: domain.LanguageChosen, LanguageTag: "it"}},
	}
	store.unchosen[fixtureDispositionKey(OwnerID, "unchosen-de")] = true
	store.unchosen[fixtureDispositionKey(OwnerID, "unchosen-it")] = true

	for _, language := range []string{"", "de"} {
		result, err := store.ListMyBooksBrowse(t.Context(), OwnerID, "", language, "", false, 0, 25)
		require.NoError(t, err)
		assert.Zero(t, result.AllCount, "language %q", language)
		assert.Zero(t, result.ScopeTotal, "language %q", language)
		assert.Zero(t, result.Total, "language %q", language)
		assert.Empty(t, result.DispositionCounts, "language %q", language)
		assert.Zero(t, result.ReadCount, "language %q", language)
	}
}

// TestStoreMyBooksBrowseCountsWorkflow holds the To Read and Read tab counts,
// the To Read tab filter, and the active-language scope.
func TestStoreMyBooksBrowseCountsWorkflow(t *testing.T) {
	store := NewStore()
	store.books = nil
	store.myBooks = []domain.MyBook{
		{Book: domain.Book{ID: "browse-de-toread", OwnerID: OwnerID, Title: "To Read", LanguageState: domain.LanguageChosen, LanguageTag: "de"}},
		{Book: domain.Book{ID: "browse-de-read", OwnerID: OwnerID, Title: "Read", LanguageState: domain.LanguageChosen, LanguageTag: "de"}, CompletionCount: 1},
		{Book: domain.Book{ID: "browse-unknown", OwnerID: OwnerID, Title: "Unknown", LanguageState: domain.LanguageUnknown}},
		{Book: domain.Book{ID: "browse-it-toread", OwnerID: OwnerID, Title: "Italian", LanguageState: domain.LanguageChosen, LanguageTag: "it"}},
	}
	for _, id := range []string{"browse-de-toread", "browse-unknown", "browse-it-toread"} {
		store.dispositions[fixtureDispositionKey(OwnerID, id)] = domain.BookDispositionToRead
	}

	all, err := store.ListMyBooksBrowse(t.Context(), OwnerID, "", "", "", false, 0, 25)
	require.NoError(t, err)
	t.Run("the visible collection drives every count", func(t *testing.T) {
		assert.Equal(t, 4, all.AllCount)
		assert.Equal(t, 4, all.ScopeTotal)
		assert.Equal(t, 4, all.Total)
		assert.Equal(t, 1, all.ReadCount)
		assert.Equal(t, 3, dispositionCount(all.DispositionCounts, domain.BookDispositionToRead))
	})

	history, err := store.ListMyBooksBrowse(t.Context(), OwnerID, "", "", "", true, 0, 25)
	require.NoError(t, err)
	t.Run("Read selects the Read bucket only", func(t *testing.T) {
		assert.Equal(t, 1, history.Total)
		assert.Equal(t, []string{"browse-de-read"}, fixtureBookIDs(history.Items))
		assert.Equal(t, 1, history.ReadCount)
	})

	toRead, err := store.ListMyBooksBrowse(t.Context(), OwnerID, "", "", string(domain.BookDispositionToRead), false, 0, 25)
	require.NoError(t, err)
	t.Run("the To Read tab selects that bucket", func(t *testing.T) {
		assert.Equal(t, 3, toRead.Total)
		assert.Equal(t, []string{"browse-it-toread", "browse-de-toread", "browse-unknown"}, fixtureBookIDs(toRead.Items))
	})

	deOnly, err := store.ListMyBooksBrowse(t.Context(), OwnerID, "", "de", "", false, 0, 25)
	require.NoError(t, err)
	t.Run("the active language excludes an unchosen Book", func(t *testing.T) {
		assert.Equal(t, 2, deOnly.Total)
		assert.Equal(t, 2, deOnly.ScopeTotal)
		assert.Equal(t, []string{"browse-de-read", "browse-de-toread"}, fixtureBookIDs(deOnly.Items))
	})

	unknownScope, err := store.ListMyBooksBrowse(t.Context(), OwnerID, "", domain.LanguageUnknown, "", false, 0, 25)
	require.NoError(t, err)
	t.Run("the retired unknown scope is no longer a language", func(t *testing.T) {
		assert.Zero(t, unknownScope.Total)
		assert.Zero(t, unknownScope.ScopeTotal)
		assert.Empty(t, unknownScope.Items)
	})
}

// TestStoreMyBooksBrowseOrdersDeterministically holds the ordering step:
// lowercased title, then exact title, then Book ID.
func TestStoreMyBooksBrowseOrdersDeterministically(t *testing.T) {
	store := NewStore()
	store.books = nil
	store.myBooks = []domain.MyBook{
		{Book: domain.Book{ID: "browse-lower", OwnerID: OwnerID, Title: "beta", LanguageState: domain.LanguageChosen, LanguageTag: "de"}},
		{Book: domain.Book{ID: "browse-upper", OwnerID: OwnerID, Title: "Beta", LanguageState: domain.LanguageChosen, LanguageTag: "de"}},
		{Book: domain.Book{ID: "browse-tie-b", OwnerID: OwnerID, Title: "Tie", LanguageState: domain.LanguageChosen, LanguageTag: "de"}},
		{Book: domain.Book{ID: "browse-tie-a", OwnerID: OwnerID, Title: "Tie", LanguageState: domain.LanguageChosen, LanguageTag: "de"}},
	}
	for _, book := range store.myBooks {
		store.dispositions[fixtureDispositionKey(OwnerID, book.Book.ID)] = domain.BookDispositionToRead
	}

	result, err := store.ListMyBooksBrowse(t.Context(), OwnerID, "", "de", "", false, 0, 25)
	require.NoError(t, err)
	assert.Equal(t, []string{"browse-upper", "browse-lower", "browse-tie-a", "browse-tie-b"}, fixtureBookIDs(result.Items), "case-insensitive title, then exact title, then ID")
}

// TestStoreMyBooksProjectsReadingHistoryEvidence holds the completion evidence
// the My Books projection derives: an imported previously-read completion and
// every finish in reading history each count once, the most recent completion
// supplies LatestCompletionAt and LatestCompletionSource, and another owner's
// history never leaks in.
func TestStoreMyBooksProjectsReadingHistoryEvidence(t *testing.T) {
	store := NewStore()
	store.books = nil
	store.myBooks = []domain.MyBook{
		{Book: domain.Book{ID: "history-imported", OwnerID: OwnerID, Title: "Imported", LanguageState: domain.LanguageChosen, LanguageTag: "de"}},
		{Book: domain.Book{ID: "history-finished", OwnerID: OwnerID, Title: "Finished", LanguageState: domain.LanguageChosen, LanguageTag: "de"}},
		{Book: domain.Book{ID: "history-quiet", OwnerID: OwnerID, Title: "Quiet", LanguageState: domain.LanguageChosen, LanguageTag: "de"}},
	}
	importedAt := time.Date(2026, time.March, 1, 9, 0, 0, 0, time.UTC)
	store.importedHistory[fixtureDispositionKey(OwnerID, "history-imported")] = domain.ReadingCompletion{
		OwnerID: OwnerID, BookID: "history-imported", CompletedAt: importedAt, Source: domain.ReadingCompletionPreviouslyRead,
	}
	earlier := time.Date(2026, time.April, 1, 9, 0, 0, 0, time.UTC)
	later := time.Date(2026, time.May, 1, 9, 0, 0, 0, time.UTC)
	store.readingHistory["finish-earlier"] = domain.ReadingCompletion{
		OwnerID: OwnerID, BookID: "history-finished", CompletedAt: earlier, Source: domain.ReadingCompletionCurrentReading,
	}
	// Two completions share the latest timestamp so the projection always sees
	// a completion that does not advance LatestCompletionAt.
	store.readingHistory["finish-later-a"] = domain.ReadingCompletion{
		OwnerID: OwnerID, BookID: "history-finished", CompletedAt: later, Source: domain.ReadingCompletionCurrentReading,
	}
	store.readingHistory["finish-later-b"] = domain.ReadingCompletion{
		OwnerID: OwnerID, BookID: "history-finished", CompletedAt: later, Source: domain.ReadingCompletionCurrentReading,
	}
	store.readingHistory["finish-other-owner"] = domain.ReadingCompletion{
		OwnerID: "another-owner", BookID: "history-finished", CompletedAt: later.Add(time.Hour), Source: domain.ReadingCompletionCurrentReading,
	}

	books, err := store.ListMyBooksWithEvidence(t.Context(), OwnerID)
	require.NoError(t, err)
	byID := make(map[string]domain.MyBook, len(books))
	for _, book := range books {
		byID[book.Book.ID] = book
	}

	t.Run("an imported previously-read completion counts and dates the Book", func(t *testing.T) {
		imported := byID["history-imported"]
		assert.Equal(t, 1, imported.CompletionCount)
		require.NotNil(t, imported.LatestCompletionAt)
		assert.Equal(t, importedAt, *imported.LatestCompletionAt)
		assert.Equal(t, domain.ReadingCompletionPreviouslyRead, imported.LatestCompletionSource)
	})

	t.Run("reading-history finishes count and the latest supplies the date", func(t *testing.T) {
		finished := byID["history-finished"]
		assert.Equal(t, 3, finished.CompletionCount, "every owned completion counts and another owner's is ignored")
		require.NotNil(t, finished.LatestCompletionAt)
		assert.Equal(t, later, *finished.LatestCompletionAt, "the later completion wins")
		assert.Equal(t, domain.ReadingCompletionCurrentReading, finished.LatestCompletionSource)
	})

	t.Run("a Book with no history reports no completion", func(t *testing.T) {
		quiet := byID["history-quiet"]
		assert.Zero(t, quiet.CompletionCount)
		assert.Nil(t, quiet.LatestCompletionAt)
	})
}

func TestStartingFixtureBookRejectsMalformedAndUnknownIDsAsNotFound(t *testing.T) {
	store := NewStore()
	for _, bookID := range []string{"not-a-uuid", "00000000-0000-0000-0000-000000000000", "' OR 1=1 --"} {
		_, err := store.StartCurrentReading(t.Context(), OwnerID, "de", bookID)
		require.ErrorIs(t, err, domain.ErrNotFound, bookID)
	}
	_, err := store.StartCurrentReading(t.Context(), "not-a-uuid", "de", BookID)
	assert.ErrorIs(t, err, domain.ErrNotFound, "malformed owner")
}

func fixtureBookIDs(books []domain.MyBook) []string {
	ids := make([]string, 0, len(books))
	for _, book := range books {
		ids = append(ids, book.Book.ID)
	}
	return ids
}

func dispositionCount(counts []domain.DispositionCount, disposition domain.BookDisposition) int {
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

	err = store.EndCurrentReading(ctx, OwnerID, "it")
	require.NoError(t, err, "end Italian reading: %v", err)
	deGoal, err = store.GetCurrentReading(ctx, OwnerID, "de")
	require.NoError(t, err, "German Goal after Italian clear=%+v err=%v", deGoal, err)
	assert.Equal(t, BookID, deGoal.BookID, "German Goal after Italian clear=%+v err=%v", deGoal, err)
	itGoal, err = store.GetCurrentReading(ctx, OwnerID, "it")
	require.NoError(t, err, "Italian Goal after clear=%+v err=%v", itGoal, err)
	assert.Empty(t, itGoal.BookID, "Italian Goal after clear=%+v err=%v", itGoal, err)

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

func TestMigrationFixturesPinTheMetadataOnlyBook(t *testing.T) {
	store := NewStore()
	ctx := context.Background()

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

func TestFixtureCatalogueAliasScopesAcquisition(t *testing.T) {
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

	store.aliases[0].ConnectionID = "fixture-failed-connection"
	t.Run("wrong connection acquisition is not found", func(t *testing.T) {
		_, err := sync.FindAcquisitionTarget(ctx, OwnerID, "fixture-metadata-only")
		require.ErrorIs(t, err, cataloguesync.ErrNotFound, "wrong connection acquisition error=%v", err)
	})
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
	assert.Equal(t, int64(3), page.Total, "the default fixture shows every identity with its Study state")
	require.Len(t, page.Rows, 3)
	assert.Equal(t, domain.VocabularyStudyNotIntroduced, page.Rows[0].StudyState)
	assert.Equal(t, domain.VocabularyStudyMature, page.Rows[1].StudyState)
	assert.Equal(t, domain.VocabularyStudyInStudy, page.Rows[2].StudyState)

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
	assert.Equal(t, int64(3), browse(t, query).Total, "the empty scenario restores the default fixture")
}

// TestVocabularyBrowsePagesTheFixtureAndClampsPages holds how Browse serves the
// deliberately multi-page fixture and clamps pages: the "paging" prefix gives a
// multi-page fixture, and an ordinary prefix filters the default rows and clamps
// a stale page back into range.
func TestVocabularyBrowsePagesTheFixtureAndClampsPages(t *testing.T) {
	store := NewStore()
	ctx := t.Context()
	paging := func(page int) domain.VocabularyBrowseQuery {
		return domain.VocabularyBrowseQuery{Language: "de", CurrentBookID: BookID, ReadingBookID: BookID, Prefix: "paging", Page: page}
	}
	cases := []struct {
		name      string
		query     domain.VocabularyBrowseQuery
		wantPage  int
		wantTotal int64
		wantRows  int
		wantFirst string
	}{
		{name: "the first page serves the leading fixture rows", query: paging(1), wantPage: 1, wantTotal: 26, wantRows: 25, wantFirst: "paging00"},
		{name: "the second page serves the final fixture row", query: paging(2), wantPage: 2, wantTotal: 26, wantRows: 1, wantFirst: "paging25"},
		{name: "a page beyond the fixture clamps to the last page", query: paging(99), wantPage: 2, wantTotal: 26, wantRows: 1, wantFirst: "paging25"},
		{name: "a page below one becomes the first page", query: paging(0), wantPage: 1, wantTotal: 26, wantRows: 25, wantFirst: "paging00"},
		{
			name:      "an ordinary prefix filters the default rows and clamps the page",
			query:     domain.VocabularyBrowseQuery{Language: "de", CurrentBookID: BookID, ReadingBookID: BookID, Prefix: "  GE ", Page: 5},
			wantPage:  1,
			wantTotal: 1,
			wantRows:  1,
			wantFirst: "gehen",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			page, err := store.ListVocabularyBrowsePage(ctx, OwnerID, "de", tc.query)
			require.NoError(t, err)
			assert.Equal(t, tc.wantPage, page.Page)
			assert.Equal(t, tc.wantTotal, page.Total)
			require.Len(t, page.Rows, tc.wantRows)
			assert.Equal(t, tc.wantFirst, page.Rows[0].CanonicalLemma)
		})
	}
}

// TestVocabularyBrowseWithoutTheFixtureBookReturnsAnEmptyPage holds the Browse
// guard: the canned rows only appear for the fixture Book, so an ended reading
// and another current Book both yield an empty page that still names the
// current Book.
func TestVocabularyBrowseWithoutTheFixtureBookReturnsAnEmptyPage(t *testing.T) {
	store := NewStore()
	ctx := t.Context()
	query := domain.VocabularyBrowseQuery{Language: "de", Page: 1}

	unscoped, err := store.ListVocabularyBrowsePage(ctx, OwnerID, "de", query)
	require.NoError(t, err)
	assert.Empty(t, unscoped.CurrentBookID)
	assert.Empty(t, unscoped.Books)
	assert.Empty(t, unscoped.Rows)

	query.CurrentBookID = routeMatchBookID
	other, err := store.ListVocabularyBrowsePage(ctx, OwnerID, "de", query)
	require.NoError(t, err)
	assert.Equal(t, routeMatchBookID, other.CurrentBookID)
	assert.Empty(t, other.Books)
	assert.Empty(t, other.Rows)
}

// TestFixtureGetBookResolvesUntaggedSourceAndReportsMissing holds how GetBook
// projects a source entry and what it does when no Book matches.
func TestFixtureGetBookResolvesUntaggedSourceAndReportsMissing(t *testing.T) {
	store := NewStore()
	ctx := t.Context()
	store.books = append(store.books, domain.SourceMaterialSummary{
		Source: domain.SourceMaterial{ID: "fixture-untagged-source", OwnerID: OwnerID, Title: "Untagged"},
		BookID: "fixture-untagged-book",
	})

	t.Run("a source with a blank language is unchosen", func(t *testing.T) {
		book, err := store.GetBook(ctx, OwnerID, "fixture-untagged-book")
		require.NoError(t, err)
		assert.Equal(t, domain.LanguageUnknown, book.LanguageState)
		assert.Empty(t, book.LanguageTag)
		assert.Equal(t, "Untagged", book.Title)
	})

	t.Run("a source with no resolved Book ID is addressed by its source ID", func(t *testing.T) {
		resolved, err := store.GetBook(ctx, OwnerID, "fixture-empty")
		require.NoError(t, err)
		assert.Equal(t, "fixture-empty", resolved.ID)
	})

	t.Run("an unknown Book is not found", func(t *testing.T) {
		_, err := store.GetBook(ctx, OwnerID, "missing")
		require.ErrorIs(t, err, domain.ErrNotFound)
	})
}

// TestStoreStudyLanguagesIgnoreTagsThatNormalizeAway holds that a raw tag which
// trims to something but canonicalizes to no base language never becomes a study
// language.
func TestStoreStudyLanguagesIgnoreTagsThatNormalizeAway(t *testing.T) {
	store := NewStore()
	store.books = append(store.books, domain.SourceMaterialSummary{
		Source: domain.SourceMaterial{ID: "fixture-empty-tag-source", OwnerID: OwnerID, Language: "-"},
	})
	store.myBooks = append(store.myBooks, domain.MyBook{
		Book: domain.Book{ID: "fixture-empty-tag-book", OwnerID: OwnerID, LanguageState: domain.LanguageChosen, LanguageTag: "-"},
	})

	languages, err := store.ListStudyLanguages(t.Context(), OwnerID)
	require.NoError(t, err)
	got := make([]string, 0, len(languages))
	for _, language := range languages {
		got = append(got, language.Language)
	}
	assert.Equal(t, []string{"de", "it"}, got, "a tag that normalizes away is not a study language")
}

// TestFixtureLemmaDecisionRejectsAStaleOccurrenceClaim holds the decision
// write's precondition: a decision whose occurrence carries a correction the
// stored occurrence does not have is refused as not found and stores nothing.
func TestFixtureLemmaDecisionRejectsAStaleOccurrenceClaim(t *testing.T) {
	store := NewStore()
	ctx := t.Context()
	occurrences, err := store.ListLemmaReviewOccurrences(ctx, OwnerID, routeMatchBookID, "Weg")
	require.NoError(t, err)
	require.NotEmpty(t, occurrences)

	stale := occurrences[0]
	stale.CorrectedLemma = "pfad" // the stored occurrence carries no correction
	err = store.PutLemmaDecisions(ctx, []domain.LemmaReviewDecision{{Occurrence: stale, CanonicalLemma: "pfad"}})
	require.ErrorIs(t, err, domain.ErrNotFound)

	unchanged, err := store.ListLemmaReviewOccurrences(ctx, OwnerID, routeMatchBookID, "Weg")
	require.NoError(t, err)
	assert.Empty(t, unchanged[0].CorrectedLemma, "a rejected decision persists nothing")
}

func TestClassicalFixtureStartsWithNoOwnedUnitsAndRefusesUnknownUnits(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	legacy, err := store.ListLegacyLanguageRows(ctx, "grc")
	require.NoError(t, err)
	assert.Empty(t, legacy)
	owned, err := store.ListOwnedClassicalReadingUnits(ctx, OwnerID)
	require.NoError(t, err)
	assert.Empty(t, owned)
	_, err = store.AddClassicalReadingUnits(ctx, OwnerID, []string{"not-in-the-fixture-release"})
	require.ErrorContains(t, err, "not current")
}

func TestClassicalFixtureListsNoPreparationJobs(t *testing.T) {
	store := NewStore()
	jobs, err := store.ListBookPreparationJobs(context.Background(), OwnerID)
	require.NoError(t, err)
	for _, job := range jobs {
		assert.NotEqual(t, domain.BookPreparationKindClassical, job.Kind, "the browser harness binds no River queue")
		assert.NotEqual(t, domain.BookPreparationKindEPUBAcquisition, job.Kind, "the browser harness binds no River queue")
	}
}

func TestFixtureEPUBAcquisitionFailureRequiresCatalogIdentityOutsideEntryStage(t *testing.T) {
	cases := []struct {
		name    string
		stage   domain.BookPreparationStage
		wantErr bool
	}{
		{name: "a fetch failure names its catalog entry", stage: domain.BookPreparationStageFetch, wantErr: true},
		{name: "an unresolved entry-stage failure has no catalog identity", stage: domain.BookPreparationStageEntry},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := NewStore()
			err := store.RecordEPUBAcquisitionFailure(context.Background(), domain.EPUBAcquisitionFailureRecord{
				OwnerID: OwnerID, BookID: BookID,
				Stage: tc.stage, Code: domain.BookPreparationCodeSourceDownloadFailed,
			})
			if tc.wantErr {
				require.ErrorContains(t, err, "requires a catalog entry")
				return
			}
			require.NoError(t, err)
		})
	}
}

func TestStoreReadNextReplacesTheCurrentReadingAndRefusesIneligibleBooks(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	store.dispositions[fixtureDispositionKey(OwnerID, LemmaFlagBookID)] = domain.BookDispositionToRead
	store.dispositions[fixtureDispositionKey(OwnerID, routeMatchBookID)] = domain.BookDispositionToRead

	reading, err := store.GetCurrentReading(ctx, OwnerID, "de")
	require.NoError(t, err)
	require.Equal(t, BookID, reading.BookID)

	_, err = store.StartCurrentReading(ctx, OwnerID, "de", ItalianGoalBookID)
	var ineligible domain.CurrentReadingIneligibleError
	require.ErrorAs(t, err, &ineligible)
	assert.Equal(t, domain.CurrentReadingOtherLanguage, ineligible.Reason)
	current, err := store.GetCurrentReading(ctx, OwnerID, "de")
	require.NoError(t, err)
	assert.Equal(t, BookID, current.BookID, "a refused Read next keeps the current reading")

	flagged, err := store.StartCurrentReadingResult(ctx, OwnerID, "de", LemmaFlagBookID)
	require.NoError(t, err, "a Book with no prior reading is eligible for Read next")
	assert.Equal(t, LemmaFlagBookID, flagged.Reading.BookID)
	assert.False(t, flagged.Replayed)

	switched, err := store.StartCurrentReadingResult(ctx, OwnerID, "de", routeMatchBookID)
	require.NoError(t, err)
	assert.Equal(t, routeMatchBookID, switched.Reading.BookID)
	replayed, err := store.StartCurrentReadingResult(ctx, OwnerID, "de", routeMatchBookID)
	require.NoError(t, err)
	assert.True(t, replayed.Replayed)
}

func TestStoreEndAndFinishCurrentReadingNeedNoCommitment(t *testing.T) {
	ctx := context.Background()
	store := NewStore()

	require.NoError(t, store.EndCurrentReading(ctx, OwnerID, "de"))
	require.ErrorIs(t, store.EndCurrentReading(ctx, OwnerID, "de"), domain.ErrNotFound, "nothing left to end")
	_, err := store.FinishCurrentReading(ctx, OwnerID, "de")
	require.ErrorIs(t, err, domain.ErrNotFound, "nothing to finish")
	italian, err := store.GetCurrentReading(ctx, OwnerID, "it")
	require.NoError(t, err)
	assert.Equal(t, ItalianGoalBookID, italian.BookID, "ending the German reading leaves the Italian one")
}

func TestStoreReadingCompletionHistoryAllowsFutureCompletionOfSameBook(t *testing.T) {
	ctx := context.Background()
	store := NewStore()
	first, err := store.FinishCurrentReading(ctx, OwnerID, "de")
	require.NoError(t, err)
	assert.Equal(t, BookID, first.Completion.BookID)

	require.NoError(t, store.SetBookDisposition(ctx, OwnerID, BookID, domain.BookDispositionToRead))
	_, err = store.StartCurrentReading(ctx, OwnerID, "de", BookID)
	require.NoError(t, err)
	second, err := store.FinishCurrentReading(ctx, OwnerID, "de")
	require.NoError(t, err)
	assert.False(t, second.Completion.CompletedAt.Before(first.Completion.CompletedAt))
	assert.Len(t, store.readingHistory, 2, "every Finish appends a completion")
}
