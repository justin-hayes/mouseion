//go:build integration

package persistence

import (
	"context"
	"sync"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPreviouslyReadImportConvergesUnderConcurrentRetries(t *testing.T) {
	ctx := context.Background()
	databaseURL, pool := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)

	owner, err := store.CreateUser(ctx, "previously-read-concurrent", false)
	require.NoError(t, err)
	book, err := store.CreateBook(ctx, domain.Book{
		OwnerID: owner.ID, Title: "Imported history", MetadataProvenance: domain.MetadataProvenanceCatalogueSync,
		LanguageState: domain.LanguageChosen, LanguageTag: "de",
	})
	require.NoError(t, err)

	const retries = 16
	results := make(chan domain.ReadingCompletion, retries)
	errors := make(chan error, retries)
	var wait sync.WaitGroup
	wait.Add(retries)
	for range retries {
		go func() {
			defer wait.Done()
			completion, importErr := store.ImportPreviouslyRead(ctx, owner.ID, book.ID)
			if importErr != nil {
				errors <- importErr
				return
			}
			results <- completion
		}()
	}
	wait.Wait()
	close(results)
	close(errors)
	for importErr := range errors {
		require.NoError(t, importErr)
	}
	var canonical domain.ReadingCompletion
	first := true
	for completion := range results {
		assert.Equal(t, domain.ReadingCompletionPreviouslyRead, completion.Source)
		assert.Zero(t, completion.SnapshotVocabularyCount)
		assert.Zero(t, completion.EligibleVocabularyCount)
		if first {
			canonical = completion
			first = false
		}
		assert.Equal(t, canonical.CompletedAt, completion.CompletedAt)
	}
	var importedCount, knownCount int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM reading_history WHERE owner_id=$1 AND book_id=$2 AND completion_source='previously_read_import'`, owner.ID, book.ID).Scan(&importedCount))
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM known_vocabulary WHERE owner_id=$1 AND language='de'`, owner.ID).Scan(&knownCount))
	assert.Equal(t, 1, importedCount)
	assert.Zero(t, knownCount)
}
