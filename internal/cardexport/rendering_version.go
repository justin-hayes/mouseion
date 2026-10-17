package cardexport

const (
	// RenderInputVersion identifies the frozen specification inputs consumed by
	// the renderer. Version 2 adds the persisted sentence start offset and the
	// token offsets that anchor elided and inversion-hyphen targets (ADR 0102).
	// Version 3 adds the frozen matched dictionary entry, its native entry
	// identity and its source pin (ADR 0101).
	RenderInputVersion = 3
	// PresentationVersion identifies the current card presentation contract.
	// Version 6 adds the hierarchical dictionary-entry block to the card back.
	PresentationVersion = 6
)
