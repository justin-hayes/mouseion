package cardexport

const (
	// RenderInputVersion identifies the frozen specification inputs consumed by
	// the renderer. Version 2 adds the persisted sentence start offset and the
	// token offsets that anchor elided and inversion-hyphen targets (ADR 0102).
	// Version 3 adds the frozen matched dictionary entry, its native entry
	// identity and its source pin (ADR 0101). Version 4 adds the frozen LSJ sense
	// summary provider version (ADR 0101). Version 5 adds the optional typed
	// source attribution and source-line presentation contract (ADR 0103).
	// Version 6 adds the frozen idiom matched spans, anchor identity and
	// dictionary or reviewed-list provenance (ADR 0114). Version 7 lets a frozen
	// item record several dictionary entry IDs, joined, for the Lewis & Short
	// entries of a Latin card (ADR 0133); the frozen block also carries the
	// target outline of a resolved cross-reference. Latin ships with this spec, so
	// that frozen shape is part of version 7 and needs no further bump.
	RenderInputVersion = 7
	// PresentationVersion identifies the current card presentation contract.
	// Version 6 adds the hierarchical dictionary-entry block to the card back
	// (ADR 0101). Version 7 adds the French article rule, its gender label field,
	// and the HStatus evidence the rule reads (ADR 0102). Version 8 adds the
	// provider-written LSJ sense summary block above the dictionary entry
	// (ADR 0101). Version 9 adds the separate Attribution field and explicit
	// per-source-line front markup (ADR 0103). Version 10 adds the idiom card
	// front, back and stable note identity (ADR 0114). Version 11 adds the
	// stacked Lewis & Short entries of a Latin card and their language-neutral
	// entry classes (ADR 0133).
	PresentationVersion = 11
)

// DecidePreparedDeckFreshness reports whether a completed run's published deck
// is current for the requested versions, and the presentation version a
// rerender should publish.
//
// A deck is current when the run's presentation version is at least the
// requested one and the newer of the run's frozen render inputs and the
// preparation's rendered artifact inputs is at least the current render-input
// version. A rerendered artifact records the version it was rendered at on the
// preparation while the run keeps the version its inputs were frozen at, so
// both must be considered.
//
// The published version is the requested one, raised to the run's own when the
// run is already ahead.
func DecidePreparedDeckFreshness(runPresentationVersion, runRenderInputVersion, preparationRenderInputVersion, requestedPresentationVersion int) (upToDate bool, publishPresentationVersion int) {
	publishPresentationVersion = max(requestedPresentationVersion, runPresentationVersion)
	upToDate = runPresentationVersion >= publishPresentationVersion && max(runRenderInputVersion, preparationRenderInputVersion) >= RenderInputVersion
	return upToDate, publishPresentationVersion
}
