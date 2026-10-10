package webapp

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/fixtures"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadingEntryURLForSourceRequiresCurrentOwnerScopedReadingEntry(t *testing.T) {
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
			got, err := h.readingBookURLForSource(context.Background(), test.owner, test.sourceID)
			require.NoError(t, err)
			assert.Equal(t, test.expected, got, test.name)
		})
	}
}

func TestLegacyResultSurfacesUseReadingEntryURLsOrNoBookLink(t *testing.T) {
	h := &Handler{services: Services{Store: storeDependencies(fixtures.NewStore())}}
	bookContexts, err := h.jobBookContexts(context.Background(), fixtures.OwnerID, []string{fixtures.SourceID, "fixture-failed"})
	require.NoError(t, err)

	jobs := []domain.AnalysisJob{
		{ID: 1, DisplayNumber: 1, SourceMaterialID: fixtures.SourceID, AnalysisState: "completed", AnalysisRunID: "run", CorpusID: "corpus"},
		{ID: 2, DisplayNumber: 2, SourceMaterialID: "fixture-failed", AnalysisState: "completed", AnalysisRunID: "run", CorpusID: "corpus"},
	}
	var jobsHTML bytes.Buffer
	require.NoError(t, JobsPage(domain.User{Username: "learner"}, "csrf", jobs, "", bookContexts).Render(context.Background(), &jobsHTML))
	assert.True(t, strings.Contains(jobsHTML.String(), `href="/reading#journey-book-fixture-book"`) && !strings.Contains(jobsHTML.String(), `href="/books/fixture-failed"`), "jobs rendered legacy or dead result link: %s", jobsHTML.String())
	assert.Contains(t, jobsHTML.String(), "Fehlgeschlagene Analyse")

}

func TestJobStatusPreservesLegacyDeckPreparationWithoutBookIdentity(t *testing.T) {
	status := analysis.Status{ID: 7, CorpusID: "legacy-corpus", LogicalState: "completed"}
	var output bytes.Buffer
	require.NoError(t, JobStatus("csrf", status, "", "").Render(context.Background(), &output))
	assert.True(t, strings.Contains(output.String(), `action="/jobs/7/deck/preparations"`) && strings.Contains(output.String(), "Prepare a deck"), "legacy completed result lost deck preparation action: %s", output.String())
}
