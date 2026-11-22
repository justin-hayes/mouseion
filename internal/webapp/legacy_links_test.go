package webapp

import (
	"bytes"
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJobBookContextForSourceRequiresCurrentOwnerScopedReadingEntry(t *testing.T) {
	h := &Handler{services: Services{Store: storeDependencies(fixtures.NewStore())}}
	tests := []struct {
		name     string
		owner    string
		sourceID string
		expected string
	}{
		{name: "current analyzed source", owner: fixtures.OwnerID, sourceID: fixtures.SourceID, expected: "/reading#journey-book-fixture-book"},
		{name: "unassessed source", owner: fixtures.OwnerID, sourceID: "fixture-empty", expected: ""},
		{name: "failed source", owner: fixtures.OwnerID, sourceID: "fixture-failed", expected: ""},
		{name: "other owner", owner: "other-owner", sourceID: fixtures.SourceID, expected: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := h.jobBookContextForSource(context.Background(), test.owner, test.sourceID)
			require.NoError(t, err)
			assert.Equal(t, test.expected, got.ReadingURL, test.name)
		})
	}
}

// TestJobsDetailUsesResolvedBookContext proves the analysis detail page still
// names the Book its run belongs to, resolving through the same owner-scoped
// book context the fixture serves.
func TestJobsDetailUsesResolvedBookContext(t *testing.T) {
	h := &Handler{services: Services{Store: storeDependencies(fixtures.NewStore())}}
	book, err := h.jobBookContextForSource(context.Background(), fixtures.OwnerID, fixtures.SourceID)
	require.NoError(t, err)
	assert.Equal(t, "Der lange Weg nach Hause", book.Title)

	var output bytes.Buffer
	status := analysis.Status{ID: 1, DisplayNumber: 1, Lifecycle: domain.JobLifecycleCompleted, Stage: domain.BookPreparationStageFinalize, State: "completed", SourceMaterialID: fixtures.SourceID, CorpusID: "fixture-corpus", RunID: fixtures.ResultRunID}
	require.NoError(t, JobPage(domain.User{Username: "learner"}, "csrf", status, book.ReadingURL, book.Title).Render(context.Background(), &output))
	assert.Contains(t, output.String(), "Der lange Weg nach Hause")
	assert.Contains(t, output.String(), `href="/reading#journey-book-fixture-book"`)
}

func TestJobStatusOffersNoDeckPreparationForLegacyResult(t *testing.T) {
	status := analysis.Status{ID: 7, CorpusID: "legacy-corpus", Lifecycle: domain.JobLifecycleCompleted, Stage: domain.BookPreparationStageAnalysis}
	var output bytes.Buffer
	require.NoError(t, JobStatus("csrf", status, "", "").Render(context.Background(), &output))
	assert.NotContains(t, output.String(), `action="/jobs/7/deck/preparations"`)
	assert.Contains(t, output.String(), "Book decks are prepared from the Book in Reading")
}
