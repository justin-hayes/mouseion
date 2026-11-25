package classical

import "fmt"

// Preparation codes name every way Classical preparation refuses a source. They
// are stable, source-format independent, carry no source text, and are the only
// part of a refusal an operator sees.
const (
	CodeReadFailed       = "source_read_failed"
	CodeOversize         = "source_oversize"
	CodePinMismatch      = "source_pin_mismatch"
	CodeMalformed        = "source_malformed"
	CodeUnknownPOS       = "source_unknown_pos"
	CodeUnknownInterpret = "interpretation_unknown"
	// CodeSentenceUnassigned reports a sentence whose first token belongs to no
	// Reading unit in a whole-work cut.
	CodeSentenceUnassigned = "sentence_unassigned"
	// CodeSentenceAmbiguous reports a sentence whose first token belongs to
	// more than one Reading unit in a whole-work cut.
	CodeSentenceAmbiguous = "sentence_ambiguous"
	// CodeEmptyCitationRange reports a Reading-unit range that does not
	// resolve to real source tokens.
	CodeEmptyCitationRange = "citation_range_empty"
	// CodeOpeningMissing reports a Reading unit whose only sentences are
	// speaker labels, so it has no Opening to derive.
	CodeOpeningMissing = "opening_missing"
	// CodeUnknownSourceFormat reports a definition whose source format has no
	// import adapter.
	CodeUnknownSourceFormat = "source_format_unknown"
	// CodeSourceLayerUnknown reports a definition whose layer is absent from the
	// release's layer pins.
	CodeSourceLayerUnknown = "source_layer_unknown"
	// CodeSourceDownloadFailed reports a pinned work file that could not be
	// fetched.
	CodeSourceDownloadFailed = "source_download_failed"
	// CodeAdapterRefused is the stable aggregate code recorded when an import
	// adapter refuses a pinned source during interpretation or validation,
	// including a malformed file, an unknown POS, or a citation cut that does not
	// cover the source. The specific refusal code remains in the job's error.
	CodeAdapterRefused = "source_adapter_refused"
)

// PreparationError is a refused Classical preparation, whatever the source
// format. Code is stable and safe to print; Detail holds only ASCII
// diagnostics, never source text.
type PreparationError struct {
	Code   string
	Line   int
	Detail string
}

func (e *PreparationError) Error() string {
	message := "classical: preparation refused: " + e.Code
	if e.Line > 0 {
		message += fmt.Sprintf(" (line %d)", e.Line)
	}
	if e.Detail != "" {
		message += ": " + e.Detail
	}
	return message
}
