package webapp

import "github.com/justin-hayes/mouseion/internal/domain"

// Typed analysis evidence for presentation tests. Each value is the signal set
// persistence derives for a Book in that state.
var (
	testAnalyzed    = domain.AnalysisSignals{Content: domain.ContentCurrentEPUB, Published: domain.PublishedCurrent, LatestRun: domain.RunCompleted}
	testNotAnalyzed = domain.AnalysisSignals{Content: domain.ContentCurrentEPUB}
	testQueued      = domain.AnalysisSignals{Content: domain.ContentCurrentEPUB, LatestRun: domain.RunQueued}
	testRunning     = domain.AnalysisSignals{Content: domain.ContentCurrentEPUB, LatestRun: domain.RunRunning}
	testFailed      = domain.AnalysisSignals{Content: domain.ContentCurrentEPUB, LatestRun: domain.RunFailed}
	testCancelled   = domain.AnalysisSignals{Content: domain.ContentCurrentEPUB, LatestRun: domain.RunCancelled}
	testStale       = domain.AnalysisSignals{Content: domain.ContentCurrentEPUB, Published: domain.PublishedStale}
	testNoContent   = domain.AnalysisSignals{Content: domain.ContentNoCurrentRevision}
	testPending     = domain.AnalysisSignals{Content: domain.ContentCurrentEPUB, LatestRun: domain.RunPublicationPending}
)
