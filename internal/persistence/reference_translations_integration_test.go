//go:build integration

package persistence

import (
	"context"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestReferenceTranslationsRoundTripAndIdempotency covers the installation-wide
// translation store: a re-import writes the same rows, a pin with no import
// reports empty values, invalid input is refused, and different pins for the
// same citation coexist instead of conflicting.
func TestReferenceTranslationsRoundTripAndIdempotency(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, Migrate)
	store := openIntegrationStore(t, ctx, databaseURL)
	pin := "f4c88203196a4f2ed315d47642d10f9515c568374139ba1e25e7934312586ea7"
	otherPin := "0000000000000000000000000000000000000000000000000000000000000001"
	translations := map[string]string{"1.2.18": "second", "1.2.17": "first"}

	written, err := store.PutReferenceTranslations(ctx, pin, "grc", translations)
	require.NoError(t, err)
	assert.Equal(t, 2, written)

	written, err = store.PutReferenceTranslations(ctx, pin, "grc", translations)
	require.NoError(t, err)
	assert.Equal(t, 2, written, "a re-import writes the same rows")

	loaded, err := store.ListReferenceTranslationsByPin(ctx, pin)
	require.NoError(t, err)
	assert.Equal(t, translations, loaded)

	found, err := store.HasReferenceTranslations(ctx, pin)
	require.NoError(t, err)
	assert.True(t, found)
	found, err = store.HasReferenceTranslations(ctx, otherPin)
	require.NoError(t, err)
	assert.False(t, found)

	for name, attempt := range map[string]func() error{
		"empty citation": func() error {
			_, err := store.PutReferenceTranslations(ctx, pin, "grc", map[string]string{"": "x"})
			return err
		},
		"empty language": func() error {
			_, err := store.PutReferenceTranslations(ctx, pin, " ", translations)
			return err
		},
		"invalid pin": func() error {
			_, err := store.PutReferenceTranslations(ctx, "sha256:other", "grc", translations)
			return err
		},
		"no sections": func() error {
			_, err := store.PutReferenceTranslations(ctx, pin, "grc", nil)
			return err
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.ErrorIs(t, attempt(), errInvalidReferenceTranslation)
		})
	}

	// A different pin for the same citation is a separate installation-wide
	// translation, not a per-owner conflict.
	_, err = store.PutReferenceTranslations(ctx, otherPin, "grc", map[string]string{"1.2.17": "another first"})
	require.NoError(t, err)
	loaded, err = store.ListReferenceTranslationsByPin(ctx, pin)
	require.NoError(t, err)
	assert.Equal(t, "first", loaded["1.2.17"])
	other, err := store.ListReferenceTranslationsByPin(ctx, otherPin)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{"1.2.17": "another first"}, other)

	empty, err := store.ListReferenceTranslationsByPin(ctx, strings.Repeat("a", 64))
	require.NoError(t, err)
	assert.Empty(t, empty)
	empty, err = store.ListReferenceTranslationsByPin(ctx, "")
	require.NoError(t, err)
	assert.Empty(t, empty)
	found, err = store.HasReferenceTranslations(ctx, "")
	require.NoError(t, err)
	assert.False(t, found)

	var nilStore *PostgresStore
	_, err = nilStore.PutReferenceTranslations(ctx, pin, "grc", translations)
	require.ErrorIs(t, err, errInvalidReferenceTranslation)
	_, err = nilStore.ListReferenceTranslationsByPin(ctx, pin)
	require.Error(t, err)
	_, err = nilStore.HasReferenceTranslations(ctx, pin)
	require.Error(t, err)
}
