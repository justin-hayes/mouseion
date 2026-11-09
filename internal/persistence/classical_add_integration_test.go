//go:build integration

package persistence

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func classicalAddDefinition(key, workKey, title string, book *int) classical.Definition {
	books := 2
	if book == nil {
		books = 1
	}
	return classical.Definition{
		Key:                   key,
		WorkKey:               workKey,
		WorkTitle:             title,
		WorkAuthor:            "Xenophon",
		GreekTitle:            "Ἀνάβασις",
		EditionKey:            workKey + "-edition",
		EditionLabel:          "Reviewed edition",
		Language:              classical.Language,
		ConstituentBook:       book,
		ConstituentBooks:      books,
		CitationStart:         "1.1.1",
		CitationEnd:           "1.1.10",
		WordCount:             100,
		Source:                classical.SourcePin{Layer: "fixture", FileName: "work.conllu", Size: 10, SHA256: strings.Repeat("a", 64), Format: classical.SourceFormatOGACoNLLU},
		InterpretationVersion: "oga-agdt-upos/5",
		Revision:              1,
	}
}

func classicalAddDefinitions() []classical.Definition {
	first, second := 1, 2
	return []classical.Definition{
		classicalAddDefinition("work-1", "work", "Work", &first),
		classicalAddDefinition("work-2", "work", "Work", &second),
		classicalAddDefinition("whole-1", "whole", "Whole", nil),
	}
}

func TestAddClassicalReadingUnitsCreatesInboxBooksAndReopensThem(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	databaseURL, pool := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "classical-add-owner", false)
	require.NoError(t, err)
	require.NoError(t, store.ReconcileClassicalManifest(ctx, classicalAddDefinitions()))
	require.NoError(t, store.SetActiveStudyLanguage(ctx, owner.ID, "de"))

	report, err := store.AddClassicalReadingUnits(ctx, owner.ID, []string{"work-1", "work-2"})
	require.NoError(t, err)
	require.Equal(t, 2, report.CreatedBooks())
	require.Len(t, report.Books, 2)
	assert.Equal(t, "work-1", report.Books[0].ReadingUnitKey)
	assert.Equal(t, "work-2", report.Books[1].ReadingUnitKey)

	first, err := store.GetBookDetail(ctx, owner.ID, report.Books[0].BookID)
	require.NoError(t, err)
	assert.Equal(t, "Work", first.Book.Title)
	assert.Equal(t, "Xenophon", first.Book.Author)
	assert.Equal(t, classical.Language, first.Book.LanguageTag)
	assert.Equal(t, domain.BookDispositionInbox, first.Disposition)
	require.NotNil(t, first.ReadingUnit)

	active, err := store.GetStoredActiveStudyLanguage(ctx, owner.ID)
	require.NoError(t, err)
	assert.Equal(t, "de", active, "Add never changes the active study language")

	var jobs, sources, books int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM analysis_jobs WHERE owner_id = $1`, owner.ID).Scan(&jobs))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM source_materials WHERE owner_id = $1`, owner.ID).Scan(&sources))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM books WHERE owner_id = $1`, owner.ID).Scan(&books))
	assert.Zero(t, jobs, "Add starts no job")
	assert.Zero(t, sources, "Add downloads nothing")
	assert.Equal(t, 2, books)

	again, err := store.AddClassicalReadingUnits(ctx, owner.ID, []string{"work-1", "work-2"})
	require.NoError(t, err)
	assert.Zero(t, again.CreatedBooks())
	assert.Equal(t, report.Books[0].BookID, again.Books[0].BookID)
	assert.Equal(t, report.Books[1].BookID, again.Books[1].BookID)

	_, err = store.SetBookHidden(ctx, owner.ID, report.Books[0].BookID, 0, true)
	require.NoError(t, err)
	reopened, err := store.AddClassicalReadingUnits(ctx, owner.ID, []string{"work-1"})
	require.NoError(t, err)
	assert.False(t, reopened.Books[0].Created)
	hidden, _, err := store.GetBookVisibility(ctx, owner.ID, report.Books[0].BookID)
	require.NoError(t, err)
	assert.False(t, hidden, "re-adding reopens a hidden existing Book")

	var bookCount int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM books WHERE owner_id = $1 AND language_tag = $2`, owner.ID, classical.Language).Scan(&bookCount))
	assert.Equal(t, 2, bookCount)

	owned, err := store.ListOwnedClassicalReadingUnits(ctx, owner.ID)
	require.NoError(t, err)
	assert.Len(t, owned, 2)
	assert.Equal(t, "work-1", owned[0].ReadingUnitKey)
}

func TestAddClassicalReadingUnitsRefusesLegacyGreekRows(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	databaseURL, pool := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "classical-legacy-owner", false)
	require.NoError(t, err)
	require.NoError(t, store.ReconcileClassicalManifest(ctx, classicalAddDefinitions()))
	_, err = pool.Exec(ctx, `INSERT INTO books(owner_id, title, metadata_provenance, language_state, language_tag) VALUES ($1, 'Legacy Greek', 'catalog_sync', 'chosen', 'grc')`, owner.ID)
	require.NoError(t, err)

	_, err = store.AddClassicalReadingUnits(ctx, owner.ID, []string{"work-1"})
	require.ErrorIs(t, err, classical.ErrLegacyRows)
	assert.Contains(t, err.Error(), classical.LegacyRowsCode)

	var books int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM books WHERE owner_id = $1 AND metadata_provenance = $2`, owner.ID, domain.MetadataProvenanceOperatorRegistration).Scan(&books))
	assert.Zero(t, books, "the refusal writes no Book")
}

func TestAddClassicalReadingUnitsRefusesUnknownUnitsAndRejectsEmptyInput(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "classical-unknown-owner", false)
	require.NoError(t, err)
	require.NoError(t, store.ReconcileClassicalManifest(ctx, classicalAddDefinitions()))

	t.Run("empty input", func(t *testing.T) {
		_, err := store.AddClassicalReadingUnits(ctx, owner.ID, nil)
		require.ErrorContains(t, err, "at least one Reading unit")
	})
	t.Run("unknown unit", func(t *testing.T) {
		_, err := store.AddClassicalReadingUnits(ctx, owner.ID, []string{"nonexistent"})
		require.ErrorContains(t, err, "not current")
	})
	t.Run("missing owner", func(t *testing.T) {
		_, err := store.AddClassicalReadingUnits(ctx, "  ", []string{"work-1"})
		require.ErrorContains(t, err, "owner is required")
	})
	t.Run("blank key", func(t *testing.T) {
		_, err := store.AddClassicalReadingUnits(ctx, owner.ID, []string{"  "})
		require.ErrorContains(t, err, "key is required")
	})
	t.Run("repeated key", func(t *testing.T) {
		_, err := store.AddClassicalReadingUnits(ctx, owner.ID, []string{"work-1", "work-1"})
		require.NoError(t, err)
		var associations int
		err = store.Pool().QueryRow(ctx, `SELECT count(*) FROM book_reading_units WHERE owner_id = $1`, owner.ID).Scan(&associations)
		require.NoError(t, err)
		assert.Equal(t, 1, associations, "a repeated key never adds twice")
	})
	t.Run("owner row is required", func(t *testing.T) {
		_, err := store.AddClassicalReadingUnits(ctx, "00000000-0000-0000-0000-0000000000ff", []string{"work-2"})
		require.Error(t, err, "a missing learner row rolls back every write")
	})
	t.Run("ownership read failure", func(t *testing.T) {
		cancelled, cancel := context.WithCancel(context.Background())
		cancel()
		_, err := store.ListOwnedClassicalReadingUnits(cancelled, owner.ID)
		require.Error(t, err)
	})
}
