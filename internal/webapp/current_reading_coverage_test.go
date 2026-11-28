package webapp

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
)

func TestReadingCoverageToTargetText(t *testing.T) {
	cover := func(target domain.CoverageThreshold) domain.AnalysisCoverage {
		return domain.AnalysisCoverage{AnalyzableTokenCount: 1000, ToTarget: target}
	}
	assert.Equal(t, "About 1,234 more words to reach Target coverage (98%).",
		readingCoverageToTargetText(cover(domain.CoverageThreshold{TargetPercent: 98, LemmaCount: 1234, Reachable: true})))
	assert.Equal(t, "Target coverage (98%) reached.",
		readingCoverageToTargetText(cover(domain.CoverageThreshold{TargetPercent: 98, Reachable: true})))
	assert.Equal(t, "Target coverage (98%): not reachable with the words in this Book.",
		readingCoverageToTargetText(cover(domain.CoverageThreshold{TargetPercent: 98, LemmaCount: 5})))
	assert.Equal(t, "No words to compare for Target coverage.", readingCoverageToTargetText(domain.AnalysisCoverage{}))
	assert.Equal(t, "No words to compare", readingCoverageMatureDisplay(domain.AnalysisCoverage{}))
}

func TestMyBookCanReadNext(t *testing.T) {
	ready := domain.MyBook{
		Book:        domain.Book{LanguageState: domain.LanguageChosen, LanguageTag: "de"},
		Disposition: domain.BookDispositionToRead,
		Acquired:    &domain.SourceMaterialSummary{Source: domain.SourceMaterial{MediaType: "application/epub+zip", ContentRevisionID: "r", ContentSnapshotID: "s"}, AnalysisRunID: "run", CorpusID: "corpus", Signals: testAnalyzed},
	}
	assert.True(t, myBookCanReadNext(ready))

	current := ready
	current.IsCurrentReading = true
	assert.False(t, myBookCanReadNext(current), "the pinned Book is already the Current reading")

	noLanguage := ready
	noLanguage.Book.LanguageState = domain.LanguageUnknown
	assert.False(t, myBookCanReadNext(noLanguage), "a Book without a Book language cannot be read next")

	stale := ready
	staleSource := *ready.Acquired
	staleSource.Signals = testStale
	stale.Acquired = &staleSource
	assert.False(t, myBookCanReadNext(stale))

	read := ready
	read.Disposition = domain.BookDispositionInbox
	assert.False(t, myBookCanReadNext(read))
}
