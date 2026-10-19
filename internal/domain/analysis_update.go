package domain

// AnalysisPipelineIdentity is the analyzer-side identity that produced an
// analysis (ADR 0107): the Go analyzer name and version and the selection
// configuration identity, together with the language's normalization profile
// and version reported by the running analysis service. Model and dependency
// versions are deliberately not part of it.
type AnalysisPipelineIdentity struct {
	AnalyzerName         string
	AnalyzerVersion      string
	ConfigIdentity       string
	NormalizationProfile string
	NormalizationVersion string
}

// AnalysisUpdateTarget is the part of the current Analysis pipeline identity
// that is known without reading a Book's snapshot: the analyzer and the
// language's normalization. An offer carries it so confirmation can detect
// that the running service changed in the meantime.
type AnalysisUpdateTarget struct {
	AnalyzerName         string
	AnalyzerVersion      string
	NormalizationProfile string
	NormalizationVersion string
}

// Key is the stable form a confirmation form carries back to the server.
func (t AnalysisUpdateTarget) Key() string {
	return t.AnalyzerName + "/" + t.AnalyzerVersion + "/" + t.NormalizationProfile + "/" + t.NormalizationVersion
}

// AnalysisUpdateCandidate is a Book's published analysis whose Analysis
// pipeline identity differs from the current one. It records what the
// confirmation must disclose: the confirmed Lemma corrections and Occurrence
// exclusions on the published analysis, which do not carry over.
type AnalysisUpdateCandidate struct {
	PublishedRunID       string
	Target               AnalysisUpdateTarget
	LemmaCorrections     int
	OccurrenceExclusions int
}

// AnalysisUpdateOffer is the Analysis update a Book is offered. It is not an
// evidence state: the published analysis stays in effect whatever the offer.
type AnalysisUpdateOffer string

const (
	AnalysisUpdateNone AnalysisUpdateOffer = "none"
	// AnalysisUpdateAvailable offers the learner-confirmed update.
	AnalysisUpdateAvailable AnalysisUpdateOffer = "available"
	// AnalysisUpdateEndReadingFirst names the update for the Current reading's
	// Book, whose frozen evidence cannot be refreshed until the Reading ends.
	AnalysisUpdateEndReadingFirst AnalysisUpdateOffer = "end-reading-first"
)

// analysisUpdateOffer offers an update only for a To Read EPUB Book whose
// published analysis is in effect and whose latest run completed. A newer run
// that is queued, running, failed, or cancelled keeps its own status and
// retry instead; imported analyses have no NLP producer and are never offered.
func analysisUpdateOffer(content ContentSignal, signals AnalysisSignals, disposition BookDisposition) AnalysisUpdateOffer {
	if !signals.PipelineOutdated || content != ContentCurrentEPUB || signals.Published != PublishedCurrent || signals.LatestRun != RunCompleted {
		return AnalysisUpdateNone
	}
	if signals.CurrentReading {
		return AnalysisUpdateEndReadingFirst
	}
	if disposition != BookDispositionToRead {
		return AnalysisUpdateNone
	}
	return AnalysisUpdateAvailable
}
