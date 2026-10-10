package webapp

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

func requireHandler(t *testing.T, h http.Handler) *Handler {
	t.Helper()
	result, ok := h.(*Handler)
	require.True(t, ok, "webapp test handler has unexpected type %T", h)
	return result
}

// allStoreCapabilities is satisfied by fixtures.Store and test doubles that
// embed it, so a single fake can supply every learner surface.
type allStoreCapabilities interface {
	ShellStore
	MyBooksStore
	ReadingStore
	VocabularyStore
	CatalogsStore
	JobsStore
}

func storeDependencies(store allStoreCapabilities) StoreDependencies {
	return StoreDependencies{
		Shell:      store,
		MyBooks:    store,
		Reading:    store,
		Vocabulary: store,
		Catalogs:   store,
		Jobs:       store,
	}
}
