package webapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestValidReadingDeckBookPreservesOwnerMembershipAndCurrentAnalysisChecks(t *testing.T) {
	store := fixtures.NewStore()
	h := &Handler{services: Services{
		Store:    storeDependencies(store),
		Analysis: fixtures.Analysis{},
	}}
	r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/reading/books/fixture-route-match/deck/preparations/new", nil)
	beforeStart := httptest.NewRecorder()
	_, _, ok := h.validReadingDeckBook(beforeStart, r, fixtures.OwnerID, "fixture-route-match")
	assert.False(t, ok)
	assert.Equal(t, http.StatusNotFound, beforeStart.Code, "To Read Books cannot open a preparation task")
	current, err := store.GetCurrentReading(context.Background(), fixtures.OwnerID, "de")
	require.NoError(t, err)
	_, err = store.SwitchCurrentReading(context.Background(), fixtures.OwnerID, "de", "fixture-route-match", current.BookID, current.SnapshotID)
	require.NoError(t, err)
	recorder := httptest.NewRecorder()
	detail, result, ok := h.validReadingDeckBook(recorder, r, fixtures.OwnerID, "fixture-route-match")
	require.True(t, ok)
	assert.Equal(t, fixtures.OwnerID, detail.Book.OwnerID)
	assert.True(t, detail.IsToRead)
	assert.Equal(t, "fixture-route-match-run", result.RunID)
	assert.Equal(t, "fixture-route-match", result.SourceMaterialID)

	wrongOwnerRecorder := httptest.NewRecorder()
	_, _, ok = h.validReadingDeckBook(wrongOwnerRecorder, r, "another-owner", "fixture-route-match")
	assert.False(t, ok)
	assert.Equal(t, http.StatusNotFound, wrongOwnerRecorder.Code)
}
