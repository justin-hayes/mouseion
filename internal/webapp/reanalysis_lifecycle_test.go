package webapp

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestReanalysisLifecycleOffersRetryOnlyForNonCurrentBook(t *testing.T) {
	published := domain.AnalysisSignals{Content: domain.ContentCurrentEPUB, Published: domain.PublishedCurrent}
	summary := func(run domain.LatestRunSignal, current bool) domain.SourceMaterialSummary {
		signals := published
		signals.LatestRun = run
		return domain.SourceMaterialSummary{
			Source:           domain.SourceMaterial{ID: "book-source", Language: "de"},
			BookID:           "book",
			Signals:          signals,
			AnalysisJobID:    42,
			IsToRead:         true,
			IsCurrentReading: current,
		}
	}

	for _, run := range []domain.LatestRunSignal{domain.RunFailed, domain.RunJobFailed, domain.RunCancelled} {
		other := bookLifecycleActionFor(summary(run, false))
		assert.Equal(t, "Retry analysis", other.Label, "a To Read book under %s retries", run)
		assert.True(t, other.Submit, "a To Read book under %s submits its retry", run)
		assert.Equal(t, readingReanalyzeURL("book"), other.URL)

		current := bookLifecycleActionFor(summary(run, true))
		assert.NotEqual(t, "Retry analysis", current.Label, "the Current reading under %s offers no retry", run)
		assert.False(t, current.Submit, "the Current reading under %s submits nothing", run)
		assert.Equal(t, "/jobs/42", current.URL, "the Current reading under %s links to its job", run)
		assert.Equal(t, "The current analysis stays in effect.", current.Description)
		assert.Equal(t, other.Status, current.Status, "the run's state is presented the same way")
	}
}

func TestReanalysisLifecycleOmitsLinkWithoutJob(t *testing.T) {
	book := domain.SourceMaterialSummary{
		Source:           domain.SourceMaterial{ID: "book-source", Language: "de"},
		BookID:           "book",
		Signals:          domain.AnalysisSignals{Content: domain.ContentCurrentEPUB, Published: domain.PublishedCurrent, LatestRun: domain.RunFailed},
		IsCurrentReading: true,
	}
	action := bookLifecycleActionFor(book)
	assert.Equal(t, "Re-analysis failed", action.Status)
	assert.Empty(t, action.Label, "no job means no dead link")
	assert.Empty(t, action.URL)
	assert.False(t, action.Submit)
}
