//go:build integration

package persistence

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadingJourneyBackfillAndPersistence(t *testing.T) {
	ctx := context.Background()
	url, pool := testutil.Postgres(t, ctx, Migrate)
	store, err := Open(ctx, url)
	require.NoError(t, err)
	defer store.Close()
	alice, err := store.CreateUser(ctx, "journey-alice", false)
	require.NoError(t, err)
	bob, err := store.CreateUser(ctx, "journey-bob", false)
	require.NoError(t, err)

	queuedEarly, _, _ := createJourneyFixture(t, ctx, store, alice.ID, "queued-early")
	shared, _, _ := createJourneyFixture(t, ctx, store, alice.ID, "queued-shared-first")
	_, _, _ = createJourneySourceAndPreparation(t, ctx, store, alice.ID, shared, "queued-shared-second")
	active, _, _ := createJourneyFixture(t, ctx, store, alice.ID, "active")
	italian, _, _ := createJourneyFixtureInLanguage(t, ctx, store, alice.ID, "it", "queued-italian")
	bobQueued, _, _ := createJourneyFixture(t, ctx, store, bob.ID, "bob-queued")
	_, _, _ = createJourneyFixture(t, ctx, store, alice.ID, "complete")
	_, _, _ = createJourneyFixture(t, ctx, store, alice.ID, "abandoned")

	for _, item := range []struct {
		owner, language, bookID string
		position                int
	}{
		{alice.ID, "de", queuedEarly.ID, 1}, {alice.ID, "de", shared.ID, 2},
		{alice.ID, "it", italian.ID, 1}, {bob.ID, "de", bobQueued.ID, 1},
	} {
		_, err = pool.Exec(ctx, `INSERT INTO reading_journeys(owner_id,language) VALUES($1,$2) ON CONFLICT DO NOTHING`, item.owner, item.language)
		require.NoError(t, err)
		_, err = pool.Exec(ctx, `INSERT INTO reading_journey_membership(owner_id,language,book_id,position) VALUES($1,$2,$3,$4)`, item.owner, item.language, item.bookID, item.position)
		require.NoError(t, err)
	}
	journey, err := store.GetReadingJourney(ctx, alice.ID, "de")
	require.NoError(t, err)
	assertJourneyEntries(t, journey, alice.ID, []string{queuedEarly.ID, shared.ID})
	italianJourney, err := store.GetReadingJourney(ctx, alice.ID, "it")
	require.NoError(t, err)
	assertJourneyEntries(t, italianJourney, alice.ID, []string{italian.ID})
	bobJourney, err := store.GetReadingJourney(ctx, bob.ID, "de")
	require.NoError(t, err)
	assertJourneyEntries(t, bobJourney, bob.ID, []string{bobQueued.ID})

	store.Close()
	store, err = Open(ctx, url)
	require.NoError(t, err)
	defer store.Close()

	carol, err := store.CreateUser(ctx, "journey-carol", false)
	require.NoError(t, err)
	empty, err := store.GetReadingJourney(ctx, carol.ID, "de")
	require.NoError(t, err)
	assert.Zero(t, empty.Revision)
	assert.Empty(t, empty.Entries)
	assert.Equal(t, carol.ID, empty.OwnerID)

	aliceExtra, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Alice extra", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	bobExtra, err := store.CreateBook(ctx, domain.Book{OwnerID: bob.ID, Title: "Bob extra", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)

	journey, err = store.GetReadingJourney(ctx, alice.ID, "de")
	require.NoError(t, err)
	revision, err := store.AddToReadingJourney(ctx, alice.ID, "de", aliceExtra.ID, journey.Revision)
	require.NoError(t, err)
	assert.Equal(t, int64(1), revision, "add alice extra")
	repeatedRevision, repeatErr := store.AddToReadingJourney(ctx, alice.ID, "de", aliceExtra.ID, revision)
	require.NoError(t, repeatErr)
	assert.Equal(t, revision, repeatedRevision, "repeated add")
	_, err = store.AddToReadingJourney(ctx, alice.ID, "de", aliceExtra.ID, 0)
	assert.ErrorIs(t, err, ErrJourneyStale)
	_, err = store.AddToReadingJourney(ctx, alice.ID, "de", bobQueued.ID, revision)
	assert.ErrorIs(t, err, ErrNotFound)

	revision, err = store.RemoveFromReadingJourney(ctx, alice.ID, "de", queuedEarly.ID, revision)
	require.NoError(t, err)
	assert.Equal(t, int64(2), revision, "remove alice early")
	journey, err = store.GetReadingJourney(ctx, alice.ID, "de")
	require.NoError(t, err)
	assertJourneyEntries(t, journey, alice.ID, []string{shared.ID, aliceExtra.ID})
	revision, err = store.RemoveFromReadingJourney(ctx, alice.ID, "de", active.ID, revision)
	require.NoError(t, err)
	assert.Equal(t, int64(2), revision, "remove absent")

	revision, err = store.MoveReadingJourneyEntry(ctx, alice.ID, "de", aliceExtra.ID, 1, revision)
	require.NoError(t, err)
	assert.Equal(t, int64(3), revision, "move earlier")
	unchanged, moveErr := store.MoveReadingJourneyEntry(ctx, alice.ID, "de", aliceExtra.ID, 1, revision)
	require.NoError(t, moveErr)
	assert.Equal(t, revision, unchanged, "move current")
	revision, err = store.MoveReadingJourneyEntry(ctx, alice.ID, "de", aliceExtra.ID, 99, revision)
	require.NoError(t, err)
	assert.Equal(t, int64(4), revision, "move high clamp")
	revision, err = store.MoveReadingJourneyEntry(ctx, alice.ID, "de", aliceExtra.ID, 0, revision)
	require.NoError(t, err)
	assert.Equal(t, int64(5), revision, "move low clamp")
	_, err = store.MoveReadingJourneyEntry(ctx, alice.ID, "de", bobQueued.ID, 1, revision)
	assert.ErrorIs(t, err, ErrNotFound)
	journey, err = store.GetReadingJourney(ctx, alice.ID, "de")
	require.NoError(t, err)
	assertJourneyEntries(t, journey, alice.ID, []string{aliceExtra.ID, shared.ID})

	bobJourney, err = store.GetReadingJourney(ctx, bob.ID, "de")
	require.NoError(t, err)
	revision, err = store.AddToReadingJourney(ctx, bob.ID, "de", bobExtra.ID, bobJourney.Revision)
	require.NoError(t, err)
	assert.Equal(t, int64(1), revision, "add bob extra")
	bobJourney, err = store.GetReadingJourney(ctx, bob.ID, "de")
	require.NoError(t, err)
	assertJourneyEntries(t, bobJourney, bob.ID, []string{bobQueued.ID, bobExtra.ID})
	revision, err = store.RemoveFromReadingJourney(ctx, bob.ID, "de", bobExtra.ID, bobJourney.Revision)
	require.NoError(t, err)
	assert.Equal(t, int64(2), revision, "remove bob extra")
	journey, err = store.GetReadingJourney(ctx, alice.ID, "de")
	require.NoError(t, err)
	assertJourneyEntries(t, journey, alice.ID, []string{aliceExtra.ID, shared.ID})

	journeyRevision := journey.Revision
	results := make(chan readingJourneyMoveResult, 2)
	var wait sync.WaitGroup
	wait.Add(2)
	for i := 0; i < 2; i++ {
		go func() {
			defer wait.Done()
			result, moveErr := store.MoveReadingJourneyEntry(ctx, alice.ID, "de", aliceExtra.ID, 2, journeyRevision)
			results <- readingJourneyMoveResult{revision: result, err: moveErr}
		}()
	}
	wait.Wait()
	close(results)
	var successes, stale int
	for result := range results {
		switch {
		case result.err == nil:
			successes++
			assert.Equal(t, journeyRevision+1, result.revision, "successful concurrent move")
		case errors.Is(result.err, ErrJourneyStale):
			stale++
		default:
			require.NoError(t, result.err, "concurrent move")
		}
	}
	assert.Equal(t, 1, successes, "concurrent move results")
	assert.Equal(t, 1, stale, "concurrent move results")
	journey, err = store.GetReadingJourney(ctx, alice.ID, "de")
	require.NoError(t, err)
	assertJourneyEntries(t, journey, alice.ID, []string{shared.ID, aliceExtra.ID})
}

func TestReadingJourneyLanguageIsolationAndLazyLifecycle(t *testing.T) {
	ctx := context.Background()
	databaseURL, pool := testutil.Postgres(t, ctx, Migrate)
	store, err := Open(ctx, databaseURL)
	require.NoError(t, err)
	defer store.Close()

	owner, err := store.CreateUser(ctx, "journey-language-isolation", false)
	require.NoError(t, err)
	deBook, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "German", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	deSecond, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "Second German", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	itBook, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "Italian", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "it"})
	require.NoError(t, err)
	unknown, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "Needs language", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown})
	require.NoError(t, err)

	deJourney, err := store.GetReadingJourney(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Zero(t, deJourney.Revision, "initial German Journey")
	assert.Equal(t, "de", deJourney.Language, "initial German Journey")
	itJourney, err := store.GetReadingJourney(ctx, owner.ID, "it")
	require.NoError(t, err)
	assert.Zero(t, itJourney.Revision, "initial Italian Journey")
	assert.Equal(t, "it", itJourney.Language, "initial Italian Journey")
	_, err = store.AddToReadingJourney(ctx, owner.ID, "de", unknown.ID, 0)
	assert.ErrorIs(t, err, ErrBookLanguageRequired)
	var journeyRows int
	err = pool.QueryRow(ctx, `SELECT count(*) FROM reading_journeys WHERE owner_id=$1`, owner.ID).Scan(&journeyRows)
	require.NoError(t, err)
	assert.Zero(t, journeyRows, "failed add materialized Journey rows")

	revision, err := store.AddToReadingJourney(ctx, owner.ID, "de", deBook.ID, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(1), revision, "German add")
	revision, err = store.AddToReadingJourney(ctx, owner.ID, "it", itBook.ID, 0)
	require.NoError(t, err)
	assert.Equal(t, int64(1), revision, "Italian add")
	revision, err = store.AddToReadingJourney(ctx, owner.ID, "de", deSecond.ID, 1)
	require.NoError(t, err)
	assert.Equal(t, int64(2), revision, "second German add")
	itJourney, err = store.GetReadingJourney(ctx, owner.ID, "it")
	require.NoError(t, err)
	assert.Equal(t, int64(1), itJourney.Revision, "Italian changed after German mutation")
	require.Len(t, itJourney.Entries, 1)
	assert.Equal(t, itBook.ID, itJourney.Entries[0].BookID, "Italian changed after German mutation")
	deJourney, err = store.GetReadingJourney(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Equal(t, int64(2), deJourney.Revision, "German isolation result")
	require.Len(t, deJourney.Entries, 2)
	assert.Equal(t, "de", deJourney.Entries[0].Language, "German isolation result")

	// Retagging a member out of a language makes it invisible immediately; the
	// next mutation cleans up the stale membership and can remove the Journey.
	_, err = store.UpdateBookMetadata(ctx, owner.ID, deBook.ID, deBook.Title, domain.LanguageUnknown, "")
	require.NoError(t, err)
	_, err = store.RemoveFromReadingJourney(ctx, owner.ID, "de", deBook.ID, deJourney.Revision)
	require.NoError(t, err)
	err = pool.QueryRow(ctx, `SELECT count(*) FROM reading_journeys WHERE owner_id=$1 AND language='de'`, owner.ID).Scan(&journeyRows)
	require.NoError(t, err)
	assert.Equal(t, 1, journeyRows, "German Journey was removed while another German Book remained")
	_, err = store.UpdateBookMetadata(ctx, owner.ID, deSecond.ID, deSecond.Title, domain.LanguageUnknown, "")
	require.NoError(t, err)
	deJourney, err = store.GetReadingJourney(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Empty(t, deJourney.Entries, "cleaned German Journey")
	_, err = store.RemoveFromReadingJourney(ctx, owner.ID, "de", deSecond.ID, deJourney.Revision)
	require.NoError(t, err)
	err = pool.QueryRow(ctx, `SELECT count(*) FROM reading_journeys WHERE owner_id=$1 AND language='de'`, owner.ID).Scan(&journeyRows)
	require.NoError(t, err)
	assert.Zero(t, journeyRows, "empty non-derived German Journey remained")
}

type readingJourneyMoveResult struct {
	revision int64
	err      error
}

// TestResolveJourneyBookID pins the identity resolution between the deck
// preparation surfaces (keyed by source_materials.id) and Reading Journey
// membership (keyed by books.id): a source material must resolve to its linked
// book, legacy sources resolve through the source-identifier alias, and ids
// without a book identity must report "not found" rather than failing an add.
func TestResolveJourneyBookID(t *testing.T) {
	ctx := context.Background()
	url := integrationDatabase(t, ctx)
	store, err := Open(ctx, url)
	require.NoError(t, err)
	defer store.Close()

	alice, err := store.CreateUser(ctx, "resolve-book-owner", false)
	require.NoError(t, err)
	bob, err := store.CreateUser(ctx, "resolve-book-other", false)
	require.NoError(t, err)
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Identity book", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	source := putBookSource(t, ctx, store, alice.ID, "resolve-identifier", "Identity source", []byte("resolve-source"), "resolve-source")
	err = store.LinkSourceToBook(ctx, alice.ID, book.ID, source.ID)
	require.NoError(t, err)

	// A book id is already the canonical Journey identity.
	id, ok, resolveErr := store.ResolveJourneyBookID(ctx, alice.ID, book.ID)
	require.NoError(t, resolveErr)
	assert.True(t, ok)
	assert.Equal(t, book.ID, id)
	// A linked source material resolves to its book id.
	id, ok, resolveErr = store.ResolveJourneyBookID(ctx, alice.ID, source.ID)
	require.NoError(t, resolveErr)
	assert.True(t, ok)
	assert.Equal(t, book.ID, id)

	// A legacy source with no source_materials.book_id resolves through the
	// source-identifier alias.
	legacyID := insertLegacySource(t, ctx, store.Pool(), alice.ID, "de", "legacy-resolve-identifier", "Legacy source", []byte("legacy"))
	legacyBook, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Legacy identity book", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown})
	require.NoError(t, err)
	err = store.AddBookAlias(ctx, alice.ID, legacyBook.ID, domain.AliasStrongBibliographic, domain.NamespaceSourceIdentifier, "legacy-resolve-identifier")
	require.NoError(t, err)
	id, ok, resolveErr = store.ResolveJourneyBookID(ctx, alice.ID, legacyID)
	require.NoError(t, resolveErr)
	assert.True(t, ok)
	assert.Equal(t, legacyBook.ID, id)

	// Unlinked sources, unknown ids, and cross-owner ids have no book identity.
	orphanID := insertLegacySource(t, ctx, store.Pool(), alice.ID, "de", "orphan-resolve-identifier", "Orphan source", []byte("orphan"))
	id, ok, resolveErr = store.ResolveJourneyBookID(ctx, alice.ID, orphanID)
	require.NoError(t, resolveErr)
	assert.False(t, ok)
	assert.Empty(t, id)
	id, ok, resolveErr = store.ResolveJourneyBookID(ctx, alice.ID, "00000000-0000-0000-0000-000000000000")
	require.NoError(t, resolveErr)
	assert.False(t, ok)
	assert.Empty(t, id)
	id, ok, resolveErr = store.ResolveJourneyBookID(ctx, bob.ID, source.ID)
	require.NoError(t, resolveErr)
	assert.False(t, ok)
	assert.Empty(t, id)

	// The resolved book id is the identity an add persists.
	revision, addErr := store.AddToReadingJourney(ctx, alice.ID, "de", book.ID, 0)
	require.NoError(t, addErr)
	assert.Equal(t, int64(1), revision, "add resolved book")
}

func createJourneyFixture(t *testing.T, ctx context.Context, store *PostgresStore, owner, suffix string) (domain.Book, domain.SourceMaterial, domain.DeckPreparation) {
	return createJourneyFixtureInLanguage(t, ctx, store, owner, "de", suffix)
}

func createJourneyFixtureInLanguage(t *testing.T, ctx context.Context, store *PostgresStore, owner, language, suffix string) (domain.Book, domain.SourceMaterial, domain.DeckPreparation) {
	t.Helper()
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: owner, Title: "Journey " + suffix, MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: language})
	require.NoError(t, err)
	return createJourneySourceAndPreparation(t, ctx, store, owner, book, suffix)
}

func createJourneySourceAndPreparation(t *testing.T, ctx context.Context, store *PostgresStore, owner string, book domain.Book, suffix string) (domain.Book, domain.SourceMaterial, domain.DeckPreparation) {
	return createJourneySourceAndPreparationInLanguage(t, ctx, store, owner, book, book.LanguageTag, suffix)
}

func createJourneySourceAndPreparationInLanguage(t *testing.T, ctx context.Context, store *PostgresStore, owner string, book domain.Book, language, suffix string) (domain.Book, domain.SourceMaterial, domain.DeckPreparation) {
	t.Helper()
	source := putBookSourceInLanguage(t, ctx, store, owner, language, "journey-"+suffix, "Journey "+suffix, []byte("journey-"+suffix), suffix)
	err := store.LinkSourceToBook(ctx, owner, book.ID, source.ID)
	require.NoError(t, err)
	prep, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner, SourceMaterialID: source.ID, Filename: suffix + ".apkg", DeckName: "Journey " + suffix, ContentHash: source.ContentHash})
	require.NoError(t, err)
	prep, err = store.ClaimDeckPreparation(ctx, owner, prep.ID)
	require.NoError(t, err)
	prep, err = store.CompleteDeckPreparation(ctx, owner, prep.ID, domain.DeckPreparation{Artifact: []byte("apkg-" + suffix), Filename: suffix + ".apkg", DeckName: "Journey " + suffix})
	require.NoError(t, err)
	return book, source, prep
}

func makeJourneyMemberAnalyzed(t *testing.T, ctx context.Context, store *PostgresStore, book domain.Book, source domain.SourceMaterial) {
	t.Helper()
	journey, err := store.GetReadingJourney(ctx, book.OwnerID, "de")
	require.NoError(t, err)
	_, err = store.AddToReadingJourney(ctx, book.OwnerID, "de", book.ID, journey.Revision)
	require.NoError(t, err)
	err = store.PutArtifact(ctx, domain.NormalizedArtifact{
		ContentHash:          source.ContentHash,
		Language:             source.Language,
		SchemaVersion:        "1",
		NormalizationProfile: source.Language,
		NormalizationVersion: "1",
		AnalyzerName:         "test",
		AnalyzerVersion:      "1",
	}, nil)
	require.NoError(t, err)
	var snapshotID string
	err = store.Pool().QueryRow(ctx, `SELECT current_snapshot_id::text FROM source_materials WHERE owner_id=$1 AND id=$2`, source.OwnerID, source.ID).Scan(&snapshotID)
	require.NoError(t, err)
	var runID string
	err = store.Pool().QueryRow(ctx, `INSERT INTO analysis_runs(owner_id,source_material_id,content_revision_id,snapshot_id,analyzer_name,analyzer_version,config_identity,state,completed_at) VALUES($1,$2,$3,$4,'test','1',$5,'completed',now()) RETURNING id::text`, source.OwnerID, source.ID, source.ContentRevisionID, snapshotID, source.ID).Scan(&runID)
	require.NoError(t, err)
	corpus, err := store.PutCorpus(ctx, source.OwnerID, source.ID, source.ContentHash)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE corpora SET analysis_run_id=$1,status='complete' WHERE owner_id=$2 AND id=$3`, runID, source.OwnerID, corpus.ID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE analysis_runs SET corpus_id=$1 WHERE owner_id=$2 AND id=$3`, corpus.ID, source.OwnerID, runID)
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `INSERT INTO book_current_analyses(owner_id,book_id,source_material_id,analysis_run_id) VALUES($1,$2,$3,$4)`, book.OwnerID, book.ID, source.ID, runID)
	require.NoError(t, err)
}

func assertJourneyEntries(t *testing.T, journey domain.ReadingJourney, owner string, bookIDs []string) {
	t.Helper()
	assert.Equal(t, owner, journey.OwnerID)
	require.Len(t, journey.Entries, len(bookIDs), "journey=%+v want owner=%q books=%v", journey, owner, bookIDs)
	for i, entry := range journey.Entries {
		assert.Equal(t, owner, entry.OwnerID, "journey entry[%d]", i)
		assert.Equal(t, bookIDs[i], entry.BookID, "journey entry[%d]", i)
		assert.Equal(t, i+1, entry.Position, "journey entry[%d]", i)
	}
}
