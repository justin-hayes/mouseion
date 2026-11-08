//go:build integration

package persistence

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/require"
)

func TestCurrentDeckPreparationForBookTxReadsTheUnretiredPreparation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	url, _ := testutil.Postgres(t, ctx, Migrate)
	store, err := Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)

	owner, err := store.CreateUser(ctx, "idiom-review-"+uuid.NewString(), false)
	require.NoError(t, err)
	book, err := store.CreateBook(ctx, domain.Book{
		OwnerID: owner.ID, Title: "Libro", MetadataProvenance: domain.MetadataProvenanceAcquisition,
		LanguageState: domain.LanguageChosen, LanguageTag: "it",
	})
	require.NoError(t, err)
	prepare := func(hash string) domain.DeckPreparation {
		t.Helper()
		source, sourceErr := store.PutSourceMaterial(ctx, domain.SourceMaterial{
			OwnerID: owner.ID, Language: "it", SourceIdentifier: uuid.NewString(), Title: "Libro",
			MediaType: "text/plain", ContentHash: hash, Content: []byte("testo"), FullText: "testo",
		})
		require.NoError(t, sourceErr)
		require.NoError(t, store.LinkSourceToBook(ctx, owner.ID, book.ID, source.ID))
		prep, prepErr := store.CreateDeckPreparation(ctx, domain.DeckPreparation{
			OwnerID: owner.ID, SourceMaterialID: source.ID,
			Filename: cardexport.DownloadFilename("Libro"), DeckName: "Libro", ContentHash: hash,
		})
		require.NoError(t, prepErr)
		return prep
	}
	retired := prepare(uuid.NewString())
	current := prepare(uuid.NewString())
	require.NotEqual(t, retired.ID, current.ID)

	read := func(bookID string) (domain.DeckPreparation, error) {
		tx, beginErr := store.Pool().BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
		require.NoError(t, beginErr)
		defer func() { require.NoError(t, tx.Rollback(ctx)) }()
		return store.CurrentDeckPreparationForBookTx(ctx, tx, bookID)
	}

	t.Run("the current preparation is read in any state", func(t *testing.T) {
		got, readErr := read(book.ID)
		require.NoError(t, readErr)
		require.Equal(t, current.ID, got.ID)
		require.Equal(t, owner.ID, got.OwnerID)
		require.NotEqual(t, domain.DeckPreparationReady, got.State)
	})
	t.Run("an unknown Book is not found", func(t *testing.T) {
		_, readErr := read(uuid.NewString())
		require.ErrorIs(t, readErr, ErrNotFound)
	})
}
