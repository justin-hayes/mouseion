package webapp

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
)

func acquisitionFailureBook(status domain.EPUBAcquisitionStatus) domain.MyBook {
	return domain.MyBook{
		Book:            domain.Book{ID: "book-1", Title: "Der lange Weg", LanguageState: domain.LanguageChosen, LanguageTag: "de"},
		Disposition:     domain.BookDispositionToRead,
		EPUBAcquisition: &status,
	}
}

func TestEPUBAcquisitionFailurePresentation(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		status   domain.EPUBAcquisitionStatus
		evidence string
		recovery string
	}{
		{
			name:     "queued",
			status:   domain.EPUBAcquisitionStatus{State: domain.JobLifecycleQueued},
			evidence: epubAcquisitionInProgressLabel + ".",
			recovery: "",
		},
		{
			name:     "running",
			status:   domain.EPUBAcquisitionStatus{State: domain.JobLifecycleRunning},
			evidence: epubAcquisitionInProgressLabel + ".",
			recovery: "",
		},
		{
			name:     "entry missing",
			status:   domain.EPUBAcquisitionStatus{Stage: domain.BookPreparationStageEntry, Code: domain.BookPreparationCodeCatalogEntryMissing},
			evidence: epubAcquisitionEntryMissingLabel,
			recovery: epubAcquisitionRetryLabel,
		},
		{
			name:     "download failed",
			status:   domain.EPUBAcquisitionStatus{Stage: domain.BookPreparationStageFetch, Code: domain.BookPreparationCodeSourceDownloadFailed},
			evidence: epubAcquisitionDownloadFailedLabel,
			recovery: epubAcquisitionRetryLabel,
		},
		{
			name:     "oversize source",
			status:   domain.EPUBAcquisitionStatus{Stage: domain.BookPreparationStageSource, Code: domain.BookPreparationCodeSourceOversize},
			evidence: epubAcquisitionOversizeLabel,
			recovery: "",
		},
		{
			name:     "unknown source format",
			status:   domain.EPUBAcquisitionStatus{Stage: domain.BookPreparationStageSource, Code: domain.BookPreparationCodeSourceFormatUnknown},
			evidence: epubAcquisitionFormatLabel,
			recovery: "",
		},
		{
			name:     "import failed",
			status:   domain.EPUBAcquisitionStatus{Stage: domain.BookPreparationStageImport, Code: domain.BookPreparationCodeEPUBImportFailed},
			evidence: epubAcquisitionImportFailedLabel,
			recovery: epubAcquisitionRetryLabel,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			book := acquisitionFailureBook(tt.status)
			assert.Equal(t, tt.evidence, myBookMarginEvidence(book))
			assert.Equal(t, tt.recovery, myBookEvidenceRecovery(book))
			if tt.recovery != "" {
				assert.Equal(t, "/reading/books/book-1/reanalyze", myBookEvidenceRecoveryURL(book))
			}
		})
	}
}

// TestEPUBAcquisitionInProgressReadsAsClassicalPreparation states the one
// in-progress vocabulary: an EPUB Book and a classical Book in the same
// lifecycle state read identically on Reading, and neither offers a
// recovery while its preparation is under way.
func TestEPUBAcquisitionInProgressReadsAsClassicalPreparation(t *testing.T) {
	t.Parallel()
	states := []struct {
		name      string
		classical domain.ClassicalPreparationStatus
		epub      domain.EPUBAcquisitionStatus
	}{
		{
			name:      "queued",
			classical: domain.ClassicalPreparationStatus{State: domain.ClassicalPreparationQueued, CurrentRevision: 1},
			epub:      domain.EPUBAcquisitionStatus{State: domain.JobLifecycleQueued},
		},
		{
			name:      "running",
			classical: domain.ClassicalPreparationStatus{State: domain.ClassicalPreparationRunning, CurrentRevision: 1},
			epub:      domain.EPUBAcquisitionStatus{State: domain.JobLifecycleRunning},
		},
	}
	for _, state := range states {
		t.Run(state.name, func(t *testing.T) {
			t.Parallel()
			classical := classicalStatusBook(state.classical)
			epub := acquisitionFailureBook(state.epub)
			assert.Equal(t, myBookMarginEvidence(classical), myBookMarginEvidence(epub))
			recovery := myBookEvidenceRecovery(epub)
			assert.Equal(t, myBookEvidenceRecovery(classical), recovery)
			assert.Empty(t, recovery)
		})
	}
}

func TestEPUBAcquisitionFailureDoesNotOverrideAnAnalysisInEffect(t *testing.T) {
	t.Parallel()
	book := acquisitionFailureBook(domain.EPUBAcquisitionStatus{
		Stage: domain.BookPreparationStageFetch, Code: domain.BookPreparationCodeSourceDownloadFailed,
	})
	book.Acquired = &domain.SourceMaterialSummary{
		Source:  domain.SourceMaterial{MediaType: "application/epub+zip"},
		Signals: domain.AnalysisSignals{Content: domain.ContentCurrentEPUB, Published: domain.PublishedCurrent, LatestRun: domain.RunCompleted},
	}
	assert.NotEqual(t, epubAcquisitionDownloadFailedLabel, myBookMarginEvidence(book))
	assert.Empty(t, myBookEvidenceRecovery(book))
}

func TestEPUBAcquisitionFailureDoesNotPresentOutsideToRead(t *testing.T) {
	t.Parallel()
	book := acquisitionFailureBook(domain.EPUBAcquisitionStatus{
		Stage: domain.BookPreparationStageFetch, Code: domain.BookPreparationCodeSourceDownloadFailed,
	})
	book.Disposition = domain.BookDispositionInbox
	assert.Empty(t, myBookMarginEvidence(book))
	assert.Empty(t, myBookEvidenceRecovery(book))

	book.CompletionCount = 1
	assert.Empty(t, myBookMarginEvidence(book))
	assert.Empty(t, myBookEvidenceRecovery(book))
}
