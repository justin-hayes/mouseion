package enrichment

import "context"

// AncientGreekLSJSense is one sense of an LSJ dictionary entry, in source
// order. It keeps the source's native sense ID and numbering label, its nesting
// level and children so the hierarchy survives, its English glosses, and the
// citations kept for it. The glosses and citations are plain text; any markup
// is added by the renderer.
type AncientGreekLSJSense struct {
	ID        string                 `json:"id"`
	Number    string                 `json:"n,omitempty"`
	Level     int                    `json:"level,omitempty"`
	Glosses   []string               `json:"glosses,omitempty"`
	Citations []string               `json:"citations,omitempty"`
	Senses    []AncientGreekLSJSense `json:"senses,omitempty"`
}

// AncientGreekLSJEntry is one LSJ main entry matched to an Ancient Greek
// identity. EntryID is the source's native entry identity and Pin is the pinned
// LSJ revision the entry was derived from; both are frozen onto a prepared deck
// item so the deck never changes when the index is re-derived.
type AncientGreekLSJEntry struct {
	EntryID  string                 `json:"entry_id"`
	Headword string                 `json:"headword"`
	Pin      string                 `json:"pin,omitempty"`
	Senses   []AncientGreekLSJSense `json:"senses,omitempty"`
}

// Empty reports whether the entry carries no matched LSJ entry, so callers can
// treat a zero value as "no entry" without inspecting every field.
func (e AncientGreekLSJEntry) Empty() bool {
	return e.EntryID == "" && e.Headword == "" && len(e.Senses) == 0
}

// AncientGreekLSJProvider is the optional local capability that resolves an
// Ancient Greek headword to its matched LSJ entry. found=false is the normal
// result for a missing or ambiguous headword and must be treated as "no entry",
// never as a guess.
type AncientGreekLSJProvider interface {
	LookupAncientGreekLSJ(context.Context, string) (AncientGreekLSJEntry, bool)
}
