//go:build integration

package persistence

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReferenceTranslationsRoundTripAndIdempotency covers the operator import
// seam: a re-import writes the same rows, an owner with no import reports empty
// values, invalid input is refused, and two languages with different pins are
// reported rather than silently relabelled.
func TestReferenceTranslationsRoundTripAndIdempotency(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	owner, err := store.CreateUser(ctx, "reference-translation-owner", false)
	require.NoError(t, err)
	pin := "sha256:f4c88203196a4f2ed315d47642d10f9515c568374139ba1e25e7934312586ea7"
	translations := map[string]string{"1.2.18": "second", "1.2.17": "first"}

	written, err := store.PutReferenceTranslations(ctx, owner.ID, "grc", pin, translations)
	require.NoError(t, err)
	assert.Equal(t, 2, written)

	written, err = store.PutReferenceTranslations(ctx, owner.ID, "grc", pin, translations)
	require.NoError(t, err)
	assert.Equal(t, 2, written, "a re-import writes the same rows")

	loaded, loadedPin, err := store.ListReferenceTranslations(ctx, owner.ID, "grc")
	require.NoError(t, err)
	assert.Equal(t, translations, loaded)
	assert.Equal(t, pin, loadedPin)

	for name, attempt := range map[string]func() error{
		"empty citation": func() error {
			_, err := store.PutReferenceTranslations(ctx, owner.ID, "grc", pin, map[string]string{"": "x"})
			return err
		},
		"empty owner": func() error {
			_, err := store.PutReferenceTranslations(ctx, "  ", "grc", pin, translations)
			return err
		},
		"no sections": func() error {
			_, err := store.PutReferenceTranslations(ctx, owner.ID, "grc", pin, nil)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.ErrorIs(t, attempt(), errInvalidReferenceTranslation)
		})
	}

	_, err = store.PutReferenceTranslations(ctx, owner.ID, "de", "sha256:other", map[string]string{"1.1": "eins"})
	require.NoError(t, err)
	_, _, err = mergedReferenceTranslations(ctx, store.Pool(), owner.ID, map[string]struct{}{"grc": {}, "de": {}})
	require.ErrorContains(t, err, "conflicting reference translation pins")

	other, err := store.CreateUser(ctx, "reference-translation-empty-owner", false)
	require.NoError(t, err)
	empty, emptyPin, err := store.ListReferenceTranslations(ctx, other.ID, "grc")
	require.NoError(t, err)
	assert.Empty(t, empty)
	assert.Empty(t, emptyPin)
	empty, emptyPin, err = store.ListReferenceTranslations(ctx, "", "grc")
	require.NoError(t, err)
	assert.Empty(t, empty)
	assert.Empty(t, emptyPin)

	var nilStore *PostgresStore
	_, err = nilStore.PutReferenceTranslations(ctx, owner.ID, "grc", pin, translations)
	require.ErrorIs(t, err, errInvalidReferenceTranslation)
	_, _, err = nilStore.ListReferenceTranslations(ctx, owner.ID, "grc")
	require.Error(t, err)
}
