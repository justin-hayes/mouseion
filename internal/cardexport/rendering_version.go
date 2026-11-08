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
	// dictionary or reviewed-list provenance (ADR 0114).
	RenderInputVersion = 6
	// PresentationVersion identifies the current card presentation contract.
	// Version 6 adds the hierarchical dictionary-entry block to the card back
	// (ADR 0101). Version 7 adds the French article rule, its gender label field,
	// and the HStatus evidence the rule reads (ADR 0102). Version 8 adds the
	// provider-written LSJ sense summary block above the dictionary entry
	// (ADR 0101). Version 9 adds the separate Attribution field and explicit
	// per-source-line front markup (ADR 0103). Version 10 adds the idiom card
	// front, back and stable note identity (ADR 0114).
	PresentationVersion = 10
)
