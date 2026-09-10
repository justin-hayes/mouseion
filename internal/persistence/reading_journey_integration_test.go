//go:build integration

package persistence

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
)

func TestReadingJourneyBackfillAndPersistence(t *testing.T) {
	ctx := context.Background()
	url, pool := testutil.Postgres(t, ctx, Migrate)
	store, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	alice, err := store.CreateUser(ctx, "journey-alice", false)
	if err != nil {
		t.Fatal(err)
	}
	bob, err := store.CreateUser(ctx, "journey-bob", false)
	if err != nil {
		t.Fatal(err)
	}

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
		if _, err = pool.Exec(ctx, `INSERT INTO reading_journeys(owner_id,language) VALUES($1,$2) ON CONFLICT DO NOTHING`, item.owner, item.language); err != nil {
			t.Fatal(err)
		}
		if _, err = pool.Exec(ctx, `INSERT INTO reading_journey_membership(owner_id,language,book_id,position) VALUES($1,$2,$3,$4)`, item.owner, item.language, item.bookID, item.position); err != nil {
			t.Fatal(err)
		}
	}
	journey, err := store.GetReadingJourney(ctx, alice.ID, "de")
	if err != nil {
		t.Fatal(err)
	}
	italianJourney, err := store.GetReadingJourney(ctx, alice.ID, "it")
	if err != nil {
		t.Fatal(err)
	}
	bobJourney, err := store.GetReadingJourney(ctx, bob.ID, "de")
	if err != nil {
		t.Fatal(err)
	}
	assertJourneyEntries(t, journey, alice.ID, []string{queuedEarly.ID, shared.ID})
	italianJourney, err = store.GetReadingJourney(ctx, alice.ID, "it")
	if err != nil {
		t.Fatal(err)
	}
	assertJourneyEntries(t, italianJourney, alice.ID, []string{italian.ID})
	bobJourney, err = store.GetReadingJourney(ctx, bob.ID, "de")
	if err != nil {
		t.Fatal(err)
	}
	assertJourneyEntries(t, bobJourney, bob.ID, []string{bobQueued.ID})

	store.Close()
	store, err = Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	carol, err := store.CreateUser(ctx, "journey-carol", false)
	if err != nil {
		t.Fatal(err)
	}
	empty, err := store.GetReadingJourney(ctx, carol.ID, "de")
	if err != nil || empty.Revision != 0 || len(empty.Entries) != 0 || empty.OwnerID != carol.ID {
		t.Fatalf("empty journey=%+v err=%v", empty, err)
	}

	aliceExtra, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Alice extra", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	if err != nil {
		t.Fatal(err)
	}
	bobExtra, err := store.CreateBook(ctx, domain.Book{OwnerID: bob.ID, Title: "Bob extra", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	if err != nil {
		t.Fatal(err)
	}

	journey, err = store.GetReadingJourney(ctx, alice.ID, "de")
	if err != nil {
		t.Fatal(err)
	}
	revision, err := store.AddToReadingJourney(ctx, alice.ID, "de", aliceExtra.ID, journey.Revision)
	if err != nil || revision != 1 {
		t.Fatalf("add alice extra revision=%d err=%v", revision, err)
	}
	if repeatedRevision, repeatErr := store.AddToReadingJourney(ctx, alice.ID, "de", aliceExtra.ID, revision); repeatErr != nil || repeatedRevision != revision {
		t.Fatalf("repeated add revision=%d want=%d err=%v", repeatedRevision, revision, repeatErr)
	}
	if _, err = store.AddToReadingJourney(ctx, alice.ID, "de", aliceExtra.ID, 0); !errors.Is(err, ErrJourneyStale) {
		t.Fatalf("stale add error=%v", err)
	}
	if _, err = store.AddToReadingJourney(ctx, alice.ID, "de", bobQueued.ID, revision); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner add error=%v", err)
	}

	if revision, err = store.RemoveFromReadingJourney(ctx, alice.ID, "de", queuedEarly.ID, revision); err != nil || revision != 2 {
		t.Fatalf("remove alice early revision=%d err=%v", revision, err)
	}
	journey, err = store.GetReadingJourney(ctx, alice.ID, "de")
	if err != nil {
		t.Fatal(err)
	}
	assertJourneyEntries(t, journey, alice.ID, []string{shared.ID, aliceExtra.ID})
	if revision, err = store.RemoveFromReadingJourney(ctx, alice.ID, "de", active.ID, revision); err != nil || revision != 2 {
		t.Fatalf("remove absent revision=%d err=%v", revision, err)
	}

	if revision, err = store.MoveReadingJourneyEntry(ctx, alice.ID, "de", aliceExtra.ID, 1, revision); err != nil || revision != 3 {
		t.Fatalf("move earlier revision=%d err=%v", revision, err)
	}
	if unchanged, moveErr := store.MoveReadingJourneyEntry(ctx, alice.ID, "de", aliceExtra.ID, 1, revision); moveErr != nil || unchanged != revision {
		t.Fatalf("move current revision=%d want=%d err=%v", unchanged, revision, moveErr)
	}
	if revision, err = store.MoveReadingJourneyEntry(ctx, alice.ID, "de", aliceExtra.ID, 99, revision); err != nil || revision != 4 {
		t.Fatalf("move high clamp revision=%d err=%v", revision, err)
	}
	if revision, err = store.MoveReadingJourneyEntry(ctx, alice.ID, "de", aliceExtra.ID, 0, revision); err != nil || revision != 5 {
		t.Fatalf("move low clamp revision=%d err=%v", revision, err)
	}
	if _, err = store.MoveReadingJourneyEntry(ctx, alice.ID, "de", bobQueued.ID, 1, revision); !errors.Is(err, ErrNotFound) {
		t.Fatalf("move absent error=%v", err)
	}
	journey, err = store.GetReadingJourney(ctx, alice.ID, "de")
	if err != nil {
		t.Fatal(err)
	}
	assertJourneyEntries(t, journey, alice.ID, []string{aliceExtra.ID, shared.ID})

	bobJourney, err = store.GetReadingJourney(ctx, bob.ID, "de")
	if err != nil {
		t.Fatal(err)
	}
	if revision, err = store.AddToReadingJourney(ctx, bob.ID, "de", bobExtra.ID, bobJourney.Revision); err != nil || revision != 1 {
		t.Fatalf("add bob extra revision=%d err=%v", revision, err)
	}
	bobJourney, err = store.GetReadingJourney(ctx, bob.ID, "de")
	if err != nil {
		t.Fatal(err)
	}
	assertJourneyEntries(t, bobJourney, bob.ID, []string{bobQueued.ID, bobExtra.ID})
	if revision, err = store.RemoveFromReadingJourney(ctx, bob.ID, "de", bobExtra.ID, bobJourney.Revision); err != nil || revision != 2 {
		t.Fatalf("remove bob extra revision=%d err=%v", revision, err)
	}
	journey, err = store.GetReadingJourney(ctx, alice.ID, "de")
	if err != nil {
		t.Fatal(err)
	}
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
		if result.err == nil {
			successes++
			if result.revision != journeyRevision+1 {
				t.Fatalf("successful concurrent move revision=%d want=%d", result.revision, journeyRevision+1)
			}
		} else if errors.Is(result.err, ErrJourneyStale) {
			stale++
		} else {
			t.Fatalf("concurrent move error=%v", result.err)
		}
	}
	if successes != 1 || stale != 1 {
		t.Fatalf("concurrent move results successes=%d stale=%d", successes, stale)
	}
	journey, err = store.GetReadingJourney(ctx, alice.ID, "de")
	if err != nil {
		t.Fatal(err)
	}
	assertJourneyEntries(t, journey, alice.ID, []string{shared.ID, aliceExtra.ID})
}

func TestReadingJourneyLanguageIsolationAndLazyLifecycle(t *testing.T) {
	ctx := context.Background()
	databaseURL, pool := testutil.Postgres(t, ctx, Migrate)
	store, err := Open(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	owner, err := store.CreateUser(ctx, "journey-language-isolation", false)
	if err != nil {
		t.Fatal(err)
	}
	deBook, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "German", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	if err != nil {
		t.Fatal(err)
	}
	deSecond, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "Second German", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	if err != nil {
		t.Fatal(err)
	}
	itBook, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "Italian", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "it"})
	if err != nil {
		t.Fatal(err)
	}
	unknown, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "Needs language", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown})
	if err != nil {
		t.Fatal(err)
	}

	deJourney, err := store.GetReadingJourney(ctx, owner.ID, "de")
	if err != nil || deJourney.Revision != 0 || deJourney.Language != "de" {
		t.Fatalf("initial German Journey=%+v err=%v", deJourney, err)
	}
	itJourney, err := store.GetReadingJourney(ctx, owner.ID, "it")
	if err != nil || itJourney.Revision != 0 || itJourney.Language != "it" {
		t.Fatalf("initial Italian Journey=%+v err=%v", itJourney, err)
	}
	if _, err = store.AddToReadingJourney(ctx, owner.ID, "de", unknown.ID, 0); !errors.Is(err, ErrBookLanguageRequired) {
		t.Fatalf("unknown-language add error=%v", err)
	}
	var journeyRows int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM reading_journeys WHERE owner_id=$1`, owner.ID).Scan(&journeyRows); err != nil {
		t.Fatal(err)
	}
	if journeyRows != 0 {
		t.Fatalf("failed add materialized %d Journey rows", journeyRows)
	}

	if revision, err := store.AddToReadingJourney(ctx, owner.ID, "de", deBook.ID, 0); err != nil || revision != 1 {
		t.Fatalf("German add revision=%d err=%v", revision, err)
	}
	if revision, err := store.AddToReadingJourney(ctx, owner.ID, "it", itBook.ID, 0); err != nil || revision != 1 {
		t.Fatalf("Italian add revision=%d err=%v", revision, err)
	}
	if revision, err := store.AddToReadingJourney(ctx, owner.ID, "de", deSecond.ID, 1); err != nil || revision != 2 {
		t.Fatalf("second German add revision=%d err=%v", revision, err)
	}
	itJourney, err = store.GetReadingJourney(ctx, owner.ID, "it")
	if err != nil || itJourney.Revision != 1 || len(itJourney.Entries) != 1 || itJourney.Entries[0].BookID != itBook.ID {
		t.Fatalf("Italian changed after German mutation=%+v err=%v", itJourney, err)
	}
	deJourney, err = store.GetReadingJourney(ctx, owner.ID, "de")
	if err != nil || deJourney.Revision != 2 || len(deJourney.Entries) != 2 || deJourney.Entries[0].Language != "de" {
		t.Fatalf("German isolation result=%+v err=%v", deJourney, err)
	}

	// Retagging a member out of a language makes it invisible immediately; the
	// next mutation cleans up the stale membership and can remove the Journey.
	if _, err = store.UpdateBookMetadata(ctx, owner.ID, deBook.ID, deBook.Title, domain.LanguageUnknown, ""); err != nil {
		t.Fatal(err)
	}
	if _, err = store.RemoveFromReadingJourney(ctx, owner.ID, "de", deBook.ID, deJourney.Revision); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM reading_journeys WHERE owner_id=$1 AND language='de'`, owner.ID).Scan(&journeyRows); err != nil {
		t.Fatal(err)
	}
	if journeyRows != 1 {
		t.Fatalf("German Journey was removed while another German Book remained: %d", journeyRows)
	}
	if _, err = store.UpdateBookMetadata(ctx, owner.ID, deSecond.ID, deSecond.Title, domain.LanguageUnknown, ""); err != nil {
		t.Fatal(err)
	}
	deJourney, err = store.GetReadingJourney(ctx, owner.ID, "de")
	if err != nil || len(deJourney.Entries) != 0 {
		t.Fatalf("cleaned German Journey=%+v err=%v", deJourney, err)
	}
	if _, err = store.RemoveFromReadingJourney(ctx, owner.ID, "de", deSecond.ID, deJourney.Revision); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM reading_journeys WHERE owner_id=$1 AND language='de'`, owner.ID).Scan(&journeyRows); err != nil {
		t.Fatal(err)
	}
	if journeyRows != 0 {
		t.Fatalf("empty non-derived German Journey remained: %d", journeyRows)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	alice, err := store.CreateUser(ctx, "resolve-book-owner", false)
	if err != nil {
		t.Fatal(err)
	}
	bob, err := store.CreateUser(ctx, "resolve-book-other", false)
	if err != nil {
		t.Fatal(err)
	}
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Identity book", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	if err != nil {
		t.Fatal(err)
	}
	source := putBookSource(t, ctx, store, alice.ID, "resolve-identifier", "Identity source", []byte("resolve-source"), "resolve-source")
	if err := store.LinkSourceToBook(ctx, alice.ID, book.ID, source.ID); err != nil {
		t.Fatal(err)
	}

	// A book id is already the canonical Journey identity.
	if id, ok, resolveErr := store.ResolveJourneyBookID(ctx, alice.ID, book.ID); resolveErr != nil || !ok || id != book.ID {
		t.Fatalf("book id resolve=(%q,%t) err=%v", id, ok, resolveErr)
	}
	// A linked source material resolves to its book id.
	if id, ok, resolveErr := store.ResolveJourneyBookID(ctx, alice.ID, source.ID); resolveErr != nil || !ok || id != book.ID {
		t.Fatalf("linked source resolve=(%q,%t) err=%v", id, ok, resolveErr)
	}

	// A legacy source with no source_materials.book_id resolves through the
	// source-identifier alias.
	legacyID := insertLegacySource(t, ctx, store.Pool(), alice.ID, "de", "legacy-resolve-identifier", "Legacy source", []byte("legacy"))
	legacyBook, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Legacy identity book", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddBookAlias(ctx, alice.ID, legacyBook.ID, domain.AliasStrongBibliographic, domain.NamespaceSourceIdentifier, "legacy-resolve-identifier"); err != nil {
		t.Fatal(err)
	}
	if id, ok, resolveErr := store.ResolveJourneyBookID(ctx, alice.ID, legacyID); resolveErr != nil || !ok || id != legacyBook.ID {
		t.Fatalf("legacy alias resolve=(%q,%t) err=%v", id, ok, resolveErr)
	}

	// Unlinked sources, unknown ids, and cross-owner ids have no book identity.
	orphanID := insertLegacySource(t, ctx, store.Pool(), alice.ID, "de", "orphan-resolve-identifier", "Orphan source", []byte("orphan"))
	if id, ok, resolveErr := store.ResolveJourneyBookID(ctx, alice.ID, orphanID); resolveErr != nil || ok || id != "" {
		t.Fatalf("orphan resolve=(%q,%t) err=%v", id, ok, resolveErr)
	}
	if id, ok, resolveErr := store.ResolveJourneyBookID(ctx, alice.ID, "00000000-0000-0000-0000-000000000000"); resolveErr != nil || ok || id != "" {
		t.Fatalf("unknown resolve=(%q,%t) err=%v", id, ok, resolveErr)
	}
	if id, ok, resolveErr := store.ResolveJourneyBookID(ctx, bob.ID, source.ID); resolveErr != nil || ok || id != "" {
		t.Fatalf("cross-owner resolve=(%q,%t) err=%v", id, ok, resolveErr)
	}

	// The resolved book id is the identity an add persists.
	if revision, addErr := store.AddToReadingJourney(ctx, alice.ID, "de", book.ID, 0); addErr != nil || revision != 1 {
		t.Fatalf("add resolved book revision=%d err=%v", revision, addErr)
	}
}

func createJourneyFixture(t *testing.T, ctx context.Context, store *PostgresStore, owner, suffix string) (domain.Book, domain.SourceMaterial, domain.DeckPreparation) {
	return createJourneyFixtureInLanguage(t, ctx, store, owner, "de", suffix)
}

func createJourneyFixtureInLanguage(t *testing.T, ctx context.Context, store *PostgresStore, owner, language, suffix string) (domain.Book, domain.SourceMaterial, domain.DeckPreparation) {
	t.Helper()
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: owner, Title: "Journey " + suffix, MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: language})
	if err != nil {
		t.Fatal(err)
	}
	return createJourneySourceAndPreparation(t, ctx, store, owner, book, suffix)
}

func createJourneySourceAndPreparation(t *testing.T, ctx context.Context, store *PostgresStore, owner string, book domain.Book, suffix string) (domain.Book, domain.SourceMaterial, domain.DeckPreparation) {
	return createJourneySourceAndPreparationInLanguage(t, ctx, store, owner, book, book.LanguageTag, suffix)
}

func createJourneySourceAndPreparationInLanguage(t *testing.T, ctx context.Context, store *PostgresStore, owner string, book domain.Book, language, suffix string) (domain.Book, domain.SourceMaterial, domain.DeckPreparation) {
	t.Helper()
	source := putBookSourceInLanguage(t, ctx, store, owner, language, "journey-"+suffix, "Journey "+suffix, []byte("journey-"+suffix), suffix)
	if err := store.LinkSourceToBook(ctx, owner, book.ID, source.ID); err != nil {
		t.Fatal(err)
	}
	prep, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner, SourceMaterialID: source.ID, Filename: suffix + ".apkg", DeckName: "Journey " + suffix, ContentHash: source.ContentHash})
	if err != nil {
		t.Fatal(err)
	}
	prep, err = store.ClaimDeckPreparation(ctx, owner, prep.ID)
	if err != nil {
		t.Fatal(err)
	}
	prep, err = store.CompleteDeckPreparation(ctx, owner, prep.ID, domain.DeckPreparation{Artifact: []byte("apkg-" + suffix), Filename: suffix + ".apkg", DeckName: "Journey " + suffix})
	if err != nil {
		t.Fatal(err)
	}
	return book, source, prep
}

func makeJourneyMemberAnalyzed(t *testing.T, ctx context.Context, store *PostgresStore, book domain.Book, source domain.SourceMaterial) {
	t.Helper()
	journey, err := store.GetReadingJourney(ctx, book.OwnerID, "de")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.AddToReadingJourney(ctx, book.OwnerID, "de", book.ID, journey.Revision); err != nil {
		t.Fatal(err)
	}
	if err := store.PutArtifact(ctx, domain.NormalizedArtifact{
		ContentHash:          source.ContentHash,
		Language:             source.Language,
		SchemaVersion:        "1",
		NormalizationProfile: source.Language,
		NormalizationVersion: "1",
		AnalyzerName:         "test",
		AnalyzerVersion:      "1",
	}, nil); err != nil {
		t.Fatal(err)
	}
	var snapshotID string
	if err := store.Pool().QueryRow(ctx, `SELECT current_snapshot_id::text FROM source_materials WHERE owner_id=$1 AND id=$2`, source.OwnerID, source.ID).Scan(&snapshotID); err != nil {
		t.Fatal(err)
	}
	var runID string
	if err := store.Pool().QueryRow(ctx, `INSERT INTO analysis_runs(owner_id,source_material_id,content_revision_id,snapshot_id,analyzer_name,analyzer_version,config_identity,state,completed_at) VALUES($1,$2,$3,$4,'test','1',$5,'completed',now()) RETURNING id::text`, source.OwnerID, source.ID, source.ContentRevisionID, snapshotID, source.ID).Scan(&runID); err != nil {
		t.Fatal(err)
	}
	corpus, err := store.PutCorpus(ctx, source.OwnerID, source.ID, source.ContentHash)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.Pool().Exec(ctx, `UPDATE corpora SET analysis_run_id=$1,status='complete' WHERE owner_id=$2 AND id=$3`, runID, source.OwnerID, corpus.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Pool().Exec(ctx, `UPDATE analysis_runs SET corpus_id=$1 WHERE owner_id=$2 AND id=$3`, corpus.ID, source.OwnerID, runID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Pool().Exec(ctx, `INSERT INTO book_current_analyses(owner_id,book_id,source_material_id,analysis_run_id) VALUES($1,$2,$3,$4)`, book.OwnerID, book.ID, source.ID, runID); err != nil {
		t.Fatal(err)
	}
}

func assertJourneyEntries(t *testing.T, journey domain.ReadingJourney, owner string, bookIDs []string) {
	t.Helper()
	if journey.OwnerID != owner || len(journey.Entries) != len(bookIDs) {
		t.Fatalf("journey=%+v want owner=%q books=%v", journey, owner, bookIDs)
	}
	for i, entry := range journey.Entries {
		if entry.OwnerID != owner || entry.BookID != bookIDs[i] || entry.Position != i+1 {
			t.Fatalf("journey entry[%d]=%+v want owner=%q book=%q position=%d", i, entry, owner, bookIDs[i], i+1)
		}
	}
}
