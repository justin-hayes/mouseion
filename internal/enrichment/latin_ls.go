package enrichment

import "context"

// LatinLSCitation is one citation kept on a Lewis & Short sense. Author is the
// cited author's abbreviation as the source gives it, resolved when the source
// writes "id."; it is empty when the source citation has no usable author.
type LatinLSCitation struct {
	Author string `json:"author,omitempty"`
	Text   string `json:"text"`
}

// LatinLSCrossRef is one cross-reference in a Lewis & Short sense. Text is the
// reference as written. EntryID and Headword name the target entry when the
// reference resolved to exactly one; both are empty otherwise.
type LatinLSCrossRef struct {
	Text     string `json:"text"`
	EntryID  string `json:"entry_id,omitempty"`
	Headword string `json:"headword,omitempty"`
}

// LatinLSSense is one sense of a Lewis & Short entry. Senses are flat and in
// source (id) order; Level carries the nesting and Number is a label that
// repeats within an entry, so neither orders the senses. Definitions are the
// italic glosses stated before the sense's first citation, with the
// dictionary's abbreviations left out.
type LatinLSSense struct {
	ID          string            `json:"id"`
	Number      string            `json:"n,omitempty"`
	Level       int               `json:"level,omitempty"`
	Definitions []string          `json:"definitions,omitempty"`
	Glosses     []string          `json:"glosses,omitempty"`
	Citations   []LatinLSCitation `json:"citations,omitempty"`
	CrossRefs   []LatinLSCrossRef `json:"crossrefs,omitempty"`
}

// LatinLSEntry is one Lewis & Short main entry. EntryID is the source's native
// entry identity and Pin is the pinned revision it was derived from. Orth is
// the source spelling with vowel-length marks.
type LatinLSEntry struct {
	EntryID  string `json:"entry_id"`
	Headword string `json:"headword"`
	Orth     string `json:"orth,omitempty"`
	// Homograph is the source's own L&S number (dīco¹, dīco²), taken from the
	// entry key. Zero means the key carried none.
	Homograph int            `json:"homograph,omitempty"`
	Pin       string         `json:"pin,omitempty"`
	Senses    []LatinLSSense `json:"senses,omitempty"`
}

// Empty reports whether the entry is the zero value.
func (e LatinLSEntry) Empty() bool {
	return e.EntryID == "" && e.Headword == "" && len(e.Senses) == 0
}

// LatinLSProvider is the optional local capability that resolves a Latin lemma
// to every Lewis & Short entry under its key, in entry id order. found=false is
// the normal result for a missing headword.
type LatinLSProvider interface {
	LookupLatinLS(context.Context, string) ([]LatinLSEntry, bool)
}
