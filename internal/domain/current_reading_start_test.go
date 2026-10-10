package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDecideStart(t *testing.T) {
	eligible := StartFacts{
		Language:          "de",
		Signals:           AnalysisSignals{Content: ContentCurrentEPUB, Published: PublishedCurrent, LatestRun: RunCompleted},
		Disposition:       BookDispositionToRead,
		BookLanguage:      "de",
		IdentityPublished: true,
	}
	with := func(change func(*StartFacts)) StartFacts {
		facts := eligible
		change(&facts)
		return facts
	}
	tests := []struct {
		name    string
		facts   StartFacts
		outcome StartOutcome
		reason  CurrentReadingEligibilityReason
	}{
		{"accepted", eligible, StartAccepted, CurrentReadingEligible},
		{
			"accepted under a newer failed run",
			with(func(f *StartFacts) { f.Signals.LatestRun = RunFailed }),
			StartAccepted, CurrentReadingEligible,
		},
		{
			"already current",
			with(func(f *StartFacts) { f.AlreadyCurrent = true }),
			StartRejectedAlreadyCurrent, CurrentReadingEligible,
		},
		{
			"not To Read",
			with(func(f *StartFacts) { f.Disposition = BookDispositionInbox }),
			StartRejectedIneligible, CurrentReadingNotToRead,
		},
		{
			"no chosen language",
			with(func(f *StartFacts) { f.BookLanguage = "" }),
			StartRejectedIneligible, CurrentReadingNoChosenLanguage,
		},
		{
			"other language",
			with(func(f *StartFacts) { f.BookLanguage = "it" }),
			StartRejectedIneligible, CurrentReadingOtherLanguage,
		},
		{
			"needs current content",
			with(func(f *StartFacts) { f.Signals = AnalysisSignals{Content: ContentNotAcquired} }),
			StartRejectedIneligible, CurrentReadingNeedsCurrentContent,
		},
		{
			"analysis in progress",
			with(func(f *StartFacts) { f.Signals = AnalysisSignals{Content: ContentCurrentEPUB, LatestRun: RunRunning} }),
			StartRejectedIneligible, CurrentReadingAnalysisInProgress,
		},
		{
			"failed",
			with(func(f *StartFacts) { f.Signals = AnalysisSignals{Content: ContentCurrentEPUB, LatestRun: RunFailed} }),
			StartRejectedIneligible, CurrentReadingFailed,
		},
		{
			"cancelled",
			with(func(f *StartFacts) { f.Signals = AnalysisSignals{Content: ContentCurrentEPUB, LatestRun: RunCancelled} }),
			StartRejectedIneligible, CurrentReadingCancelled,
		},
		{
			"stale",
			with(func(f *StartFacts) {
				f.Signals = AnalysisSignals{Content: ContentCurrentEPUB, Published: PublishedStale, LatestRun: RunCompleted}
			}),
			StartRejectedIneligible, CurrentReadingStale,
		},
		{
			"no completed analysis",
			with(func(f *StartFacts) { f.Signals = AnalysisSignals{Content: ContentCurrentEPUB} }),
			StartRejectedIneligible, CurrentReadingNoCompletedAnalysis,
		},
		{
			"eligible without a published identity",
			with(func(f *StartFacts) { f.IdentityPublished = false }),
			StartRejectedIneligible, CurrentReadingNoCompletedAnalysis,
		},
		{
			"unresolved lemma review flags",
			with(func(f *StartFacts) { f.UnresolvedLemmaReviewFlags = true }),
			StartRejectedUnresolvedFlags, CurrentReadingEligible,
		},
		{
			"already current wins over ineligibility",
			with(func(f *StartFacts) { f.AlreadyCurrent = true; f.Disposition = BookDispositionInbox }),
			StartRejectedAlreadyCurrent, CurrentReadingEligible,
		},
		{
			"ineligibility wins over unresolved flags",
			with(func(f *StartFacts) { f.Disposition = BookDispositionInbox; f.UnresolvedLemmaReviewFlags = true }),
			StartRejectedIneligible, CurrentReadingNotToRead,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := DecideStart(tt.facts)
			assert.Equal(t, tt.outcome, got.Outcome)
			assert.Equal(t, tt.reason, got.Reason)
			assert.Equal(t, tt.outcome == StartAccepted, got.Accepted())
		})
	}
}
