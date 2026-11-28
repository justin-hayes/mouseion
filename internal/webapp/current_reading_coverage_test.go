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

func TestCurrentReadingCoverageFor(t *testing.T) {
	ready := readingBookView{Coverage: &domain.AnalysisCoverage{AnalyzableTokenCount: 1000, MatureTokenCount: 370, InStudyTokenCount: 421}}
	view := currentReadingCoverageFor(ready)
	assert.True(t, view.Ready)
	assert.Equal(t, "37.0%", view.MatureText)
	assert.Equal(t, "42.1%", view.InStudyText)
	assert.InDelta(t, 37.0, view.MaturePct, 0.001)
	assert.InDelta(t, 5.1, view.ExtraPct, 0.001, "the bar stacks the share beyond Mature, since in study includes Mature")
	assert.Equal(t, "370 of 1,000 word occurrences are Mature.", view.Occurrences)
	assert.Equal(t, "Mature 37.0%; with words in study 42.1%", view.BarLabel)

	equal := readingBookView{Coverage: &domain.AnalysisCoverage{AnalyzableTokenCount: 100, MatureTokenCount: 40, InStudyTokenCount: 40}}
	assert.InDelta(t, 0, currentReadingCoverageFor(equal).ExtraPct, 0.001)

	over := readingBookView{Coverage: &domain.AnalysisCoverage{AnalyzableTokenCount: 100, MatureTokenCount: 140, InStudyTokenCount: 150}}
	view = currentReadingCoverageFor(over)
	assert.InDelta(t, 100, view.MaturePct, 0.001)
	assert.InDelta(t, 0, view.ExtraPct, 0.001)

	updating := currentReadingCoverageFor(readingBookView{CountsUpdating: true})
	assert.False(t, updating.Ready)
	assert.Equal(t, "Updating", updating.Word)

	for name, item := range map[string]readingBookView{
		"no coverage":        {},
		"statistics missing": {Coverage: ready.Coverage, StatisticsUnavailable: true},
		"no analyzable":      {Coverage: &domain.AnalysisCoverage{}},
	} {
		view := currentReadingCoverageFor(item)
		assert.False(t, view.Ready, name)
		assert.Equal(t, "Unavailable", view.Word, name)
	}
}
