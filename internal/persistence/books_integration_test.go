//go:build integration

package persistence

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/justin-hayes/mouseion/migrations"
)

func TestMyBooksPersistenceAndBackfill(t *testing.T) {
	ctx := context.Background()
	url, pool := testutil.Postgres(t, ctx, Migrate)
	store, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	alice, err := store.CreateUser(ctx, "books-alice", false)
	if err != nil {
		t.Fatal(err)
	}
	bob, err := store.CreateUser(ctx, "books-bob", false)
	if err != nil {
		t.Fatal(err)
	}

	legacySources := []struct {
		owner, language, identifier, title string
	}{
		{alice.ID, "de", "legacy-de", "Deutsches Buch"},
		{alice.ID, "it", "legacy-it", "Libro italiano"},
		{bob.ID, "fr", "legacy-fr", "Livre français"},
	}
	legacyIDs := make([]string, 0, len(legacySources))
	for i, source := range legacySources {
		id := insertLegacySource(t, ctx, pool, source.owner, source.language, source.identifier, source.title, []byte("legacy-epub-"+string(rune('a'+i))))
		legacyIDs = append(legacyIDs, id)
	}
	backfillUp := migrationSQL(t, "000037_my_books_backfill.up.sql")
	if _, err = pool.Exec(ctx, backfillUp); err != nil {
		t.Fatal(err)
	}
	// Re-running the data migration is intentionally a no-op for already linked sources.
	if _, err = pool.Exec(ctx, backfillUp); err != nil {
		t.Fatal(err)
	}

	books, err := store.ListMyBooks(ctx, alice.ID)
	if err != nil || len(books) != 2 {
		t.Fatalf("alice backfill books=%+v err=%v", books, err)
	}
	for _, source := range legacySources[:2] {
		book, found, resolveErr := store.ResolveBookByAlias(ctx, source.owner, domain.NamespaceSourceIdentifier, source.identifier)
		if resolveErr != nil || !found || book.LanguageState != domain.LanguageChosen || book.LanguageTag != source.language {
			t.Fatalf("backfill alias %q book=%+v found=%v err=%v", source.identifier, book, found, resolveErr)
		}
	}
	var linkedCount int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM source_materials WHERE book_id IS NOT NULL`).Scan(&linkedCount); err != nil || linkedCount != len(legacySources) {
		t.Fatalf("backfilled source links=%d err=%v", linkedCount, err)
	}

	metadataOnly, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Unacquired book", MetadataProvenance: "manual", LanguageState: domain.LanguageUnknown})
	if err != nil {
		t.Fatal(err)
	}
	books, err = store.ListMyBooks(ctx, alice.ID)
	if err != nil || !containsBook(books, metadataOnly.ID) {
		t.Fatalf("metadata-only book missing from My Books: books=%+v err=%v", books, err)
	}
	if _, err = store.CreateEPUBReviewedScope(ctx, domain.EPUBReviewedScopeSnapshot{OwnerID: alice.ID, SourceMaterialID: metadataOnly.ID}); !errors.Is(err, domain.ErrEPUBReviewedScopeUnavailable) {
		t.Fatalf("metadata-only scope creation error=%v", err)
	}

	first := putBookSource(t, ctx, store, alice.ID, "acquisition-source", "Acquired", []byte("epub-one"), "readable")
	bookID, err := store.ResolveOrCreateBookForAcquisition(ctx, alice.ID, first.SourceIdentifier, first.Language, first.Title)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.LinkSourceToBook(ctx, alice.ID, bookID, first.ID); err != nil {
		t.Fatal(err)
	}
	if _, found, err := store.FindSourceMaterialForAcquisition(ctx, alice.ID, first.SourceIdentifier, first.ContentHash); err != nil || !found {
		t.Fatalf("same acquisition was not found: found=%v err=%v", found, err)
	}
	if _, found, err := store.FindSourceMaterialForAcquisition(ctx, alice.ID, first.SourceIdentifier, domain.EPUBContentDigest([]byte("epub-two"))); err != nil || found {
		t.Fatalf("changed acquisition was reported present: found=%v err=%v", found, err)
	}
	repeated := putBookSource(t, ctx, store, alice.ID, "acquisition-source", "Acquired", []byte("epub-one"), "readable")
	repeatedBookID, err := store.ResolveOrCreateBookForAcquisition(ctx, alice.ID, repeated.SourceIdentifier, repeated.Language, repeated.Title)
	if err != nil || repeatedBookID != bookID || repeated.ID != first.ID {
		t.Fatalf("repeated acquisition source=%+v book=%q want source=%q book=%q err=%v", repeated, repeatedBookID, first.ID, bookID, err)
	}
	changed := putBookSource(t, ctx, store, alice.ID, "acquisition-source", "Acquired", []byte("epub-two"), "readable")
	changedBookID, err := store.ResolveOrCreateBookForAcquisition(ctx, alice.ID, changed.SourceIdentifier, changed.Language, changed.Title)
	if err != nil || changedBookID != bookID || changed.ID != first.ID {
		t.Fatalf("changed acquisition source=%+v book=%q want source=%q book=%q err=%v", changed, changedBookID, first.ID, bookID, err)
	}
	var revisions int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM source_content_revisions WHERE owner_id=$1 AND source_material_id=$2`, alice.ID, first.ID).Scan(&revisions); err != nil || revisions != 2 {
		t.Fatalf("acquisition revisions=%d err=%v", revisions, err)
	}

	otherSource := putBookSource(t, ctx, store, bob.ID, "acquisition-source", "Acquired", []byte("epub-one"), "readable")
	otherBookID, err := store.ResolveOrCreateBookForAcquisition(ctx, bob.ID, otherSource.SourceIdentifier, otherSource.Language, otherSource.Title)
	if err != nil {
		t.Fatal(err)
	}
	if otherBookID == bookID {
		t.Fatal("cross-owner acquisition reused a Book")
	}
	if _, err = store.GetBook(ctx, alice.ID, otherBookID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("alice accessed bob book: err=%v", err)
	}
	if err = store.LinkSourceToBook(ctx, bob.ID, otherBookID, otherSource.ID); err != nil {
		t.Fatal(err)
	}

	secondBook, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Second", MetadataProvenance: "manual", LanguageState: domain.LanguageUnknown})
	if err != nil {
		t.Fatal(err)
	}
	if err = store.AddBookAlias(ctx, alice.ID, bookID, domain.AliasStrongBibliographic, "isbn", "978-conflict"); err != nil {
		t.Fatal(err)
	}
	if err = store.AddBookAlias(ctx, alice.ID, secondBook.ID, domain.AliasStrongBibliographic, "isbn", "978-conflict"); !errors.Is(err, ErrAliasConflict) {
		t.Fatalf("alias conflict error=%v", err)
	}
	resolved, found, err := store.ResolveBookByAlias(ctx, alice.ID, "isbn", "978-conflict")
	if err != nil || !found || resolved.ID != bookID {
		t.Fatalf("alias was reassigned: book=%+v found=%v err=%v", resolved, found, err)
	}
	if err = store.LinkSourceToBook(ctx, alice.ID, secondBook.ID, first.ID); !errors.Is(err, ErrSourceBookConflict) {
		t.Fatalf("source conflict error=%v", err)
	}

	if err = store.RemoveBookFromMyBooks(ctx, alice.ID, bookID); err != nil {
		t.Fatal(err)
	}
	if books, err = store.ListMyBooks(ctx, alice.ID); err != nil || containsBook(books, bookID) {
		t.Fatalf("removed book still active: books=%+v err=%v", books, err)
	}
	if err = store.AddBookToMyBooks(ctx, alice.ID, bookID); err != nil {
		t.Fatal(err)
	}
	if restored, err := store.GetBook(ctx, alice.ID, bookID); err != nil || restored.ID != bookID {
		t.Fatalf("restored book=%+v err=%v", restored, err)
	}
	if _, found, err = store.ResolveBookByAlias(ctx, alice.ID, domain.NamespaceSourceIdentifier, "acquisition-source"); err != nil || !found {
		t.Fatalf("source alias lost after membership removal: found=%v err=%v", found, err)
	}

	if err = store.AddBookAlias(ctx, alice.ID, secondBook.ID, "invalid", "failure", "must-not-commit"); err == nil {
		t.Fatal("invalid alias was accepted")
	}
	var invalidAliases int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM book_aliases WHERE owner_id=$1 AND value='must-not-commit'`, alice.ID).Scan(&invalidAliases); err != nil || invalidAliases != 0 {
		t.Fatalf("failed alias left partial row count=%d err=%v", invalidAliases, err)
	}

	backfillDown := migrationSQL(t, "000037_my_books_backfill.down.sql")
	if _, err = pool.Exec(ctx, backfillDown); err != nil {
		t.Fatal(err)
	}
	for _, id := range legacyIDs {
		var bookID *string
		var content []byte
		if err = pool.QueryRow(ctx, `SELECT book_id::text,content FROM source_materials WHERE id=$1`, id).Scan(&bookID, &content); err != nil {
			t.Fatal(err)
		}
		if bookID != nil || len(content) == 0 {
			t.Fatalf("backfill down changed source id=%s book=%v content=%d", id, bookID, len(content))
		}
	}
}

func migrationSQL(t *testing.T, name string) string {
	t.Helper()
	sql, err := migrations.FS.ReadFile(name)
	if err != nil {
		t.Fatal(err)
	}
	return string(sql)
}

func insertLegacySource(t *testing.T, ctx context.Context, pool *pgxpool.Pool, owner, language, identifier, title string, content []byte) string {
	t.Helper()
	var id string
	if err := pool.QueryRow(ctx, `INSERT INTO source_materials(owner_id,language,source_identifier,title,media_type,content_hash,content,full_text) VALUES($1,$2,$3,$4,'application/epub+zip',$5,$6,$7) RETURNING id::text`, owner, language, identifier, title, "legacy:"+identifier, content, string(content)).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func putBookSource(t *testing.T, ctx context.Context, store *PostgresStore, owner, identifier, title string, content []byte, fullText string) domain.SourceMaterial {
	t.Helper()
	unit := domain.ExtractedUnit{ID: domain.EPUBUnitID(0, "item"), Order: 0, SpineIndex: 0, ManifestID: "item", Text: fullText, EndOffset: uint64(len([]rune(fullText))), MediaType: "application/xhtml+xml", Linear: true}
	source, err := store.PutSourceMaterialWithExtractedUnits(ctx, domain.SourceMaterial{OwnerID: owner, Language: "de", SourceIdentifier: identifier, Title: title, MediaType: "application/epub+zip", Content: content, FullText: fullText}, domain.ExtractedUnits{SchemaVersion: domain.ExtractedUnitsSchemaVersion, Units: []domain.ExtractedUnit{unit}})
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func containsBook(books []domain.Book, id string) bool {
	for _, book := range books {
		if book.ID == id {
			return true
		}
	}
	return false
}
