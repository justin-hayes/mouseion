package webapp

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/fixtures"
)

func TestJourneyEntryURLForSourceRequiresCurrentOwnerScopedJourneyEntry(t *testing.T) {
	h := &Handler{services: Services{Store: fixtures.NewStore()}}
	tests := []struct {
		name     string
		owner    string
		sourceID string
		expected string
	}{
		{name: "current analyzed source", owner: fixtures.OwnerID, sourceID: fixtures.SourceID, expected: "/journey/fixture-book"},
		{name: "unassessed source", owner: fixtures.OwnerID, sourceID: "fixture-empty", expected: ""},
		{name: "failed source", owner: fixtures.OwnerID, sourceID: "fixture-failed", expected: ""},
		{name: "other owner", owner: "other-owner", sourceID: fixtures.SourceID, expected: ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := h.journeyEntryURLForSource(context.Background(), test.owner, test.sourceID)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.expected {
				t.Fatalf("Journey entry URL=%q, want %q", got, test.expected)
			}
		})
	}
}

func TestLegacyResultSurfacesUseJourneyEntryURLsOrNoBookLink(t *testing.T) {
	h := &Handler{services: Services{Store: fixtures.NewStore()}}
	urls, err := h.journeyEntryURLs(context.Background(), fixtures.OwnerID, []string{fixtures.SourceID, "fixture-failed"})
	if err != nil {
		t.Fatal(err)
	}

	jobs := []domain.AnalysisJob{
		{ID: 1, DisplayNumber: 1, SourceMaterialID: fixtures.SourceID, AnalysisState: "completed", AnalysisRunID: "run", CorpusID: "corpus"},
		{ID: 2, DisplayNumber: 2, SourceMaterialID: "fixture-failed", AnalysisState: "completed", AnalysisRunID: "run", CorpusID: "corpus"},
	}
	var jobsHTML bytes.Buffer
	if err := JobsPage(domain.User{Username: "learner"}, "csrf", jobs, "", urls).Render(context.Background(), &jobsHTML); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(jobsHTML.String(), `href="/journey/fixture-book"`) || strings.Contains(jobsHTML.String(), `href="/books/fixture-failed"`) {
		t.Fatalf("jobs rendered legacy or dead result link: %s", jobsHTML.String())
	}

}

func TestJobStatusPreservesLegacyDeckPreparationWithoutBookIdentity(t *testing.T) {
	status := analysis.Status{ID: 7, CorpusID: "legacy-corpus", LogicalState: "completed"}
	var output bytes.Buffer
	if err := JobStatus("csrf", status, "").Render(context.Background(), &output); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(output.String(), `action="/jobs/7/deck/preparations"`) || !strings.Contains(output.String(), "Prepare a deck") {
		t.Fatalf("legacy completed result lost deck preparation action: %s", output.String())
	}
}
