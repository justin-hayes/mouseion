package domain

import "time"

// EPUBAcquisitionFailureStage names the step that stopped an EPUB acquisition.
// It reuses Classical preparation's stage names where the meaning matches and
// adds entry for a catalog entry that no longer resolves.
type EPUBAcquisitionFailureStage string

const (
	EPUBAcquisitionFailureNone   EPUBAcquisitionFailureStage = ""
	EPUBAcquisitionFailureEntry  EPUBAcquisitionFailureStage = "entry"
	EPUBAcquisitionFailureFetch  EPUBAcquisitionFailureStage = "fetch"
	EPUBAcquisitionFailureSource EPUBAcquisitionFailureStage = "source"
	EPUBAcquisitionFailureImport EPUBAcquisitionFailureStage = "import"
)

// Stable EPUB acquisition failure codes. They are the durable values recorded
// by reading intent; the domain spells them here so presentation and recovery
// share one vocabulary.
const (
	EPUBAcquisitionCodeCatalogEntryMissing  = "catalog_entry_missing"
	EPUBAcquisitionCodeSourceDownloadFailed = "source_download_failed"
	EPUBAcquisitionCodeSourceOversize       = "source_oversize"
	EPUBAcquisitionCodeSourceFormatUnknown  = "source_format_unknown"
	EPUBAcquisitionCodeEPUBImportFailed     = "epub_import_failed"
)

// EPUBAcquisitionFailureRecord is the durable per-Book evidence of the latest
// failed EPUB acquisition. ConnectionID and EntryIdentifier are empty when the
// failing attempt could not resolve the catalog entry. It never carries source
// text.
type EPUBAcquisitionFailureRecord struct {
	OwnerID         string
	BookID          string
	ConnectionID    string
	EntryIdentifier string
	Stage           EPUBAcquisitionFailureStage
	Code            string
	FailedAt        time.Time
}

// EPUBAcquisitionStatus is the failure projection read onto a Book. It carries
// the stable stage and code, never a raw error.
type EPUBAcquisitionStatus struct {
	Stage EPUBAcquisitionFailureStage
	Code  string
}

// EPUBAcquisitionFailureForCode returns the stage that owns a stable EPUB
// acquisition failure code. An unrecognised code is a fetch-stage failure: the
// acquisition stopped before the source could be imported.
func EPUBAcquisitionFailureForCode(code string) EPUBAcquisitionFailureStage {
	switch code {
	case EPUBAcquisitionCodeCatalogEntryMissing:
		return EPUBAcquisitionFailureEntry
	case EPUBAcquisitionCodeSourceDownloadFailed:
		return EPUBAcquisitionFailureFetch
	case EPUBAcquisitionCodeSourceOversize, EPUBAcquisitionCodeSourceFormatUnknown:
		return EPUBAcquisitionFailureSource
	case EPUBAcquisitionCodeEPUBImportFailed:
		return EPUBAcquisitionFailureImport
	case "":
		return EPUBAcquisitionFailureNone
	default:
		return EPUBAcquisitionFailureFetch
	}
}

// AcquisitionState is the learner-facing state of an EPUB acquisition failure.
// It names the stage without carrying prose or a raw code.
type AcquisitionState string

const (
	AcquisitionNone                AcquisitionState = ""
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
	stage := status.Stage
	if stage == EPUBAcquisitionFailureNone {
		stage = EPUBAcquisitionFailureForCode(status.Code)
	}
	switch stage {
	case EPUBAcquisitionFailureEntry:
		return acquisitionEvidence{AcquisitionClassification{State: AcquisitionEntryMissing}, BookUnavailable, PhaseNotAnalyzed, RecoveryRetryAcquisition}
	case EPUBAcquisitionFailureFetch:
		return acquisitionEvidence{AcquisitionClassification{State: AcquisitionFetchFailed}, BookUnavailable, PhaseNotAnalyzed, RecoveryRetryAcquisition}
	case EPUBAcquisitionFailureSource:
		state := AcquisitionSourceRefused
		switch status.Code {
		case EPUBAcquisitionCodeSourceOversize:
			state = AcquisitionSourceOversize
		case EPUBAcquisitionCodeSourceFormatUnknown:
			state = AcquisitionSourceFormatUnknown
		}
		return acquisitionEvidence{AcquisitionClassification{State: state}, BookUnavailable, PhaseNotAnalyzed, RecoveryNone}
	case EPUBAcquisitionFailureImport:
		return acquisitionEvidence{AcquisitionClassification{State: AcquisitionImportFailed}, BookUnavailable, PhaseFailed, RecoveryRetryAcquisition}
	case EPUBAcquisitionFailureNone:
	}
	return acquisitionEvidence{}
}
