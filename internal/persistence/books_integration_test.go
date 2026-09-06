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
	aliceConnection, err := store.CreateOpdsConnection(ctx, alice.ID, domain.OpdsConnection{Name: "Alice catalog", URL: "https://alice.example/opds"})
	if err != nil {
		t.Fatal(err)
	}
	bobConnection, err := store.CreateOpdsConnection(ctx, bob.ID, domain.OpdsConnection{Name: "Bob catalog", URL: "https://bob.example/opds"})
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
	for i, source := range legacySources {
		connectionID := aliceConnection.ID
		if source.owner == bob.ID {
			connectionID = bobConnection.ID
		}
		if _, err = store.ReconcileCatalogueEntry(ctx, source.owner, connectionID, source.identifier, source.title, source.language); err != nil {
			t.Fatal(err)
		}
		var bookID string
		if err = pool.QueryRow(ctx, `SELECT book_id::text FROM book_aliases WHERE owner_id=$1 AND connection_id=$2 AND value=$3`, source.owner, connectionID, source.identifier).Scan(&bookID); err != nil {
			t.Fatal(err)
		}
		if err = store.LinkSourceToBook(ctx, source.owner, bookID, legacyIDs[i]); err != nil {
			t.Fatal(err)
		}
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

	metadataOnly, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Unacquired book", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown})
	if err != nil {
		t.Fatal(err)
	}
	retriedMetadata, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Unacquired book", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown})
	if err != nil || retriedMetadata.ID == metadataOnly.ID {
		t.Fatalf("repeated metadata create book=%q unexpectedly reused=%q err=%v", retriedMetadata.ID, metadataOnly.ID, err)
	}
	books, err = store.ListMyBooks(ctx, alice.ID)
	if err != nil || !containsBook(books, metadataOnly.ID) {
		t.Fatalf("metadata-only book missing from My Books: books=%+v err=%v", books, err)
	}
	view, err := store.ListMyBooksWithEvidence(ctx, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	var metadataView *domain.MyBook
	for i := range view {
		if view[i].Book.ID == metadataOnly.ID {
			metadataView = &view[i]
			break
		}
	}
	if metadataView == nil || metadataView.Acquired != nil || metadataView.EvidenceState != domain.MyBookNotAcquired || metadataView.Book.LanguageState != domain.LanguageUnknown {
		t.Fatalf("metadata-only read model=%+v", metadataView)
	}

	promotion, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Promote this book", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown})
	if err != nil {
		t.Fatal(err)
	}
	promotionSource := putBookSource(t, ctx, store, alice.ID, "promotion-source", "Promoted", []byte("promotion-epub"), "promoted")
	resolvedPromotion, err := store.ResolveOrCreateBookForAcquisitionForBook(ctx, alice.ID, promotion.ID, promotionSource.SourceIdentifier, promotionSource.Language, promotionSource.Title)
	if err != nil || resolvedPromotion != promotion.ID {
		t.Fatalf("promotion resolved book=%q want=%q err=%v", resolvedPromotion, promotion.ID, err)
	}
	if err = store.LinkSourceToBook(ctx, alice.ID, promotion.ID, promotionSource.ID); err != nil {
		t.Fatal(err)
	}
	if repeated, repeatErr := store.ResolveOrCreateBookForAcquisitionForBook(ctx, alice.ID, promotion.ID, promotionSource.SourceIdentifier, promotionSource.Language, promotionSource.Title); repeatErr != nil || repeated != promotion.ID {
		t.Fatalf("repeated promotion book=%q want=%q err=%v", repeated, promotion.ID, repeatErr)
	}
	view, err = store.ListMyBooksWithEvidence(ctx, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	foundPromotion := false
	for i := range view {
		if view[i].Book.ID == promotion.ID {
			foundPromotion = true
			if view[i].Acquired == nil || view[i].Acquired.Source.ID != promotionSource.ID || view[i].Book.LanguageState != domain.LanguageUnknown || view[i].Book.LanguageTag != "" || view[i].EvidenceState != domain.MyBookAcquiredUnassessed {
				t.Fatalf("promoted read model=%+v", view[i])
			}
			break
		}
	}
	if !foundPromotion {
		t.Fatalf("promoted book missing from My Books read model: %+v", view)
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

	secondBook, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Second", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown})
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
	if _, found, err = store.ResolveBookByAlias(ctx, alice.ID, domain.NamespaceSourceIdentifier, "acquisition-source"); err != nil || found {
		t.Fatalf("connectionless acquisition source alias was persisted: found=%v err=%v", found, err)
	}

	if err = store.AddBookAlias(ctx, alice.ID, secondBook.ID, "invalid", "failure", "must-not-commit"); err == nil {
		t.Fatal("invalid alias was accepted")
	}
	var invalidAliases int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM book_aliases WHERE owner_id=$1 AND value='must-not-commit'`, alice.ID).Scan(&invalidAliases); err != nil || invalidAliases != 0 {
		t.Fatalf("failed alias left partial row count=%d err=%v", invalidAliases, err)
	}

}

func TestGetBookDetailResolvesBookAndSourceIDsWithinOwner(t *testing.T) {
	ctx := context.Background()
	url, _ := testutil.Postgres(t, ctx, Migrate)
	store, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	alice, err := store.CreateUser(ctx, "book-detail-alice", false)
	if err != nil {
		t.Fatal(err)
	}
	bob, err := store.CreateUser(ctx, "book-detail-bob", false)
	if err != nil {
		t.Fatal(err)
	}
	metadata, err := store.CreateBook(ctx, domain.Book{OwnerID: alice.ID, Title: "Metadata", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown})
	if err != nil {
		t.Fatal(err)
	}
	metadataDetail, err := store.GetBookDetail(ctx, alice.ID, metadata.ID)
	if err != nil || metadataDetail.Acquired != nil || metadataDetail.EvidenceState != domain.MyBookNotAcquired {
		t.Fatalf("metadata detail=%+v err=%v", metadataDetail, err)
	}

	source := putBookSource(t, ctx, store, alice.ID, "detail-source", "Acquired", []byte("detail-epub"), "de")
	bookID, err := store.ResolveOrCreateBookForAcquisition(ctx, alice.ID, source.SourceIdentifier, source.Language, source.Title)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.LinkSourceToBook(ctx, alice.ID, bookID, source.ID); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{bookID, source.ID} {
		detail, detailErr := store.GetBookDetail(ctx, alice.ID, id)
		if detailErr != nil || detail.Book.ID != bookID || detail.Acquired == nil || detail.Acquired.Source.ID != source.ID || detail.EvidenceState != domain.MyBookAcquiredUnassessed {
			t.Fatalf("detail id=%q result=%+v err=%v", id, detail, detailErr)
		}
	}
	if _, err = store.GetBookDetail(ctx, bob.ID, bookID); !errors.Is(err, ErrNotFound) {
		t.Fatalf("cross-owner book detail error=%v", err)
	}
	if _, err = store.GetBookDetail(ctx, alice.ID, "missing"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("unknown book detail error=%v", err)
	}
}

func TestListStudyLanguagesDerivesActiveChosenBooks(t *testing.T) {
	ctx := context.Background()
	url, _ := testutil.Postgres(t, ctx, Migrate)
	store, err := Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()

	alice, err := store.CreateUser(ctx, "study-languages-alice", false)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.PutSupportedLanguage(ctx, "de", "German"); err != nil {
		t.Fatal(err)
	}
	inputs := []struct {
		title, language string
		state           string
	}{
		{title: "German one", language: "DE_de", state: domain.LanguageChosen},
		{title: "German two", language: "de-DE", state: domain.LanguageChosen},
		{title: "Fallback", language: "PT_br", state: domain.LanguageChosen},
		{title: "Unknown", state: domain.LanguageUnknown},
	}
	for _, input := range inputs {
		book, bookErr := domain.NewBook(alice.ID, input.title, domain.MetadataProvenanceCatalogueSync, input.state, input.language)
		if bookErr != nil {
			t.Fatal(bookErr)
		}
		if _, err = store.CreateBook(ctx, book); err != nil {
			t.Fatal(err)
		}
	}
	books, err := store.ListMyBooks(ctx, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	var duplicateBookID string
	for _, book := range books {
		if book.Title == "German one" {
			duplicateBookID = book.ID
			break
		}
	}
	if duplicateBookID == "" {
		t.Fatal("German one book missing")
	}
	if err = store.RemoveBookFromMyBooks(ctx, alice.ID, duplicateBookID); err != nil {
		t.Fatal(err)
	}

	languages, err := store.ListStudyLanguages(ctx, alice.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(languages) != 2 || languages[0] != (domain.StudyLanguage{Language: "de", DisplayName: "German"}) || languages[1] != (domain.StudyLanguage{Language: "pt", DisplayName: "pt"}) {
		t.Fatalf("derived study languages=%+v", languages)
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
