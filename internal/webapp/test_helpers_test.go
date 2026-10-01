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
// embed it, so a single fake can supply every focused store capability.
type allStoreCapabilities interface {
	StudyLanguageStore
	BookStore
	GoalStore
	CurrentReadingStore
	CatalogStore
	AnalysisJobStore
	BookCoverStore
	LemmaReviewStore
	VocabularyBrowseStore
	VocabularyConcordanceStore
}

func storeDependencies(store allStoreCapabilities) StoreDependencies {
	dependencies := StoreDependencies{
		StudyLanguages:        store,
		Books:                 store,
		Goals:                 store,
		CurrentReading:        store,
		Catalog:               store,
		AnalysisJobs:          store,
		Covers:                store,
		LemmaReview:           store,
		VocabularyBrowse:      store,
		VocabularyConcordance: store,
	}
	if selectionStore, ok := store.(VocabularySelectionStore); ok {
		dependencies.VocabularySelection = selectionStore
	}
	return dependencies
}
