package cardexport

const (
	// RenderInputVersion identifies the frozen specification inputs consumed by
	// the renderer. Version 2 adds the persisted sentence start offset and the
	// token offsets that anchor elided and inversion-hyphen targets (ADR 0102).
	RenderInputVersion = 2
	// PresentationVersion identifies the current card presentation contract.
	PresentationVersion = 5
)
