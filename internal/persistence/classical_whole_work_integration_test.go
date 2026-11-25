//go:build integration

package persistence

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// perBookDefinition is one withdrawn ADR 0116 per-book definition of a
// two-book Work.
func perBookDefinition(book int) classical.Definition {
	start, end := "1.1.1", "1.1.5"
	if book == 2 {
		start, end = "2.1.1", "2.1.5"
	}
	number := book
	definition := manifestDefinition(fmt.Sprintf("work-%d", book), "work", "Work", "Author", "Ἔργον",
		"work-edition", "Edition", start, end, &number, 2, 1)
	definition.WordCount = 5
	return definition
}

// wholeWorkDefinition is the ADR 0128 whole-work definition of the same Work.
func wholeWorkDefinition() classical.Definition {
	definition := manifestDefinition("work", "work", "Work", "Author", "Ἔργον",
		"work-edition", "Edition", "1.1.1", "2.1.5", nil, 2, 1)
	definition.WordCount = 10
	definition.Contents = []classical.ConstituentBookSpan{
		{Book: 1, CitationStart: "1.1.1", CitationEnd: "1.1.5", WordCount: 5},
		{Book: 2, CitationStart: "2.1.1", CitationEnd: "2.1.5", WordCount: 5},
	}
	return definition
}

// TestReconcileManifestReplacesPerBookDefinitionsWithAWholeWorkUnit proves the
// ADR 0128 migration: the whole-work definition is added, the per-book
// definitions are withdrawn, learner Books on them stay readable and linked,
// and a second start changes nothing.
func TestReconcileManifestReplacesPerBookDefinitionsWithAWholeWorkUnit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	databaseURL, pool := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "whole-work-owner", false)
	require.NoError(t, err)

	// A learner registered a per-book Book before the whole-work release.
	require.NoError(t, reconcileManifest(t, store, perBookDefinition(1), perBookDefinition(2)))
	perBook := perBookDefinition(1)
	registration, err := store.RegisterReadingUnitBook(ctx, owner.ID, domain.ReadingUnitDefinition{
		Key: perBook.Key, Language: perBook.Language, WorkTitle: perBook.WorkTitle, WorkAuthor: perBook.WorkAuthor,
		EditionLabel: perBook.EditionLabel, CitationStart: perBook.CitationStart, CitationEnd: perBook.CitationEnd,
		SourceFileName: perBook.Source.FileName, SourceFileSize: perBook.Source.Size,
		SourceFileSHA256: perBook.Source.SHA256, InterpretationVersion: perBook.InterpretationVersion,
	})
	require.NoError(t, err)
	perBookBook := registration.BookID

	replacement := []classical.Definition{wholeWorkDefinition(), perBookDefinition(1), perBookDefinition(2)}
	require.NoError(t, reconcileManifest(t, store, replacement...))

	var constituentBook *int
	var constituentBooks int
	var withdrawn bool
	require.NoError(t, pool.QueryRow(ctx, `SELECT constituent_book, constituent_books, withdrawn FROM reading_units WHERE definition_key = 'work'`).Scan(&constituentBook, &constituentBooks, &withdrawn))
	assert.Nil(t, constituentBook, "a whole-work unit has no constituent book number")
	assert.Equal(t, 2, constituentBooks)
	assert.False(t, withdrawn)

	var withdrawnPerBook int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM reading_units WHERE definition_key IN ('work-1', 'work-2') AND withdrawn`).Scan(&withdrawnPerBook))
	assert.Equal(t, 2, withdrawnPerBook)

	var linkedUnit string
	require.NoError(t, pool.QueryRow(ctx, `
		SELECT ru.definition_key FROM book_reading_units bru JOIN reading_units ru ON ru.id = bru.reading_unit_id WHERE bru.book_id = $1`, perBookBook).Scan(&linkedUnit))
	assert.Equal(t, "work-1", linkedUnit, "an existing Book keeps its withdrawn Reading unit")

	counts := func() (units, works, editions, books int) {
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM reading_units`).Scan(&units))
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM works`).Scan(&works))
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM editions`).Scan(&editions))
		require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM books`).Scan(&books))
		return units, works, editions, books
	}
	units, works, editions, books := counts()
	require.NoError(t, reconcileManifest(t, store, replacement...))
	againUnits, againWorks, againEditions, againBooks := counts()
	assert.Equal(t, []int{units, works, editions, books}, []int{againUnits, againWorks, againEditions, againBooks}, "reconciliation is idempotent and never creates Books")
	assert.Equal(t, 3, units)
	assert.Equal(t, 1, works)
	assert.Equal(t, 1, books)

	_, err = store.AddClassicalReadingUnits(ctx, owner.ID, []string{"work-2"})
	require.Error(t, err, "a withdrawn per-book definition is refused by Add")

	whole, err := store.AddClassicalReadingUnits(ctx, owner.ID, []string{"work"})
	require.NoError(t, err, "a learner who owns only a withdrawn per-book Book can add the whole Work")
	require.Len(t, whole.Books, 1)
	assert.True(t, whole.Books[0].Created)
	assert.NotEqual(t, perBookBook, whole.Books[0].BookID)

	ownedUnits, err := store.ListOwnedClassicalReadingUnits(ctx, owner.ID)
	require.NoError(t, err)
	keys := make([]string, 0, len(ownedUnits))
	for _, unit := range ownedUnits {
		keys = append(keys, unit.ReadingUnitKey)
	}
	assert.ElementsMatch(t, []string{"work-1", "work"}, keys)
}
