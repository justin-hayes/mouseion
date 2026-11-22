package domain

import "time"

// EPUBAcquisitionFailureRecord is the durable per-Book evidence of the latest
// failed EPUB acquisition. ConnectionID and EntryIdentifier are empty when the
// failing attempt could not resolve the catalog entry. It never carries source
// text.
type EPUBAcquisitionFailureRecord struct {
	OwnerID         string
	BookID          string
	ConnectionID    string
	EntryIdentifier string
	Stage           BookPreparationStage
	Code            BookPreparationCode
	FailedAt        time.Time
}

// EPUBAcquisitionStatus is the acquisition projection read onto a Book. It
// carries the shared lifecycle state of a live job or the stable stage and code
// of a settled failure, never a raw error.
type EPUBAcquisitionStatus struct {
	State JobLifecycleState
	Stage BookPreparationStage
	Code  BookPreparationCode
}

// AcquisitionState is the learner-facing state of an EPUB acquisition. It names
// a live job or a failed stage without carrying prose or a raw code.
type AcquisitionState string

const (
	AcquisitionNone                AcquisitionState = ""
	AcquisitionInProgress          AcquisitionState = "in-progress"
	AcquisitionEntryMissing        AcquisitionState = "entry-missing"
	AcquisitionFetchFailed         AcquisitionState = "fetch-failed"
	AcquisitionSourceOversize      AcquisitionState = "source-oversize"
	AcquisitionSourceFormatUnknown AcquisitionState = "source-format-unknown"
	AcquisitionSourceRefused       AcquisitionState = "source-refused"
	AcquisitionImportFailed        AcquisitionState = "import-failed"
)

// AcquisitionClassification is the Book's EPUB acquisition state. It is empty
// when no acquisition failure applies, which includes every Book whose
// analysis is already in effect.
type AcquisitionClassification struct {
	State AcquisitionState
}

// Applies reports whether the classification presents an acquisition failure.
func (a AcquisitionClassification) Applies() bool {
	return a.State != AcquisitionNone
}

// acquisitionEvidence is the classification fragment an EPUB acquisition
// failure contributes while no analysis is in effect.
type acquisitionEvidence struct {
	classification AcquisitionClassification
	evidence       BookEvidenceState
	phase          AnalysisPhase
	recovery       BookEvidenceRecovery
}

// classifyEPUBAcquisition maps a durable failure onto the evidence state,
// phase and one recovery action the learner sees. A nil or empty status
// classifies as no failure.
func classifyEPUBAcquisition(status *EPUBAcquisitionStatus) acquisitionEvidence {
	if status == nil {
		return acquisitionEvidence{}
	}
	if status.State.Active() {
		return acquisitionEvidence{AcquisitionClassification{State: AcquisitionInProgress}, BookAcquiredUnassessed, PhaseAnalyzing, RecoveryNone}
	}
	switch status.Stage {
	case BookPreparationStageEntry:
		return acquisitionEvidence{AcquisitionClassification{State: AcquisitionEntryMissing}, BookUnavailable, PhaseNotAnalyzed, RecoveryRetryAcquisition}
	case BookPreparationStageFetch:
		return acquisitionEvidence{AcquisitionClassification{State: AcquisitionFetchFailed}, BookUnavailable, PhaseNotAnalyzed, RecoveryRetryAcquisition}
	case BookPreparationStageSource:
		state := AcquisitionSourceRefused
		switch status.Code {
		case BookPreparationCodeSourceOversize:
			state = AcquisitionSourceOversize
		case BookPreparationCodeSourceFormatUnknown:
			state = AcquisitionSourceFormatUnknown
		default:
			// Every other source-stage code stays a refused source.
		}
		return acquisitionEvidence{AcquisitionClassification{State: state}, BookUnavailable, PhaseNotAnalyzed, RecoveryNone}
	case BookPreparationStageImport:
		return acquisitionEvidence{AcquisitionClassification{State: AcquisitionImportFailed}, BookUnavailable, PhaseFailed, RecoveryRetryAcquisition}
	case BookPreparationStageNone, BookPreparationStageCatalog, BookPreparationStageWithdrawn,
		BookPreparationStageAnalysis, BookPreparationStageFinalize:
		// Not an EPUB acquisition failure stage.
	}
	return acquisitionEvidence{}
}
