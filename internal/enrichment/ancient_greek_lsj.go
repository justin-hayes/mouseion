package enrichment

import (
	"context"
)

// AncientGreekLSJSense is one sense of an LSJ dictionary entry, in source
// order. It keeps the source's native sense ID and numbering label, its nesting
// level and children so the hierarchy survives, its English glosses, and the
// citations kept for it. Definitions are the glosses LSJ states before the
// sense's first citation; they are empty in an index derived before they
// existed. The glosses and citations are plain text; any markup is added by
// the renderer.
type AncientGreekLSJSense struct {
	ID          string                 `json:"id"`
	Number      string                 `json:"n,omitempty"`
	Level       int                    `json:"level,omitempty"`
	Definitions []string               `json:"definitions,omitempty"`
	Glosses     []string               `json:"glosses,omitempty"`
	Citations   []string               `json:"citations,omitempty"`
	Senses      []AncientGreekLSJSense `json:"senses,omitempty"`
}

// AncientGreekLSJEntry is one LSJ main entry matched to an Ancient Greek
// identity. EntryID is the source's native entry identity and Pin is the pinned
// LSJ revision the entry was derived from.
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

// AncientGreekLSJEntriesProvider is the optional local capability that resolves
// an Ancient Greek headword to every competing LSJ entry, in source order
// (ADR 0139). found=false means no entry competes.
type AncientGreekLSJEntriesProvider interface {
	LookupAncientGreekLSJEntries(context.Context, string) ([]AncientGreekLSJEntry, bool)
}
