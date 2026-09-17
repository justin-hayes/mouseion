//go:build integration

package persistence

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAddToReadingJourneyRejectsBookWithoutChosenLanguage(t *testing.T) {
	ctx := context.Background()
	url, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, url)

	owner, err := store.CreateUser(ctx, "journey-language-owner", false)
	require.NoError(t, err)
	book, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "Awaiting language", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageUnknown})
	require.NoError(t, err)
	journey, err := store.GetReadingJourney(ctx, owner.ID, "de")
	require.NoError(t, err)
	_, err = store.AddToReadingJourney(ctx, owner.ID, "de", book.ID, journey.Revision)
	assert.ErrorIs(t, err, ErrBookLanguageRequired)
	unchanged, err := store.GetReadingJourney(ctx, owner.ID, "de")
	require.NoError(t, err)
	assert.Empty(t, unchanged.Entries, "unknown-language add changed Journey")
	chosen, err := store.CreateBook(ctx, domain.Book{OwnerID: owner.ID, Title: "Chosen language", MetadataProvenance: domain.MetadataProvenanceCatalogueSync, LanguageState: domain.LanguageChosen, LanguageTag: "de"})
	require.NoError(t, err)
	_, err = store.AddToReadingJourney(ctx, owner.ID, "de", chosen.ID, unchanged.Revision)
	require.NoError(t, err, "chosen-language Journey add")
	_, err = store.pool.Exec(ctx, `INSERT INTO reading_journey_membership(owner_id,language,book_id,position) VALUES($1,$2,$3,$4)`, owner.ID, "de", book.ID, 2)
	require.NoError(t, err)
	filtered, err := store.GetReadingJourney(ctx, owner.ID, "de")
	require.NoError(t, err)
	require.Len(t, filtered.Entries, 1)
	assert.Equal(t, chosen.ID, filtered.Entries[0].BookID, "unknown-language membership was visible")
}
