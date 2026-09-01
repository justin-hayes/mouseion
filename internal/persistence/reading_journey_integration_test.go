//go:build integration

package persistence

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
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

	queuedEarly, queuedEarlySource, queuedEarlyPrep := createJourneyFixture(t, ctx, store, alice.ID, "queued-early")
	shared, sharedFirstSource, sharedFirstPrep := createJourneyFixture(t, ctx, store, alice.ID, "queued-shared-first")
	_, sharedSecondSource, sharedSecondPrep := createJourneySourceAndPreparation(t, ctx, store, alice.ID, shared, "queued-shared-second")
	active, activeSource, activePrep := createJourneyFixture(t, ctx, store, alice.ID, "active")
	_, completeSource, completePrep := createJourneyFixture(t, ctx, store, alice.ID, "complete")
	_, abandonedSource, abandonedPrep := createJourneyFixture(t, ctx, store, alice.ID, "abandoned")
	bobQueued, bobSource, bobPrep := createJourneyFixture(t, ctx, store, bob.ID, "bob-queued")

	base := time.Date(2026, time.January, 1, 0, 0, 0, 0, time.UTC)
	insertJourneyCampaign(t, ctx, pool, alice.ID, queuedEarlySource.ID, queuedEarlyPrep.ID, "queued", "queued", base)
	insertJourneyCampaign(t, ctx, pool, alice.ID, sharedFirstSource.ID, sharedFirstPrep.ID, "queued", "queued", base.Add(24*time.Hour))
	insertJourneyCampaign(t, ctx, pool, alice.ID, sharedSecondSource.ID, sharedSecondPrep.ID, "queued", "queued", base.Add(48*time.Hour))
	insertJourneyCampaign(t, ctx, pool, alice.ID, activeSource.ID, activePrep.ID, "reading", "studying", base.Add(72*time.Hour))
	insertJourneyCampaign(t, ctx, pool, alice.ID, completeSource.ID, completePrep.ID, "finished", "reviewed", base.Add(96*time.Hour))
	insertJourneyCampaign(t, ctx, pool, alice.ID, abandonedSource.ID, abandonedPrep.ID, "abandoned", "queued", base.Add(120*time.Hour))
	insertJourneyCampaign(t, ctx, pool, bob.ID, bobSource.ID, bobPrep.ID, "queued", "queued", base.Add(24*time.Hour))

	var beforeCampaigns, beforeVocabulary int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM learning_campaigns`).Scan(&beforeCampaigns); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM learning_campaign_vocabulary`).Scan(&beforeVocabulary); err != nil {
		t.Fatal(err)
	}

	backfill := migrationSQL(t, "000039_reading_journey_backfill.up.sql")
	if _, err = pool.Exec(ctx, backfill); err != nil {
		t.Fatal(err)
	}
	journey, err := store.GetReadingJourney(ctx, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertJourneyEntries(t, journey, alice.ID, []string{queuedEarly.ID, shared.ID})
	bobJourney, err := store.GetReadingJourney(ctx, bob.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertJourneyEntries(t, bobJourney, bob.ID, []string{bobQueued.ID})

	if _, err = pool.Exec(ctx, backfill); err != nil {
		t.Fatal(err)
	}
	repeatedAlice, err := store.GetReadingJourney(ctx, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertJourneyEntries(t, repeatedAlice, alice.ID, []string{queuedEarly.ID, shared.ID})
	repeatedBob, err := store.GetReadingJourney(ctx, bob.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertJourneyEntries(t, repeatedBob, bob.ID, []string{bobQueued.ID})
	var afterCampaigns, afterVocabulary int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM learning_campaigns`).Scan(&afterCampaigns); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM learning_campaign_vocabulary`).Scan(&afterVocabulary); err != nil {
		t.Fatal(err)
	}
	if afterCampaigns != beforeCampaigns || afterVocabulary != beforeVocabulary {
		t.Fatalf("backfill changed campaign data: campaigns %d -> %d, vocabulary %d -> %d", beforeCampaigns, afterCampaigns, beforeVocabulary, afterVocabulary)
	}

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
	empty, err := store.GetReadingJourney(ctx, carol.ID)
	if err != nil || empty.Revision != 0 || len(empty.Entries) != 0 || empty.OwnerID != carol.ID {
		t.Fatalf("empty journey=%+v err=%v", empty, err)
	}

	aliceExtra, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Alice extra", MetadataProvenance: domain.MetadataProvenanceManualEntry, LanguageState: domain.LanguageUnknown})
	if err != nil {
		t.Fatal(err)
	}
	bobExtra, err := store.CreateBook(ctx, domain.Book{OwnerID: bob.ID, Title: "Bob extra", MetadataProvenance: domain.MetadataProvenanceManualEntry, LanguageState: domain.LanguageUnknown})
	if err != nil {
		t.Fatal(err)
	}

	journey, err = store.GetReadingJourney(ctx, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	revision, err := store.AddToReadingJourney(ctx, alice.ID, aliceExtra.ID, journey.Revision)
	if err != nil || revision != 1 {
		t.Fatalf("add alice extra revision=%d err=%v", revision, err)
	}
	if repeatedRevision, repeatErr := store.AddToReadingJourney(ctx, alice.ID, aliceExtra.ID, revision); repeatErr != nil || repeatedRevision != revision {
		t.Fatalf("repeated add revision=%d want=%d err=%v", repeatedRevision, revision, repeatErr)
	}
	if _, err = store.AddToReadingJourney(ctx, alice.ID, aliceExtra.ID, 0); !errors.Is(err, ErrJourneyStale) {
		t.Fatalf("stale add error=%v", err)
	}
	if _, err = store.AddToReadingJourney(ctx, alice.ID, bobQueued.ID, revision); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner add error=%v", err)
	}

	if revision, err = store.RemoveFromReadingJourney(ctx, alice.ID, queuedEarly.ID, revision); err != nil || revision != 2 {
		t.Fatalf("remove alice early revision=%d err=%v", revision, err)
	}
	journey, err = store.GetReadingJourney(ctx, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertJourneyEntries(t, journey, alice.ID, []string{shared.ID, aliceExtra.ID})
	if revision, err = store.RemoveFromReadingJourney(ctx, alice.ID, active.ID, revision); err != nil || revision != 2 {
		t.Fatalf("remove absent revision=%d err=%v", revision, err)
	}

	if revision, err = store.MoveReadingJourneyEntry(ctx, alice.ID, aliceExtra.ID, 1, revision); err != nil || revision != 3 {
		t.Fatalf("move earlier revision=%d err=%v", revision, err)
	}
	if unchanged, moveErr := store.MoveReadingJourneyEntry(ctx, alice.ID, aliceExtra.ID, 1, revision); moveErr != nil || unchanged != revision {
		t.Fatalf("move current revision=%d want=%d err=%v", unchanged, revision, moveErr)
	}
	if revision, err = store.MoveReadingJourneyEntry(ctx, alice.ID, aliceExtra.ID, 99, revision); err != nil || revision != 4 {
		t.Fatalf("move high clamp revision=%d err=%v", revision, err)
	}
	if revision, err = store.MoveReadingJourneyEntry(ctx, alice.ID, aliceExtra.ID, 0, revision); err != nil || revision != 5 {
		t.Fatalf("move low clamp revision=%d err=%v", revision, err)
	}
	if _, err = store.MoveReadingJourneyEntry(ctx, alice.ID, bobQueued.ID, 1, revision); !errors.Is(err, ErrNotFound) {
		t.Fatalf("move absent error=%v", err)
	}
	journey, err = store.GetReadingJourney(ctx, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertJourneyEntries(t, journey, alice.ID, []string{aliceExtra.ID, shared.ID})

	bobJourney, err = store.GetReadingJourney(ctx, bob.ID)
	if err != nil {
		t.Fatal(err)
	}
	if revision, err = store.AddToReadingJourney(ctx, bob.ID, bobExtra.ID, bobJourney.Revision); err != nil || revision != 1 {
		t.Fatalf("add bob extra revision=%d err=%v", revision, err)
	}
	bobJourney, err = store.GetReadingJourney(ctx, bob.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertJourneyEntries(t, bobJourney, bob.ID, []string{bobQueued.ID, bobExtra.ID})
	if revision, err = store.RemoveFromReadingJourney(ctx, bob.ID, bobExtra.ID, bobJourney.Revision); err != nil || revision != 2 {
		t.Fatalf("remove bob extra revision=%d err=%v", revision, err)
	}
	journey, err = store.GetReadingJourney(ctx, alice.ID)
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
			result, moveErr := store.MoveReadingJourneyEntry(ctx, alice.ID, aliceExtra.ID, 2, journeyRevision)
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
	journey, err = store.GetReadingJourney(ctx, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	assertJourneyEntries(t, journey, alice.ID, []string{shared.ID, aliceExtra.ID})
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
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Identity book", MetadataProvenance: domain.MetadataProvenanceManualEntry, LanguageState: domain.LanguageUnknown})
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
	legacyBook, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Legacy identity book", MetadataProvenance: domain.MetadataProvenanceManualEntry, LanguageState: domain.LanguageUnknown})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddBookAlias(ctx, alice.ID, legacyBook.ID, domain.AliasCatalogEntry, domain.NamespaceSourceIdentifier, "legacy-resolve-identifier"); err != nil {
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
	if revision, addErr := store.AddToReadingJourney(ctx, alice.ID, book.ID, 0); addErr != nil || revision != 1 {
		t.Fatalf("add resolved book revision=%d err=%v", revision, addErr)
	}
}

func createJourneyFixture(t *testing.T, ctx context.Context, store *PostgresStore, owner, suffix string) (domain.Book, domain.SourceMaterial, domain.DeckPreparation) {
	t.Helper()
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: owner, Title: "Journey " + suffix, MetadataProvenance: domain.MetadataProvenanceManualEntry, LanguageState: domain.LanguageUnknown})
	if err != nil {
		t.Fatal(err)
	}
	return createJourneySourceAndPreparation(t, ctx, store, owner, book, suffix)
}

func createJourneySourceAndPreparation(t *testing.T, ctx context.Context, store *PostgresStore, owner string, book domain.Book, suffix string) (domain.Book, domain.SourceMaterial, domain.DeckPreparation) {
	t.Helper()
	source := putBookSource(t, ctx, store, owner, "journey-"+suffix, "Journey "+suffix, []byte("journey-"+suffix), suffix)
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

func insertJourneyCampaign(t *testing.T, ctx context.Context, pool *pgxpool.Pool, owner, sourceID, preparationID, bookStatus, deckStatus string, createdAt time.Time) string {
	t.Helper()
	var id string
	err := pool.QueryRow(ctx, `INSERT INTO learning_campaigns(owner_id,source_material_id,deck_preparation_id,book_status,deck_status,created_at,book_finished_at,deck_reviewed_at,completed_at,abandoned_at)
	VALUES($1,$2,$3,$4,$5,$6::timestamptz,
	 CASE WHEN $4='finished' THEN $6::timestamptz ELSE NULL END,
	 CASE WHEN $5='reviewed' THEN $6::timestamptz ELSE NULL END,
	 CASE WHEN $4='finished' AND $5='reviewed' THEN $6::timestamptz ELSE NULL END,
	 CASE WHEN $4='abandoned' OR $5='abandoned' THEN $6::timestamptz ELSE NULL END)
	RETURNING id::text`, owner, sourceID, preparationID, bookStatus, deckStatus, createdAt).Scan(&id)
	if err != nil {
		t.Fatal(err)
	}
	return id
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
