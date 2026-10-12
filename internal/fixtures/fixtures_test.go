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

// TestStoreMyBooksBrowseVisibilityScope holds the Hidden visibility scope:
// Hidden Books leave the page unless showHidden is set, HiddenCount follows the
// selected language regardless of that choice, and AllCount stays complete.
func TestStoreMyBooksBrowseVisibilityScope(t *testing.T) {
	store := NewStore()
	store.books = nil
	store.myBooks = []domain.MyBook{
		{Book: domain.Book{ID: "browse-visible-de", OwnerID: OwnerID, Title: "Visible German", LanguageState: domain.LanguageChosen, LanguageTag: "de"}},
		{Book: domain.Book{ID: "browse-hidden-de", OwnerID: OwnerID, Title: "Hidden German", LanguageState: domain.LanguageChosen, LanguageTag: "de"}},
		{Book: domain.Book{ID: "browse-hidden-it", OwnerID: OwnerID, Title: "Hidden Italian", LanguageState: domain.LanguageChosen, LanguageTag: "it"}},
	}
	store.visibility[fixtureDispositionKey(OwnerID, "browse-hidden-de")] = fixtureVisibility{hidden: true, revision: 1}
	store.visibility[fixtureDispositionKey(OwnerID, "browse-hidden-it")] = fixtureVisibility{hidden: true, revision: 1}

	t.Run("Hidden Books leave the page but stay counted", func(t *testing.T) {
		hidden, err := store.ListMyBooksBrowseWithVisibility(t.Context(), OwnerID, "", "de", "", false, false, 0, 25)
		require.NoError(t, err)
		assert.Equal(t, 3, hidden.AllCount, "AllCount stays the complete collection")
		assert.Equal(t, 1, hidden.HiddenCount, "HiddenCount follows the language scope")
		assert.Equal(t, 1, hidden.Total)
		assert.Equal(t, 1, hidden.ScopeTotal)
		assert.Equal(t, []string{"browse-visible-de"}, fixtureBookIDs(hidden.Items))
	})
	t.Run("showHidden pages the Hidden Book without changing HiddenCount", func(t *testing.T) {
		shown, err := store.ListMyBooksBrowseWithVisibility(t.Context(), OwnerID, "", "de", "", false, true, 0, 25)
		require.NoError(t, err)
		assert.Equal(t, 1, shown.HiddenCount)
		assert.Equal(t, 2, shown.Total)
		assert.Equal(t, 2, shown.ScopeTotal)
		assert.Equal(t, []string{"browse-hidden-de", "browse-visible-de"}, fixtureBookIDs(shown.Items))
	})
	t.Run("Hidden Books outside the language scope do not count", func(t *testing.T) {
		unknown, err := store.ListMyBooksBrowseWithVisibility(t.Context(), OwnerID, "", domain.LanguageUnknown, "", false, false, 0, 25)
		require.NoError(t, err)
		assert.Zero(t, unknown.HiddenCount)
		assert.Zero(t, unknown.Total)
	})
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

	search, err := store.ListMyBooksBrowseWithVisibility(t.Context(), OwnerID, "  ALPHA ", "de", "", false, false, 0, 25)
	require.NoError(t, err)
	t.Run("search is a trimmed case-insensitive title substring", func(t *testing.T) {
		assert.Equal(t, 3, search.Total)
		assert.Equal(t, []string{"browse-alpha", "browse-alpha-two", "browse-alphabet"}, fixtureBookIDs(search.Items))
	})

	first, err := store.ListMyBooksBrowseWithVisibility(t.Context(), OwnerID, "", "de", "", false, false, 0, 2)
	require.NoError(t, err)
	t.Run("the first page takes the leading matches", func(t *testing.T) {
		assert.Equal(t, 4, first.Total)
		assert.Equal(t, []string{"browse-alpha", "browse-alpha-two"}, fixtureBookIDs(first.Items))
	})

	second, err := store.ListMyBooksBrowseWithVisibility(t.Context(), OwnerID, "", "de", "", false, false, 2, 2)
	require.NoError(t, err)
	t.Run("the offset selects the next page", func(t *testing.T) {
		assert.Equal(t, 4, second.Total)
		assert.Equal(t, []string{"browse-alphabet", "browse-beta"}, fixtureBookIDs(second.Items))
	})

	beyond, err := store.ListMyBooksBrowseWithVisibility(t.Context(), OwnerID, "", "de", "", false, false, 10, 2)
	require.NoError(t, err)
	t.Run("an offset beyond the collection is empty", func(t *testing.T) {
		assert.Equal(t, 4, beyond.Total)
		assert.Empty(t, beyond.Items)
	})

	clamped, err := store.ListMyBooksBrowseWithVisibility(t.Context(), OwnerID, "", "de", "", false, false, -3, -1)
	require.NoError(t, err)
	t.Run("negative offset and limit clamp to an empty page", func(t *testing.T) {
		assert.Equal(t, 4, clamped.Total)
		assert.Empty(t, clamped.Items)
	})

	overflow, err := store.ListMyBooksBrowseWithVisibility(t.Context(), OwnerID, "", "de", "", false, false, 1, math.MaxInt)
	require.NoError(t, err)
	t.Run("an overflowing page end falls back to the collection end", func(t *testing.T) {
		assert.Equal(t, []string{"browse-alpha-two", "browse-alphabet", "browse-beta"}, fixtureBookIDs(overflow.Items))
	})
}

// TestStoreMyBooksBrowseCountsLanguageAndWorkflow holds the count steps: the
// language pills with unknown last, the Read projection, the persisted
// disposition counts, the Read-history filter, and the language scope.
func TestStoreMyBooksBrowseCountsLanguageAndWorkflow(t *testing.T) {
	store := NewStore()
	store.books = nil
	store.myBooks = []domain.MyBook{
		{Book: domain.Book{ID: "browse-de-inbox", OwnerID: OwnerID, Title: "Inbox", LanguageState: domain.LanguageChosen, LanguageTag: "de"}},
		{Book: domain.Book{ID: "browse-de-read", OwnerID: OwnerID, Title: "Read", LanguageState: domain.LanguageChosen, LanguageTag: "de"}, CompletionCount: 1},
		{Book: domain.Book{ID: "browse-unknown", OwnerID: OwnerID, Title: "Unknown", LanguageState: domain.LanguageUnknown}},
		{Book: domain.Book{ID: "browse-it-toread", OwnerID: OwnerID, Title: "Italian", LanguageState: domain.LanguageChosen, LanguageTag: "it"}},
	}
	store.dispositions[fixtureDispositionKey(OwnerID, "browse-it-toread")] = domain.BookDispositionToRead

	all, err := store.ListMyBooksBrowseWithVisibility(t.Context(), OwnerID, "", "", "", false, false, 0, 25)
	require.NoError(t, err)
	t.Run("the visible collection drives every count", func(t *testing.T) {
		assert.Equal(t, 4, all.AllCount)
		assert.Equal(t, 4, all.ScopeTotal)
		assert.Equal(t, 4, all.Total)
		assert.Equal(t, 1, all.ReadCount)
		assert.Equal(t, []domain.LanguageCount{{Tag: "de", Count: 2}, {Tag: "it", Count: 1}, {Tag: "unknown", Count: 1}}, all.Counts, "unknown sorts last")
		assert.Equal(t, 2, dispositionCount(all.DispositionCounts, domain.BookDispositionInbox))
		assert.Equal(t, 1, dispositionCount(all.DispositionCounts, domain.BookDispositionToRead))
	})

	history, err := store.ListMyBooksBrowseWithVisibility(t.Context(), OwnerID, "", "", "", true, false, 0, 25)
	require.NoError(t, err)
	t.Run("Read history selects the Read bucket only", func(t *testing.T) {
		assert.Equal(t, 1, history.Total)
		assert.Equal(t, []string{"browse-de-read"}, fixtureBookIDs(history.Items))
		assert.Equal(t, 1, history.ReadCount)
	})

	inbox, err := store.ListMyBooksBrowseWithVisibility(t.Context(), OwnerID, "", "", string(domain.BookDispositionInbox), false, false, 0, 25)
	require.NoError(t, err)
	t.Run("the disposition filter selects one bucket", func(t *testing.T) {
		assert.Equal(t, 2, inbox.Total)
		assert.Equal(t, []string{"browse-de-inbox", "browse-unknown"}, fixtureBookIDs(inbox.Items))
	})

	unknownOnly, err := store.ListMyBooksBrowseWithVisibility(t.Context(), OwnerID, "", domain.LanguageUnknown, "", false, false, 0, 25)
	require.NoError(t, err)
	t.Run("the language scope keeps every pill", func(t *testing.T) {
		assert.Equal(t, 1, unknownOnly.Total)
		assert.Equal(t, 1, unknownOnly.ScopeTotal)
		assert.Equal(t, []domain.LanguageCount{{Tag: "de", Count: 2}, {Tag: "it", Count: 1}, {Tag: "unknown", Count: 1}}, unknownOnly.Counts, "pills describe the whole visible collection")
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

	result, err := store.ListMyBooksBrowseWithVisibility(t.Context(), OwnerID, "", "de", "", false, false, 0, 25)
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

func TestUnresolvedLemmaReviewFlagBlocksStartingFixtureBook(t *testing.T) {
	store := NewStore()
	current, err := store.GetCurrentReading(t.Context(), OwnerID, "de")
	require.NoError(t, err)
	require.NoError(t, store.EndCurrentReading(t.Context(), OwnerID, "de", current.BookID, current.SnapshotID))
	store.dispositions[fixtureDispositionKey(OwnerID, LemmaFlagBookID)] = domain.BookDispositionToRead
	_, err = store.StartCurrentReading(t.Context(), OwnerID, "de", LemmaFlagBookID)
	assert.ErrorIs(t, err, domain.ErrUnresolvedLemmaReviewFlags)
}

func TestLemmaDecisionProposalIsRejectedForTheCurrentReadingBook(t *testing.T) {
	store := NewStore()
	current, err := store.GetCurrentReading(t.Context(), OwnerID, "de")
	require.NoError(t, err)
	require.Equal(t, BookID, current.BookID, "the fixture seeds BookID as the German current reading")
	occurrences, err := store.ListLemmaReviewOccurrences(t.Context(), OwnerID, BookID, "Weg")
	require.NoError(t, err)
	proposal := domain.LemmaReviewProposal{
		OwnerID: OwnerID, BookID: BookID, Language: "de", Surface: "Weg", Action: "correct", Lemma: "pfad",
		Occurrences: occurrences[:1], NormalizationProfile: "german-post-1996", NormalizationVersion: "6",
	}
	err = store.PutLemmaDecisionProposal(t.Context(), proposal, "any-fingerprint")
	require.ErrorIs(t, err, domain.ErrLemmaDecisionCurrentReading, "the gate runs before the preview fingerprint")
	err = store.PutLemmaDecisions(t.Context(), proposal.Decisions())
	require.ErrorIs(t, err, domain.ErrNotFound, "the single-decision path keeps its not-found contract")
	unchanged, err := store.ListLemmaReviewOccurrences(t.Context(), OwnerID, BookID, "Weg")
	require.NoError(t, err)
	assert.Empty(t, unchanged[0].CorrectedLemma, "a rejected decision persists nothing")
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

func TestStartingFixtureBookReportsAlreadyCurrentBeforeLemmaReviewFlags(t *testing.T) {
	store := NewStore()
	current, err := store.GetCurrentReading(t.Context(), OwnerID, "de")
	require.NoError(t, err)
	require.True(t, current.IsActive(), "the fixture seeds a German current reading")
	_, err = store.StartCurrentReading(t.Context(), OwnerID, "de", LemmaFlagBookID)
	assert.ErrorIs(t, err, domain.ErrCurrentReadingExists)
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
	require.ErrorIs(t, err, domain.ErrCurrentReadingStale)
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
	require.ErrorIs(t, err, domain.ErrCurrentReadingStale)
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
	require.ErrorIs(t, err, domain.ErrNotFound)

	fixtureDeck := PreparedDeck{Store: store}
	_, err = fixtureDeck.PrepareCurrentReadingDeck(ctx, OwnerID, goal.BookID, goal.SnapshotID)
	require.ErrorIs(t, err, domain.ErrDeckPreparationInvalidTransition, "a healthy ready deck for the exact snapshot is not resubmitted")
	preparation, err = store.GetDeckPreparationForSnapshot(ctx, OwnerID, goal.SnapshotID)
	require.NoError(t, err)
	assert.Equal(t, PrepID, preparation.ID)

	_, err = fixtureDeck.PrepareCurrentReadingDeck(ctx, OwnerID, goal.BookID, "fresh-goal-snapshot")
	require.ErrorIs(t, err, domain.ErrDeckPreparationStale, "a request naming another snapshot is refused")
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
	t.Run("wrong connection acquisition is not found", func(t *testing.T) {
		_, err := sync.FindAcquisitionTarget(ctx, OwnerID, "fixture-metadata-only")
		require.ErrorIs(t, err, cataloguesync.ErrNotFound, "wrong connection acquisition error=%v", err)
	})
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
	require.ErrorIs(t, err, domain.ErrUnresolvedLemmaReviewFlags, "Switch blocks on unresolved flags as Postgres does")
	_, err = store.SwitchCurrentReading(ctx, OwnerID, "de", ItalianGoalBookID, reading.BookID, reading.SnapshotID)
	var ineligible domain.CurrentReadingIneligibleError
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
	require.ErrorIs(t, store.EndCurrentReading(ctx, OwnerID, "de", switched.BookID, reading.SnapshotID), domain.ErrCurrentReadingStale, "a released snapshot is no End replay for another Book")

	restarted, err := store.StartCurrentReading(ctx, OwnerID, "de", switched.BookID)
	require.NoError(t, err)
	finished, err := store.FinishCurrentReading(ctx, OwnerID, "de", restarted.BookID, restarted.SnapshotID)
	require.NoError(t, err)
	assert.Equal(t, restarted.SnapshotID, finished.Completion.SnapshotID)
	require.ErrorIs(t, store.EndCurrentReading(ctx, OwnerID, "de", restarted.BookID, restarted.SnapshotID), domain.ErrCurrentReadingStale, "a completed snapshot is never an End replay")
}
