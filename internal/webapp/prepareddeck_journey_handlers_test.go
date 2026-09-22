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

func TestValidJourneyDeckBookPreservesOwnerMembershipAndCurrentAnalysisChecks(t *testing.T) {
	store := fixtures.NewStore()
	h := &Handler{services: Services{
		Store:    StoreDependencies{Books: store, Journey: store, Goals: store},
		Analysis: fixtures.Analysis{},
	}}
	r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/journey/books/fixture-route-match/deck/preparations/new", nil)
	recorder := httptest.NewRecorder()
	detail, result, ok := h.validJourneyDeckBook(recorder, r, fixtures.OwnerID, "fixture-route-match")
	require.True(t, ok)
	assert.Equal(t, fixtures.OwnerID, detail.Book.OwnerID)
	assert.True(t, detail.JourneyMember)
	assert.Equal(t, "fixture-route-match-run", result.RunID)
	assert.Equal(t, "fixture-route-match", result.SourceMaterialID)

	wrongOwnerRecorder := httptest.NewRecorder()
	_, _, ok = h.validJourneyDeckBook(wrongOwnerRecorder, r, "another-owner", "fixture-route-match")
	assert.False(t, ok)
	assert.Equal(t, http.StatusNotFound, wrongOwnerRecorder.Code)
}
