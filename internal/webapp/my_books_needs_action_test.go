package webapp

import (
	"net/url"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
)

func needsActionBook(signals domain.AnalysisSignals) domain.MyBook {
	return domain.MyBook{
		Book:        domain.Book{LanguageState: domain.LanguageChosen, LanguageTag: "de"},
		Disposition: domain.BookDispositionToRead,
		Acquired:    &domain.SourceMaterialSummary{Signals: signals},
	}
}

func TestMyBookNeedsActionNamesEachActionableState(t *testing.T) {
	failedAcquisition := needsActionBook(domain.AnalysisSignals{})
	failedAcquisition.Acquired = nil
	failedAcquisition.EPUBAcquisition = &domain.EPUBAcquisitionStatus{
		Stage: domain.BookPreparationStageFetch, Code: domain.BookPreparationCodeSourceDownloadFailed,
	}
	languageless := needsActionBook(testAnalyzed)
	languageless.Book.LanguageState = domain.LanguageUnknown
	outdated := needsActionBook(testAnalyzed)
	outdated.Acquired.Signals.PipelineOutdated = true
	outdated.Acquired.AnalysisUpdate = &domain.AnalysisUpdateCandidate{}
	mainText := needsActionBook(testAnalyzed)
	mainText.Acquired.SourceStructureUpdate = &domain.SourceStructureUpdateCandidate{}

	reanalysisFailed := needsActionBook(domain.AnalysisSignals{Content: domain.ContentCurrentEPUB, Published: domain.PublishedCurrent, LatestRun: domain.RunFailed})

	tests := []struct {
		name string
		book domain.MyBook
		want string
	}{
		{"analysis failed", needsActionBook(testFailed), "Analysis failed."},
		{"re-analysis failed over a current publication", reanalysisFailed, "Re-analysis failed."},
		{"analysis stale", needsActionBook(testStale), "Analysis out of date."},
		{"acquisition failed", failedAcquisition, "Acquisition failed."},
		{"missing Book language", languageless, "Choose a Book language."},
		{"analysis update pending", outdated, "Analysis update available."},
		{"main-text selection", mainText, "Main-text selection update available."},
		{"healthy analysed Book", needsActionBook(testAnalyzed), ""},
		{"analysis running is not action", needsActionBook(testRunning), ""},
		{"metadata only Book", domain.MyBook{Book: domain.Book{LanguageState: domain.LanguageChosen, LanguageTag: "de"}}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := myBookNeedsAction(tt.book)
			assert.Equal(t, tt.want != "", ok)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestMyBooksNeedsActionFilterIsKeptInTheAddress(t *testing.T) {
	request, ok := parseMyBooksBrowseRequest(&url.URL{RawQuery: "needs=action&q=faust"})
	assert.True(t, ok)
	assert.True(t, request.needsAction)

	_, ok = parseMyBooksBrowseRequest(&url.URL{RawQuery: "needs=bogus"})
	assert.False(t, ok, "an unknown needs value is not My Books")

	browse := MyBooksBrowseState{Language: "de", Disposition: domain.BookDispositionToRead, NeedsAction: true, Query: "faust"}
	assert.Equal(t, "/de/library?disposition=to_read&needs=action&q=faust", myBooksResultsURL(browse, 1))
	assert.Equal(t, "/de/library?disposition=to_read&needs=action", myBooksClearSearchURL(browse))
	assert.Equal(t, "/de/library?history=read&needs=action&q=faust", myBooksReadTabURL(browse))
	assert.Equal(t, "/de/library?disposition=to_read&q=faust", myBooksNeedsActionToggleURL(MyBooksBrowseState{Language: "de", Disposition: domain.BookDispositionToRead, NeedsAction: true, Query: "faust"}))

	returned := myBooksReturnURL("/de/library?needs=action&disposition=to_read", "de")
	assert.Equal(t, "/de/library?disposition=to_read&needs=action", returned)
}
