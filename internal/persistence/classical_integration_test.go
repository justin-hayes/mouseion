//go:build integration

package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func classicalDefinition() domain.ReadingUnitDefinition {
	return domain.ReadingUnitDefinition{
		Key:                   "example-anabasis-1",
		Language:              "grc",
		WorkTitle:             "Anabasis",
		WorkAuthor:            "Xenophon",
		EditionLabel:          "Marchant 1904",
		CitationStart:         "1.1.1",
		CitationEnd:           "1.10.19",
		SourceFileName:        "example.conllu",
		SourceFileSize:        7438037,
		SourceFileSHA256:      "c51bba2df6e80e2eb8dd2543d00cf6d6e6b8c65cc9ed03c7d27176fed063eb61",
		InterpretationVersion: "oga-agdt-upos/4",
	}
}

// TestRegisterReadingUnitBookRejectsInvalidInput covers the guards that run
// before any database work.
func TestRegisterReadingUnitBookRejectsInvalidInput(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)

	_, err := store.RegisterReadingUnitBook(ctx, "  ", classicalDefinition())
	require.ErrorContains(t, err, "owner is required")

	incomplete := classicalDefinition()
	incomplete.SourceFileSize = 0
	_, err = store.RegisterReadingUnitBook(ctx, "00000000-0000-0000-0000-000000000001", incomplete)
	require.ErrorContains(t, err, "source file size")
}

// TestRegisterReadingUnitBookReportsCancelledContext covers the pool-error
// returns on the read and write paths.
func TestRegisterReadingUnitBookReportsCancelledContext(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	cancelled, cancel := context.WithCancel(context.Background())
	cancel()

	_, err := store.RegisterReadingUnitBook(cancelled, "00000000-0000-0000-0000-000000000001", classicalDefinition())
	require.Error(t, err)

	_, err = store.ListLegacyLanguageRows(cancelled, "grc")
	require.Error(t, err)

	books := []domain.MyBook{{Book: domain.Book{ID: "00000000-0000-0000-0000-000000000002"}}}
	require.Error(t, store.populateMyBooksReadingUnits(cancelled, "00000000-0000-0000-0000-000000000001", books))
	assert.Nil(t, books[0].ReadingUnit)

	// An empty page is a no-op that never reaches the database.
	require.NoError(t, store.populateMyBooksReadingUnits(cancelled, "00000000-0000-0000-0000-000000000001", nil))
}

// TestRegisterReadingUnitBookPersistsDefinition covers the shared reference
// rows and the immutable association without going through the web app.
func TestRegisterReadingUnitBookPersistsDefinition(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	databaseURL, pool := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "classical-owner", false)
	require.NoError(t, err)

	registration, err := store.RegisterReadingUnitBook(ctx, owner.ID, classicalDefinition())
	require.NoError(t, err)
	require.True(t, registration.Created)

	repeat, err := store.RegisterReadingUnitBook(ctx, owner.ID, classicalDefinition())
	require.NoError(t, err)
	assert.False(t, repeat.Created)
	assert.Equal(t, registration.BookID, repeat.BookID)

	var workTitle, workAuthor, editionLabel, citation string
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT w.title, w.author, e.label, ru.citation_start
		FROM book_reading_units bru
		JOIN reading_units ru ON ru.id = bru.reading_unit_id
		JOIN editions e ON e.id = ru.edition_id
		JOIN works w ON w.id = e.work_id
		WHERE bru.book_id = $1`, registration.BookID).Scan(&workTitle, &workAuthor, &editionLabel, &citation))
	assert.Equal(t, "Anabasis", workTitle)
	assert.Equal(t, "Xenophon", workAuthor)
	assert.Equal(t, "Marchant 1904", editionLabel)
	assert.Equal(t, "1.1.1", citation)

	detail, err := store.GetBookDetail(ctx, owner.ID, registration.BookID)
	require.NoError(t, err)
	require.NotNil(t, detail.ReadingUnit)
	assert.Equal(t, "Marchant 1904", detail.ReadingUnit.EditionLabel)
	assert.True(t, detail.AwaitingPreparation())

	_, err = pool.Exec(ctx, `UPDATE book_reading_units SET reading_unit_id = reading_unit_id WHERE book_id = $1`, registration.BookID)
	require.ErrorContains(t, err, "immutable", "the association cannot be re-pointed")

	var rows int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM works`).Scan(&rows))
	assert.Equal(t, 1, rows, "the definition is shared reference data")
}
