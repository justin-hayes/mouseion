//go:build integration

package persistence

import (
	"context"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// legacyAnabasisDefinition is the pre-manifest operator-registered definition.
func legacyAnabasisDefinition() domain.ReadingUnitDefinition {
	return domain.ReadingUnitDefinition{
		Key:                   "xenophon-anabasis-1",
		Language:              "grc",
		WorkTitle:             "Anabasis",
		WorkAuthor:            "Xenophon",
		EditionLabel:          "Marchant 1904",
		CitationStart:         "1.1.1",
		CitationEnd:           "1.10.19",
		SourceFileName:        "anabasis.conllu",
		SourceFileSize:        7438037,
		SourceFileSHA256:      strings.Repeat("a", 64),
		InterpretationVersion: "oga-agdt-upos/5",
	}
}

// manifestDefinition builds one valid fixture definition.
func manifestDefinition(key, workKey, workTitle, author, greekTitle, editionKey, editionLabel, citationStart, citationEnd string, book *int, books int, revision int) classical.Definition {
	return classical.Definition{
		Key:                   key,
		WorkKey:               workKey,
		WorkTitle:             workTitle,
		WorkAuthor:            author,
		GreekTitle:            greekTitle,
		EditionKey:            editionKey,
		EditionLabel:          editionLabel,
		Language:              classical.Language,
		ConstituentBook:       book,
		ConstituentBooks:      books,
		CitationStart:         citationStart,
		CitationEnd:           citationEnd,
		WordCount:             10,
		Source:                classical.SourcePin{Layer: "oga-src", FileName: "anabasis.conllu", Size: 7438037, SHA256: strings.Repeat("a", 64), Format: classical.SourceFormatOGACoNLLU},
		InterpretationVersion: "oga-agdt-upos/5",
		Revision:              revision,
	}
}

// anabasisManifest corrects the legacy one-key rows to the shared Work and
// Edition keys and records the translation pin.
func anabasisManifest(revision int, sourceSHA string) classical.Definition {
	book := 1
	definition := manifestDefinition("xenophon-anabasis-1", "xenophon-anabasis", "Anabasis", "Xenophon", "Ἀνάβασις",
		"xenophon-anabasis-marchant-1904", "Marchant 1904, reviewed", "1.1.1", "1.10.19", &book, 7, revision)
	definition.Source.SHA256 = sourceSHA
	definition.Translation = &classical.TranslationPin{
		Layer: "perseus-eng2-brownson", FileName: "anabasis.xml", Size: 1101217,
		SHA256: strings.Repeat("b", 64), CitationStart: "1.1.1", CitationEnd: "1.10.19",
	}
	return definition
}

func iliadManifest(books int) []classical.Definition {
	definitions := make([]classical.Definition, 0, books)
	for book := 1; book <= books; book++ {
		number := strconv.Itoa(book)
		definitions = append(definitions, manifestDefinition(
			"homer-iliad-"+number,
			"homer-iliad", "Iliad", "Homer", "Ἰλιάς",
			"homer-iliad-monro-1902", "Monro 1902",
			"1.1."+number, "1.1."+number, &book, 24, 1))
	}
	return definitions
}

func reconcileManifest(t *testing.T, store *PostgresStore, definitions ...classical.Definition) error {
	t.Helper()
	manifest, err := classical.RenderDefinitions(definitions)
	require.NoError(t, err)
	return classical.Reconcile(t.Context(), store, manifest)
}

// TestReconcileManifestCorrectsLegacyKeysAndKeepsTheBook exercises the startup
// reconciliation contract: insert, curatorial update in place, shared keys,
// withdrawal, idempotency and no learner Book creation.
func TestReconcileManifestCorrectsLegacyKeysAndKeepsTheBook(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	databaseURL, pool := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "manifest-owner", false)
	require.NoError(t, err)

	legacy, err := store.RegisterReadingUnitBook(ctx, owner.ID, legacyAnabasisDefinition())
	require.NoError(t, err)
	require.True(t, legacy.Created)
	var readingUnitID, workID, editionID string
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT ru.id::text, w.id::text, e.id::text
		FROM book_reading_units bru
		JOIN reading_units ru ON ru.id = bru.reading_unit_id
		JOIN editions e ON e.id = ru.edition_id
		JOIN works w ON w.id = e.work_id
		WHERE bru.book_id = $1`, legacy.BookID).Scan(&readingUnitID, &workID, &editionID))

	first := append([]classical.Definition{anabasisManifest(1, strings.Repeat("a", 64))}, iliadManifest(2)...)
	require.NoError(t, reconcileManifest(t, store, first...))

	// The Anabasis Work and Edition rows are the same rows, corrected to the
	// shared keys, and the Book still points at the same Reading unit.
	var gotWorkID, gotEditionID, gotReadingUnitID string
	var workKey, greekTitle, editionKey, editionLabel, unitKey string
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT w.id::text, w.definition_key, w.greek_title,
		       e.id::text, e.definition_key, e.label,
		       ru.id::text, ru.definition_key
		FROM book_reading_units bru
		JOIN reading_units ru ON ru.id = bru.reading_unit_id
		JOIN editions e ON e.id = ru.edition_id
		JOIN works w ON w.id = e.work_id
		WHERE bru.book_id = $1`, legacy.BookID).Scan(
		&gotWorkID, &workKey, &greekTitle,
		&gotEditionID, &editionKey, &editionLabel,
		&gotReadingUnitID, &unitKey))
	assert.Equal(t, workID, gotWorkID, "the Work row is corrected in place")
	assert.Equal(t, editionID, gotEditionID, "the Edition row is corrected in place")
	assert.Equal(t, readingUnitID, gotReadingUnitID, "the Book keeps its Reading-unit association")
	assert.Equal(t, "xenophon-anabasis", workKey)
	assert.Equal(t, "xenophon-anabasis-marchant-1904", editionKey)
	assert.Equal(t, "xenophon-anabasis-1", unitKey)
	assert.Equal(t, "Ἀνάβασις", greekTitle)
	assert.Equal(t, "Marchant 1904, reviewed", editionLabel)

	var translationSHA string
	require.NoError(t, pool.QueryRow(ctx, `SELECT translation_file_sha256 FROM reading_units WHERE id = $1`, readingUnitID).Scan(&translationSHA))
	assert.Equal(t, strings.Repeat("b", 64), translationSHA, "the first reconciliation records the translation pin")

	// The two Iliad units share one Work and one Edition.
	var iliadWorks, iliadEditions, units, books int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM works WHERE definition_key = 'homer-iliad'`).Scan(&iliadWorks))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM editions WHERE definition_key = 'homer-iliad-monro-1902'`).Scan(&iliadEditions))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM reading_units`).Scan(&units))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM books`).Scan(&books))
	assert.Equal(t, 1, iliadWorks)
	assert.Equal(t, 1, iliadEditions)
	assert.Equal(t, 3, units)
	assert.Equal(t, 1, books, "reconciliation never creates learner Books")

	var disposition string
	require.NoError(t, pool.QueryRow(ctx, `SELECT disposition FROM book_dispositions WHERE owner_id = $1 AND book_id = $2`, owner.ID, legacy.BookID).Scan(&disposition))
	assert.Equal(t, "inbox", disposition, "reconciliation never changes disposition")

	// A second start with the same manifest is idempotent.
	require.NoError(t, reconcileManifest(t, store, first...))
	var repeatedUnits, repeatedWorks, repeatedEditions int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM reading_units`).Scan(&repeatedUnits))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM works`).Scan(&repeatedWorks))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM editions`).Scan(&repeatedEditions))
	assert.Equal(t, units, repeatedUnits)
	assert.Equal(t, 2, repeatedWorks, "the legacy Work rows were corrected, not duplicated")
	assert.Equal(t, 2, repeatedEditions)
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT bru.reading_unit_id::text
		FROM book_reading_units bru
		WHERE bru.book_id = $1`, legacy.BookID).Scan(&gotReadingUnitID))
	assert.Equal(t, readingUnitID, gotReadingUnitID)

	// Removing an entry withdraws it without deleting the shared rows.
	require.NoError(t, reconcileManifest(t, store, anabasisManifest(1, strings.Repeat("a", 64))))
	var iliadWithdrawn int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM reading_units WHERE definition_key LIKE 'homer-iliad-%' AND withdrawn`).Scan(&iliadWithdrawn))
	assert.Equal(t, 2, iliadWithdrawn)
	var iliadRows int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM reading_units WHERE definition_key LIKE 'homer-iliad-%'`).Scan(&iliadRows))
	assert.Equal(t, 2, iliadRows, "a withdrawn definition keeps its rows so Books never dangle")
}

// TestReconcileManifestRefusesABrokenRevision proves a reduced revision or a
// content-bearing change without a bump stops startup and writes nothing.
func TestReconcileManifestRefusesABrokenRevision(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	databaseURL, pool := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)

	require.NoError(t, reconcileManifest(t, store, anabasisManifest(2, strings.Repeat("a", 64))))

	err := reconcileManifest(t, store, anabasisManifest(1, strings.Repeat("a", 64)))
	require.ErrorContains(t, err, "reduce revision")

	err = reconcileManifest(t, store, anabasisManifest(2, strings.Repeat("c", 64)))
	require.ErrorContains(t, err, "changed content without a revision bump")

	changedTranslation := anabasisManifest(2, strings.Repeat("a", 64))
	changedTranslation.Translation.SHA256 = strings.Repeat("d", 64)
	err = reconcileManifest(t, store, changedTranslation)
	require.ErrorContains(t, err, "changed content without a revision bump")

	var revision int
	var sourceSHA string
	require.NoError(t, pool.QueryRow(ctx, `SELECT revision, source_file_sha256 FROM reading_units WHERE definition_key = 'xenophon-anabasis-1'`).Scan(&revision, &sourceSHA))
	assert.Equal(t, 2, revision, "a refused startup leaves the stored definition untouched")
	assert.Equal(t, strings.Repeat("a", 64), sourceSHA)

	require.NoError(t, reconcileManifest(t, store, anabasisManifest(3, strings.Repeat("c", 64))))
	require.NoError(t, pool.QueryRow(ctx, `SELECT revision, source_file_sha256 FROM reading_units WHERE definition_key = 'xenophon-anabasis-1'`).Scan(&revision, &sourceSHA))
	assert.Equal(t, 3, revision)
	assert.Equal(t, strings.Repeat("c", 64), sourceSHA)
}

// TestReconcileManifestUpdatesCuratorialFieldsInPlace changes only curatorial
// facts under the same revision and keeps the row identity.
func TestReconcileManifestUpdatesCuratorialFieldsInPlace(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	databaseURL, pool := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)

	first := anabasisManifest(1, strings.Repeat("a", 64))
	require.NoError(t, reconcileManifest(t, store, first))
	var workID, editionID string
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT w.id::text, e.id::text
		FROM reading_units ru
		JOIN editions e ON e.id = ru.edition_id
		JOIN works w ON w.id = e.work_id
		WHERE ru.definition_key = 'xenophon-anabasis-1'`).Scan(&workID, &editionID))

	corrected := first
	corrected.WorkTitle = "Anabasis (corrected)"
	corrected.WorkAuthor = "Xenophon of Athens"
	corrected.GreekTitle = "Ἀνάβασις (corrected)"
	corrected.EditionLabel = "Marchant 1904, second state"
	require.NoError(t, reconcileManifest(t, store, corrected))

	var gotWorkID, gotEditionID, title, author, greekTitle, label string
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT w.id::text, w.title, w.author, w.greek_title, e.id::text, e.label
		FROM reading_units ru
		JOIN editions e ON e.id = ru.edition_id
		JOIN works w ON w.id = e.work_id
		WHERE ru.definition_key = 'xenophon-anabasis-1'`).Scan(&gotWorkID, &title, &author, &greekTitle, &gotEditionID, &label))
	assert.Equal(t, workID, gotWorkID)
	assert.Equal(t, editionID, gotEditionID)
	assert.Equal(t, "Anabasis (corrected)", title)
	assert.Equal(t, "Xenophon of Athens", author)
	assert.Equal(t, "Ἀνάβασις (corrected)", greekTitle)
	assert.Equal(t, "Marchant 1904, second state", label)
}
