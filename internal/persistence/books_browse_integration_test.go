//go:build integration

package persistence

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analysisinsights"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMyBooksBrowseFiltersCountsPagingAndOwnership(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)

	alice, err := store.CreateUser(ctx, "browse-alice", false)
	require.NoError(t, err)
	bob, err := store.CreateUser(ctx, "browse-bob", false)
	require.NoError(t, err)
	create := func(owner, title, state, tag, provenance string) domain.Book {
		input, createErr := domain.NewBook(owner, title, provenance, state, tag)
		require.NoError(t, createErr)
		book, createErr := store.CreateBook(ctx, input)
		require.NoError(t, createErr)
		return book
	}

	percent := create(alice.ID, "100% real", domain.LanguageUnknown, "", domain.MetadataProvenanceCatalogueSync)
	create(alice.ID, "German Upper", domain.LanguageChosen, "DE", domain.MetadataProvenanceCatalogueSync)
	create(alice.ID, "German Lower", domain.LanguageChosen, "de", domain.MetadataProvenanceCatalogueSync)
	metadataDampf := create(alice.ID, "Metadata Donaudampf", domain.LanguageUnknown, "", domain.MetadataProvenanceCatalogueSync)
	authorBook := create(alice.ID, "The Lost Daughter", domain.LanguageChosen, "it", domain.MetadataProvenanceCatalogueSync)
	_, err = store.UpdateBookMetadata(ctx, alice.ID, authorBook.ID, authorBook.Title, "Elena Ferrante", authorBook.LanguageState, authorBook.LanguageTag)
	require.NoError(t, err)
	acquiredSource := putBookSource(t, ctx, store, alice.ID, "browse-acquired", "Acquired Donaudampf", []byte("browse-content"), "browse content")
	acquiredID, err := store.ResolveOrCreateBookForAcquisition(ctx, alice.ID, acquiredSource.SourceIdentifier, acquiredSource.Language, acquiredSource.Title)
	require.NoError(t, err)
	err = store.LinkSourceToBook(ctx, alice.ID, acquiredID, acquiredSource.ID)
	require.NoError(t, err)
	for i := range 26 {
		create(alice.ID, fmt.Sprintf("Page %02d", i), domain.LanguageChosen, "it", domain.MetadataProvenanceCatalogueSync)
	}
	tieA := create(alice.ID, "Same title", domain.LanguageUnknown, "", domain.MetadataProvenanceCatalogueSync)
	tieB := create(alice.ID, "Same title", domain.LanguageUnknown, "", domain.MetadataProvenanceCatalogueSync)
	create(bob.ID, "Metadata Donaudampf", domain.LanguageUnknown, "", domain.MetadataProvenanceCatalogueSync)

	result, err := store.ListMyBooksBrowse(ctx, alice.ID, "Donaudampf", "", "", false, 0, 25)
	require.NoError(t, err)
	assert.Equal(t, 2, result.Total)
	assert.Equal(t, 2, len(result.Items))
	assert.Equal(t, 34, result.AllCount)
	for _, item := range result.Items {
		assert.Equal(t, alice.ID, item.Book.OwnerID, "cross-owner item leaked: %+v", item)
	}
	filtered, err := store.ListMyBooksBrowse(ctx, alice.ID, "", "it", string(domain.BookDispositionInbox), false, 0, 25)
	require.NoError(t, err)
	assert.Equal(t, 27, filtered.Total, "new catalogue books start in Inbox")
	assert.Equal(t, 27, filtered.ScopeTotal, "language scope count should include all dispositions")
	assert.Len(t, filtered.DispositionCounts, 1, "workflow counts should include every populated bucket")
	require.NoError(t, store.SetBookDisposition(ctx, alice.ID, authorBook.ID, domain.BookDispositionSetAside))
	filtered, err = store.ListMyBooksBrowse(ctx, alice.ID, "", "it", string(domain.BookDispositionSetAside), false, 0, 25)
	require.NoError(t, err)
	assert.Equal(t, 27, filtered.ScopeTotal)
	require.Len(t, filtered.Items, 1)
	assert.Equal(t, authorBook.ID, filtered.Items[0].Book.ID)
	assert.Equal(t, domain.BookDispositionSetAside, filtered.Items[0].Disposition)
	search, err := store.ListMyBooksBrowse(ctx, alice.ID, "lost", "it", string(domain.BookDispositionSetAside), false, 0, 25)
	require.NoError(t, err)
	require.Len(t, search.Items, 1, "search should compose with disposition filtering")
	assert.Equal(t, authorBook.ID, search.Items[0].Book.ID)
	inboxPage, err := store.ListMyBooksBrowse(ctx, alice.ID, "", "it", string(domain.BookDispositionInbox), false, 25, 25)
	require.NoError(t, err)
	assert.Equal(t, 26, inboxPage.Total)
	assert.Len(t, inboxPage.Items, 1, "paging should compose with disposition filtering")
	otherOwner, err := store.ListMyBooksBrowse(ctx, bob.ID, "", "it", string(domain.BookDispositionSetAside), false, 0, 25)
	require.NoError(t, err)
	assert.Zero(t, otherOwner.Total, "another owner's disposition results leaked")

	literal, err := store.ListMyBooksBrowse(ctx, alice.ID, "%", "", "", false, 0, 25)
	require.NoError(t, err)
	assert.Equal(t, 1, literal.Total)
	assert.Equal(t, percent.ID, literal.Items[0].Book.ID, "LIKE wildcard was not literal")
	authorResult, err := store.ListMyBooksBrowse(ctx, alice.ID, "ferrante", "", "", false, 0, 25)
	require.NoError(t, err)
	require.Len(t, authorResult.Items, 1)
	assert.Equal(t, authorBook.ID, authorResult.Items[0].Book.ID)

	deResult, err := store.ListMyBooksBrowse(ctx, alice.ID, "", "DE", "", false, 0, 25)
	require.NoError(t, err)
	assert.Equal(t, 3, deResult.Total, "German filter")
	require.Len(t, deResult.Counts, 3)
	assert.Equal(t, "de", deResult.Counts[0].Tag)
	assert.Equal(t, 3, deResult.Counts[0].Count)
	assert.Equal(t, "it", deResult.Counts[1].Tag)
	assert.Equal(t, 27, deResult.Counts[1].Count)
	assert.Equal(t, "unknown", deResult.Counts[2].Tag)
	assert.Equal(t, 4, deResult.Counts[2].Count)
	unknown, err := store.ListMyBooksBrowse(ctx, alice.ID, "Donaudampf", domain.LanguageUnknown, "", false, 0, 25)
	require.NoError(t, err)
	assert.Equal(t, 1, unknown.Total)
	assert.Equal(t, metadataDampf.ID, unknown.Items[0].Book.ID, "unknown combined filter")

	page, err := store.ListMyBooksBrowse(ctx, alice.ID, "", "", "", false, 25, 25)
	require.NoError(t, err)
	assert.Equal(t, 34, page.Total)
	assert.Equal(t, 9, len(page.Items), "page slice")
	tieIDs := []string{tieA.ID, tieB.ID}
	orderedTies, err := store.ListMyBooksBrowse(ctx, alice.ID, "Same title", "", "", false, 0, 25)
	require.NoError(t, err)
	require.Len(t, orderedTies.Items, 2)
	assert.Equal(t, minString(tieIDs[0], tieIDs[1]), orderedTies.Items[0].Book.ID)
	assert.Equal(t, maxString(tieIDs[0], tieIDs[1]), orderedTies.Items[1].Book.ID)

	err = store.RemoveBookFromMyBooks(ctx, alice.ID, percent.ID)
	require.NoError(t, err)
	remaining, err := store.ListMyBooksBrowse(ctx, alice.ID, "", "", "", false, 0, 25)
	require.NoError(t, err)
	assert.Equal(t, 33, remaining.AllCount)
	assert.Equal(t, 33, remaining.Total)
	assert.False(t, strings.Contains(fmt.Sprint(remaining.Items), percent.ID), "removed membership remained in browse")
}

func TestMyBooksBrowseUsesKeysetBatchesAcrossLargeCollection(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "browse-keyset", false)
	require.NoError(t, err)
	for i := range 260 {
		input, createErr := domain.NewBook(owner.ID, fmt.Sprintf("Batch Book %03d", i), domain.MetadataProvenanceCatalogueSync, domain.LanguageUnknown, "")
		require.NoError(t, createErr)
		_, createErr = store.CreateBook(ctx, input)
		require.NoError(t, createErr)
	}

	page, err := store.ListMyBooksBrowse(ctx, owner.ID, "", "", "", false, 250, 20)
	require.NoError(t, err)
	assert.Equal(t, 260, page.Total)
	require.Len(t, page.Items, 10)
	assert.Equal(t, "Batch Book 250", page.Items[0].Book.Title)
	assert.Equal(t, "Batch Book 259", page.Items[9].Book.Title)

	filtered, err := store.ListMyBooksBrowse(ctx, owner.ID, "Batch Book 25", "", "", false, 5, 5)
	require.NoError(t, err)
	assert.Equal(t, 10, filtered.Total)
	require.Len(t, filtered.Items, 5)
	assert.Equal(t, "Batch Book 255", filtered.Items[0].Book.Title)
	assert.Equal(t, "Batch Book 259", filtered.Items[4].Book.Title)
}

func TestMyBooksBrowseUsesOneWorkflowBucketForFiltersAndCounts(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "browse-buckets", false)
	require.NoError(t, err)
	book, source, _ := createReadingFixture(t, ctx, store, owner.ID, "bucket-german")
	makeAnalyzedToReadBook(t, ctx, store, book, source)
	current, err := store.StartCurrentReading(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)

	toRead, err := store.ListMyBooksBrowse(ctx, owner.ID, "", "de", string(domain.BookDispositionToRead), false, 0, 25)
	require.NoError(t, err)
	assert.Equal(t, 1, toRead.Total, "current reading belongs in the To Read filter")
	require.Len(t, toRead.Items, 1)
	assert.Equal(t, book.ID, toRead.Items[0].Book.ID)
	assert.Equal(t, domain.MyBookBucketCurrentReading, toRead.Items[0].WorkflowBucket())
	assert.Equal(t, 1, dispositionCountForBucket(toRead.DispositionCounts, domain.BookDispositionToRead))
	all, err := store.ListMyBooksBrowse(ctx, owner.ID, "", "de", "", false, 0, 25)
	require.NoError(t, err)
	require.Len(t, all.Items, 1)
	assert.True(t, all.Items[0].IsCurrentReading)
	assert.Equal(t, domain.MyBookBucketCurrentReading, all.Items[0].WorkflowBucket())
	assert.Equal(t, domain.BookDispositionToRead, all.Items[0].Disposition, "visible bucket does not rewrite persisted disposition")

	_, err = store.FinishCurrentReading(ctx, owner.ID, "de", book.ID, current.SnapshotID)
	require.NoError(t, err)
	read, err := store.ListMyBooksBrowse(ctx, owner.ID, "", "de", "", true, 0, 25)
	require.NoError(t, err)
	require.Len(t, read.Items, 1)
	assert.Equal(t, domain.MyBookBucketRead, read.Items[0].WorkflowBucket())
	assert.Equal(t, 1, read.ReadCount)
	assert.Equal(t, domain.BookDispositionInbox, read.Items[0].Disposition, "Read is a projection and preserves the underlying disposition")
	detail, err := store.GetBookDetail(ctx, owner.ID, book.ID)
	require.NoError(t, err)
	assert.Equal(t, read.Items[0].CompletionCount, detail.CompletionCount)
	require.NotNil(t, detail.LatestCompletionAt)
	assert.Equal(t, read.Items[0].LatestCompletionAt, detail.LatestCompletionAt)
	assert.Equal(t, read.Items[0].LatestCompletionSource, detail.LatestCompletionSource)

	// Reconsideration retains append-only history but moves the visible Book to
	// To Read. Setting it aside again projects that same Book back into Read.
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, book.ID, domain.BookDispositionToRead))
	read, err = store.ListMyBooksBrowse(ctx, owner.ID, "", "de", "", true, 0, 25)
	require.NoError(t, err)
	assert.Empty(t, read.Items, "To Read history is not also in Read")
	assert.Zero(t, read.ReadCount)
	toRead, err = store.ListMyBooksBrowse(ctx, owner.ID, "", "de", string(domain.BookDispositionToRead), false, 0, 25)
	require.NoError(t, err)
	require.Len(t, toRead.Items, 1)
	assert.Equal(t, domain.MyBookBucketToRead, toRead.Items[0].WorkflowBucket())
	assert.Equal(t, 1, toRead.Items[0].CompletionCount, "reconsideration retains history")
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, book.ID, domain.BookDispositionSetAside))
	read, err = store.ListMyBooksBrowse(ctx, owner.ID, "", "de", "", true, 0, 25)
	require.NoError(t, err)
	require.Len(t, read.Items, 1)
	assert.Equal(t, domain.MyBookBucketRead, read.Items[0].WorkflowBucket())
	assert.Equal(t, 1, read.ReadCount)
	all, err = store.ListMyBooksBrowse(ctx, owner.ID, "", "de", "", false, 0, 25)
	require.NoError(t, err)
	require.Len(t, all.Items, 1, "a Book with history appears once in All")
	assert.Equal(t, book.ID, all.Items[0].Book.ID)

	italian, italianSource, _ := createReadingFixtureInLanguage(t, ctx, store, owner.ID, "it", "bucket-italian")
	makeAnalyzedToReadBook(t, ctx, store, italian, italianSource)
	italianCurrent, err := store.StartCurrentReading(ctx, owner.ID, "it", italian.ID)
	require.NoError(t, err)
	italianHistoryInput, err := domain.NewBook(owner.ID, "Italian historical book", domain.MetadataProvenanceCatalogueSync, domain.LanguageChosen, "it")
	require.NoError(t, err)
	italianHistoryBook, err := store.CreateBook(ctx, italianHistoryInput)
	require.NoError(t, err)
	_, err = store.ImportPreviouslyRead(ctx, owner.ID, italianHistoryBook.ID)
	require.NoError(t, err)
	inbox, err := store.ListMyBooksBrowse(ctx, owner.ID, "", "it", string(domain.BookDispositionInbox), false, 0, 25)
	require.NoError(t, err)
	assert.Empty(t, inbox.Items, "previously read Book leaves Inbox")
	read, err = store.ListMyBooksBrowse(ctx, owner.ID, "", "it", "", true, 0, 25)
	require.NoError(t, err)
	require.Len(t, read.Items, 1)
	assert.Equal(t, italianHistoryBook.ID, read.Items[0].Book.ID)
	assert.Equal(t, 1, read.ReadCount)
	require.NoError(t, store.SetBookDisposition(ctx, owner.ID, book.ID, domain.BookDispositionToRead))
	_, err = store.StartCurrentReading(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	for language, bookID := range map[string]string{"de": book.ID, "it": italian.ID} {
		currentBooks, browseErr := store.ListMyBooksBrowse(ctx, owner.ID, "", language, "", false, 0, 25)
		require.NoError(t, browseErr)
		if language == "de" {
			assert.Zero(t, currentBooks.ReadCount, "current reading with history is not also counted in Read")
			readBooks, readErr := store.ListMyBooksBrowse(ctx, owner.ID, "", language, "", true, 0, 25)
			require.NoError(t, readErr)
			assert.Empty(t, readBooks.Items, "current reading with history is excluded from Read")
		} else {
			readBooks, readErr := store.ListMyBooksBrowse(ctx, owner.ID, "", language, "", true, 0, 25)
			require.NoError(t, readErr)
			require.Len(t, readBooks.Items, 1, "Read history is scoped to the selected language")
			assert.Equal(t, italianHistoryBook.ID, readBooks.Items[0].Book.ID)
			assert.Equal(t, 1, readBooks.ReadCount)
		}
		var currentCount int
		for _, item := range currentBooks.Items {
			if item.WorkflowBucket() == domain.MyBookBucketCurrentReading {
				currentCount++
				assert.Equal(t, bookID, item.Book.ID)
			}
		}
		assert.Equal(t, 1, currentCount, "current reading remains independent in %s", language)
	}
	stopped, err := store.GetCurrentReading(ctx, owner.ID, "de")
	require.NoError(t, err)
	require.NoError(t, store.EndCurrentReading(ctx, owner.ID, "de", book.ID, stopped.SnapshotID))
	toRead, err = store.ListMyBooksBrowse(ctx, owner.ID, "", "de", string(domain.BookDispositionToRead), false, 0, 25)
	require.NoError(t, err)
	require.Len(t, toRead.Items, 1)
	assert.Equal(t, domain.MyBookBucketToRead, toRead.Items[0].WorkflowBucket(), "stopping preserves To Read intent")

	stopped, err = store.StartCurrentReading(ctx, owner.ID, "de", book.ID)
	require.NoError(t, err)
	replacement, replacementSource, _ := createReadingFixture(t, ctx, store, owner.ID, "bucket-switch")
	makeAnalyzedToReadBook(t, ctx, store, replacement, replacementSource)
	switched, err := store.SwitchCurrentReading(ctx, owner.ID, "de", replacement.ID, book.ID, stopped.SnapshotID)
	require.NoError(t, err)
	all, err = store.ListMyBooksBrowse(ctx, owner.ID, "", "de", "", false, 0, 25)
	require.NoError(t, err)
	buckets := make(map[string]domain.MyBookBucket, len(all.Items))
	for _, item := range all.Items {
		buckets[item.Book.ID] = item.WorkflowBucket()
	}
	assert.Equal(t, domain.MyBookBucketToRead, buckets[book.ID], "switch returns the former current Book to To Read")
	assert.Equal(t, domain.MyBookBucketCurrentReading, buckets[replacement.ID])
	require.NoError(t, store.EndCurrentReading(ctx, owner.ID, "de", replacement.ID, switched.SnapshotID))
	all, err = store.ListMyBooksBrowse(ctx, owner.ID, "", "de", "", false, 0, 25)
	require.NoError(t, err)
	for _, item := range all.Items {
		if item.Book.ID == replacement.ID {
			assert.Equal(t, domain.BookDispositionToRead, item.Disposition)
			assert.Equal(t, domain.MyBookBucketToRead, item.WorkflowBucket(), "ending keeps the Book in To Read")
		}
	}
	assert.NotEmpty(t, italianCurrent.SnapshotID)
}

func TestMyBooksBrowseProjectsPopulatedCoverageCurrentReadingAndDeck(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "browse-margin-evidence", false)
	require.NoError(t, err)
	book, source, _ := createReadingFixtureInLanguage(t, ctx, store, owner.ID, "el", "margin-evidence")
	makeAnalyzedToReadBook(t, ctx, store, book, source)
	var runID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text FROM book_current_analyses WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&runID))
	var corpusID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT id::text FROM corpora WHERE owner_id=$1 AND source_material_id=$2`, owner.ID, source.ID).Scan(&corpusID))
	_, err = store.Pool().Exec(ctx, `UPDATE corpora SET analyzable_token_count=100, distinct_lemma_count=2 WHERE owner_id=$1 AND source_material_id=$2`, owner.ID, source.ID)
	require.NoError(t, err)
	const sourceDocumentID = "margin-evidence-unit"
	_, err = store.Pool().Exec(ctx, `INSERT INTO corpus_sentences(owner_id,analysis_run_id,corpus_id,unit_id,sentence_ordinal,sentence_text,start_offset,end_offset) VALUES($1,$2,$3,$4,0,'fixture evidence',0,2000)`, owner.ID, runID, corpusID, sourceDocumentID)
	require.NoError(t, err)
	insertToken := func(ordinal int64, lemma, upos, dependency string, start, end int64) {
		t.Helper()
		_, insertErr := store.Pool().Exec(ctx, `INSERT INTO corpus_tokens(owner_id,language,analysis_run_id,corpus_id,sentence_ordinal,token_ordinal,surface,raw_lemma,canonical_lemma,upos,morphology,dependency,head,start_offset,end_offset) VALUES($1,'el',$2,$3,0,$4,$5,$5,$5,$6,'{}'::jsonb,$7,0,$8,$9)`, owner.ID, runID, corpusID, ordinal, lemma, upos, dependency, start, end)
		require.NoError(t, insertErr)
	}
	for i := range int64(90) {
		start := i * 10
		insertToken(i, "σπίτι", "NOUN", "root", start, start+5)
	}
	for i := range int64(10) {
		start := int64(1000) + i*10
		insertToken(90+i, "πηγαίνω", "VERB", "root", start, start+7)
	}
	for i := range int64(5) {
		start := int64(1200) + i*10
		insertToken(100+i, "123", "NOUN", "root", start, start+3)
	}
	_, err = store.PutKnownVocabulary(ctx, owner.ID, "el", "σπίτι", "")
	require.NoError(t, err)
	// Correct an unknown occurrence into Known, and exclude five formerly Known
	// occurrences. The browse projection must follow the same decisions as Reading.
	_, err = store.Pool().Exec(ctx, `INSERT INTO occurrence_lemma_corrections(owner_id,book_id,corpus_id,analysis_run_id,source_document_id,start_offset,end_offset,canonical_lemma,normalization_profile,normalization_version,excluded) VALUES($1,$2,$3,$4,$5,1000,1007,'σπίτι','el','1',false)`, owner.ID, book.ID, corpusID, runID, sourceDocumentID)
	require.NoError(t, err)
	for i := range int64(5) {
		start := i * 10
		_, err = store.Pool().Exec(ctx, `INSERT INTO occurrence_lemma_corrections(owner_id,book_id,corpus_id,analysis_run_id,source_document_id,start_offset,end_offset,canonical_lemma,normalization_profile,normalization_version,excluded) VALUES($1,$2,$3,$4,$5,$6,$7,NULL,NULL,NULL,true)`, owner.ID, book.ID, corpusID, runID, sourceDocumentID, start, start+5)
		require.NoError(t, err)
	}
	// Effective Browse counts after the correction (+1) and exclusions (-5).
	for _, count := range []struct {
		lemma, upos string
		occurrences int64
	}{{"σπίτι", "NOUN", 86}, {"πηγαίνω", "VERB", 9}} {
		_, err = store.Pool().Exec(ctx, `INSERT INTO vocabulary_browse_counts(owner_id,book_id,language,analysis_run_id,corpus_id,canonical_lemma,upos,occurrence_count) VALUES($1,$2,'el',$3,$4,$5,$6,$7)`, owner.ID, book.ID, runID, corpusID, count.lemma, count.upos, count.occurrences)
		require.NoError(t, err)
	}
	_, err = store.Pool().Exec(ctx, `INSERT INTO vocabulary_browse_count_readiness(owner_id,book_id,language,analysis_run_id,corpus_id,builder_version) VALUES($1,$2,'el',$3,$4,$5)`, owner.ID, book.ID, runID, corpusID, vocabularyBrowseCountBuilderVersion)
	require.NoError(t, err)
	preparation, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{
		OwnerID: owner.ID, SourceMaterialID: source.ID, AnalysisRunID: runID,
		Filename: "margin.apkg", DeckName: "Margin evidence", ContentHash: source.ContentHash,
	})
	require.NoError(t, err)
	_, err = store.ClaimDeckPreparation(ctx, owner.ID, preparation.ID)
	require.NoError(t, err)
	ready, err := store.CompleteDeckPreparation(ctx, owner.ID, preparation.ID, domain.DeckPreparation{
		Artifact: []byte("apkg"), Filename: "margin.apkg", DeckName: "Margin evidence", TotalCards: 42,
	})
	require.NoError(t, err)

	beforeCurrent, err := store.ListMyBooksBrowse(ctx, owner.ID, "", "el", "", false, 0, 25)
	require.NoError(t, err)
	require.Len(t, beforeCurrent.Items, 1)
	assert.False(t, beforeCurrent.Items[0].IsCurrentReading)
	assert.Empty(t, beforeCurrent.Items[0].DeckState, "prepared deck evidence is not queried for a non-current Book")

	_, err = store.StartCurrentReading(ctx, owner.ID, "el", book.ID)
	require.NoError(t, err)
	browse, err := store.ListMyBooksBrowse(ctx, owner.ID, "", "el", "", false, 0, 25)
	require.NoError(t, err)
	require.Len(t, browse.Items, 1)
	got := browse.Items[0]
	assert.True(t, got.IsCurrentReading)
	assert.Equal(t, int64(86), got.CoverageKnownTokens, "wildcard-UPOS identity plus corrections/exclusions drive Known coverage")
	assert.Equal(t, int64(100), got.CoverageTotalTokens, "the source-derived denominator is authoritative even when occurrence exclusions exist")
	authoritativeCoverage, err := analysisinsights.NewService(store).Coverage(ctx, owner.ID, corpusID)
	require.NoError(t, err)
	assert.Equal(t, authoritativeCoverage.KnownTokenCount, got.CoverageKnownTokens, "My Books Known matches the authoritative coverage service after exact-occurrence corrections and exclusions")
	assert.Equal(t, authoritativeCoverage.AnalyzableTokenCount, got.CoverageTotalTokens, "My Books total matches the authoritative source-derived denominator after exclusions")
	assert.Equal(t, "ready", got.DeckState)
	assert.Equal(t, int64(42), got.DeckCardCount)
	require.NotNil(t, got.DeckPreparedAt)
	assert.Equal(t, ready.CompletedAt, got.DeckPreparedAt)

	// Generic detail reads remain lean; only the metadata-refresh row read loads
	// corpus-sized current margin evidence.
	detail, err := store.GetBookDetail(ctx, owner.ID, book.ID)
	require.NoError(t, err)
	assert.Zero(t, detail.CoverageKnownTokens)
	assert.Zero(t, detail.CoverageTotalTokens)
	assert.Empty(t, detail.DeckState)
	assert.False(t, detail.IsCurrentReading)

	refreshDetail, err := store.GetBookDetailForMyBooksRefresh(ctx, owner.ID, book.ID)
	require.NoError(t, err)
	assert.Equal(t, got.CoverageKnownTokens, refreshDetail.CoverageKnownTokens, "excluded Known occurrences must not inflate the numerator")
	assert.Equal(t, int64(100), refreshDetail.CoverageTotalTokens)
	assert.Equal(t, got.DeckState, refreshDetail.DeckState)
	assert.Equal(t, got.DeckCardCount, refreshDetail.DeckCardCount)
	assert.True(t, refreshDetail.IsCurrentReading)
}

func dispositionCountForBucket(counts []DispositionCount, disposition domain.BookDisposition) int {
	for _, count := range counts {
		if count.Disposition == disposition {
			return count.Count
		}
	}
	return 0
}

func minString(left, right string) string {
	if left < right {
		return left
	}
	return right
}

func maxString(left, right string) string {
	if left > right {
		return left
	}
	return right
}
