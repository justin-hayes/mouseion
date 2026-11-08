package enrichment

// ItemKind is the card kind a frozen deck item and its contextual provider
// cache identity belong to (ADR 0114). The zero value means a lemma item, so
// cache identities and manifest items written before the kind existed keep
// their meaning without a separate migration in Go.
type ItemKind string

const (
	// ItemKindLemma is a vocabulary lemma card. It is the persisted spelling of
	// the zero value and of every item frozen before idiom cards existed.
	ItemKindLemma ItemKind = "lemma"
	// ItemKindIdiom is a dictionary-anchored multi-word idiom card. No idiom
	// item is produced until the idiom card work lands.
	ItemKindIdiom ItemKind = "idiom"
)

// Normalized returns the persisted spelling of the kind. An unset kind means a
// lemma item.
func (k ItemKind) Normalized() ItemKind {
	if k == "" {
		return ItemKindLemma
	}
	return k
}

// Valid reports whether the kind is one this contract defines.
func (k ItemKind) Valid() bool {
	normalized := k.Normalized()
	return normalized == ItemKindLemma || normalized == ItemKindIdiom
}

// ItemKindFromStored maps a persisted kind to its in-memory value. The lemma
// spelling becomes the zero value so callers can distinguish an explicit idiom
// without special-casing the historical lemma; any other value is preserved so
// validation can reject it.
func ItemKindFromStored(stored string) ItemKind {
	kind := ItemKind(stored)
	if kind.Normalized() == ItemKindLemma {
		return ""
	}
	return kind
}
