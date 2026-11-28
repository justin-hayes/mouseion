package webapp

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func classicalStatusBook(status domain.ClassicalPreparationStatus) domain.MyBook {
	return domain.MyBook{
		Book:                 domain.Book{ID: "greek", Title: "Anabasis", LanguageState: domain.LanguageChosen, LanguageTag: "grc"},
		Disposition:          domain.BookDispositionToRead,
		ReadingUnit:          &domain.BookReadingUnit{ReadingUnitKey: "greek-1", Revision: status.CurrentRevision, Withdrawn: status.Withdrawn},
		ClassicalPreparation: &status,
	}
}

func TestClassicalPreparationLearnerStates(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		status     domain.ClassicalPreparationStatus
		label      string
		recovery   string
		margin     string
		action     bool
		inProgress bool
	}{
		{name: "queued", status: domain.ClassicalPreparationStatus{State: domain.ClassicalPreparationQueued, CurrentRevision: 1}, label: classicalAnalysisIncompleteLabel, margin: "Analysis incomplete.", inProgress: true},
		{name: "running", status: domain.ClassicalPreparationStatus{State: domain.ClassicalPreparationRunning, CurrentRevision: 1}, label: classicalAnalysisIncompleteLabel, margin: "Analysis incomplete.", inProgress: true},
		{name: "fetch", status: domain.ClassicalPreparationStatus{Stage: domain.BookPreparationStageFetch, CurrentRevision: 1}, label: classicalAssessmentUnavailable, recovery: classicalRetryAcquisitionLabel, margin: "Assessment unavailable.", action: true},
		{name: "import", status: domain.ClassicalPreparationStatus{Stage: domain.BookPreparationStageImport, CurrentRevision: 1}, label: classicalAnalysisFailedLabel, recovery: classicalRetryAnalysisLabel, margin: "Analysis failed.", action: true},
		{name: "source", status: domain.ClassicalPreparationStatus{Stage: domain.BookPreparationStageSource, CurrentRevision: 1}, label: classicalSourceMismatchLabel, margin: "Source doesn't match its reviewed definition."},
		{name: "withdrawn", status: domain.ClassicalPreparationStatus{Stage: domain.BookPreparationStageWithdrawn, CurrentRevision: 1, Withdrawn: true}, label: classicalWithdrawnLabel, margin: "No longer in the Classical catalog."},
		{name: "cleared", status: domain.ClassicalPreparationStatus{Cleared: true, FailureRevision: 1, CurrentRevision: 2}, label: classicalAnalysisNotStarted, recovery: classicalRetryPreparationLabel, margin: "Analysis not started.", action: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			book := classicalStatusBook(tt.status)
			view, ok := classicalPreparationFor(book)
			require.True(t, ok)
			assert.Equal(t, tt.label, view.Label)
			assert.Equal(t, tt.recovery, view.Recovery)
			assert.Equal(t, tt.action, view.Actionable)
			assert.Equal(t, tt.inProgress, view.InProgress)
			assert.Equal(t, tt.margin, myBookMarginEvidence(book))
			assert.Equal(t, tt.recovery, myBookEvidenceRecovery(book))
			if view.Actionable {
				assert.Equal(t, "/reading/books/greek/reanalyze", myBookEvidenceRecoveryURL(book))
			}
		})
	}
}

func TestClassicalPreparationRecoveryFromInboxMovesToToRead(t *testing.T) {
	t.Parallel()
	book := classicalStatusBook(domain.ClassicalPreparationStatus{Stage: domain.BookPreparationStageFetch, CurrentRevision: 1})
	book.Disposition = domain.BookDispositionInbox
	assert.Equal(t, classicalRetryAcquisitionLabel+" by moving to To Read", myBookEvidenceRecovery(book))
	assert.Equal(t, "/library/books/greek/to-read", myBookEvidenceRecoveryURL(book))
}

func TestClassicalPreparationIgnoresPublishedAnalysis(t *testing.T) {
	t.Parallel()
	book := classicalStatusBook(domain.ClassicalPreparationStatus{Stage: domain.BookPreparationStageSource, CurrentRevision: 1})
	book.Acquired = &domain.SourceMaterialSummary{
		Source:        domain.SourceMaterial{ID: "source", MediaType: domain.ImportedSourceMediaType, ContentRevisionID: "revision", ContentSnapshotID: "snapshot"},
		Signals:       testAnalyzed,
		AnalysisRunID: "run",
		CorpusID:      "corpus",
	}
	_, ok := classicalPreparationFor(book)
	assert.False(t, ok, "a published analysis is ordinary evidence, not a preparation failure")
}

func TestClassicalPreparationReadingActionAndLabel(t *testing.T) {
	t.Parallel()
	book := classicalStatusBook(domain.ClassicalPreparationStatus{Stage: domain.BookPreparationStageFetch, CurrentRevision: 1})
	item := readingBookView{Book: domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: "greek", Title: "Anabasis"}}, BookID: "greek", ReadingUnit: book.ReadingUnit, ClassicalPreparation: book.ClassicalPreparation}
	assert.Equal(t, classicalAssessmentUnavailable, readingEvidenceLabel(item))
	assert.Contains(t, readingEvidenceDescription(item), "Retry acquisition")
	action := readingAnalysisAction(item)
	assert.Equal(t, classicalAssessmentUnavailable, action.Status)
	assert.Equal(t, classicalRetryAcquisitionLabel, action.Label)
	assert.Equal(t, "/reading/books/greek/reanalyze", action.URL)
	assert.True(t, action.Submit)

	terminal := readingBookView{Book: item.Book, BookID: "greek", ReadingUnit: book.ReadingUnit, ClassicalPreparation: &domain.ClassicalPreparationStatus{Stage: domain.BookPreparationStageSource, CurrentRevision: 1}}
	action = readingAnalysisAction(terminal)
	assert.Equal(t, classicalSourceMismatchLabel, action.Status)
	assert.Empty(t, action.Label, "a terminal source failure offers no retry")
	assert.False(t, action.Submit)
}

func TestClassicalPreparationRecoveryAfterCompletionOffersReadingAgain(t *testing.T) {
	t.Parallel()
	book := classicalStatusBook(domain.ClassicalPreparationStatus{Cleared: true, FailureRevision: 1, CurrentRevision: 2})
	book.Disposition = domain.BookDispositionInbox
	book.CompletionCount = 1
	assert.Equal(t, classicalRetryPreparationLabel+" by reading again", myBookEvidenceRecovery(book))
	assert.Equal(t, "/library/books/greek/read-again", myBookEvidenceRecoveryURL(book))
}

func TestClassicalPreparationForWithdrawnUnitWithoutStatus(t *testing.T) {
	t.Parallel()
	book := domain.MyBook{
		Book:        domain.Book{ID: "greek", Title: "Anabasis", LanguageState: domain.LanguageChosen, LanguageTag: "grc"},
		Disposition: domain.BookDispositionToRead,
		ReadingUnit: &domain.BookReadingUnit{ReadingUnitKey: "greek-1", Revision: 2, Withdrawn: true},
	}
	view, ok := classicalPreparationFor(book)
	require.True(t, ok)
	assert.Equal(t, classicalWithdrawnLabel, view.Label)
	assert.Equal(t, classicalWithdrawnDescription, view.Description)
	assert.False(t, view.Actionable)
}

func TestReadingClassicalWithdrawnFallbackAndCatalogState(t *testing.T) {
	t.Parallel()
	withdrawn := readingBookView{
		Book:        domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: "greek"}},
		BookID:      "greek",
		ReadingUnit: &domain.BookReadingUnit{ReadingUnitKey: "greek-1", Revision: 2, Withdrawn: true},
	}
	assert.Equal(t, domain.PreparationWithdrawn, withdrawn.classification().Preparation.State)
	assert.Equal(t, classicalWithdrawnLabel, readingEvidenceLabel(withdrawn))
	assert.Equal(t, readingEvidenceFailed, readingEvidenceState(withdrawn))

	catalog := readingBookView{
		Book:   domain.SourceMaterialSummary{Source: domain.SourceMaterial{ID: "greek"}},
		BookID: "greek",
		ClassicalPreparation: &domain.ClassicalPreparationStatus{
			Stage: domain.BookPreparationStageCatalog, CurrentRevision: 1,
		},
	}
	assert.Equal(t, readingEvidenceUnavailable, readingEvidenceState(catalog))
	assert.Empty(t, readingAnalysisAction(catalog).Label)
}
