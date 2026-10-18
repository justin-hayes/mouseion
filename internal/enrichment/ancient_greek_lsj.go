package enrichment

import (
	"context"
	"strings"
)

// LSJSenseSummaryLine is one line of a provider-written LSJ sense summary. It
// cites the LSJ sense IDs the line condenses, so a summary line can be traced
// back to the exact senses it summarizes. Every cited ID must exist in the
// summarized entry.
type LSJSenseSummaryLine struct {
	SenseIDs []string `json:"sense_ids"`
	Text     string   `json:"text"`
}

// LSJSenseSummary is a provider-written, context-free condensation of one pinned
// LSJ entry. It is shown above the entry it condenses and never replaces it, and
// it never chooses a sense for a representative sentence (ADR 0101). EntryID and
// Pin are the identity of the entry it summarizes.
type LSJSenseSummary struct {
	EntryID string                `json:"entry_id"`
	Pin     string                `json:"pin"`
	Lines   []LSJSenseSummaryLine `json:"lines"`
}

// LSJSenseSummaryKey is the durable cache identity of one LSJ sense summary: the
// pinned LSJ revision, the entry's native identity, and the prompt version that
// produced it (ADR 0101). It deliberately carries no learner identity and no
// Book data, so a summary is shared across decks and occurrences.
type LSJSenseSummaryKey struct {
	Pin, EntryID, PromptVersion string
}

// LSJSenseSummarySense is one citable sense of the summarized entry: the sense's
// native LSJ identity and the English glosses the summary condenses.
type LSJSenseSummarySense struct {
	ID      string   `json:"id"`
	Glosses []string `json:"glosses"`
}

// MaxLSJSenseSummaryLines bounds an LSJ sense summary to a few short lines, so a
// runaway provider response is discarded rather than shown in full (ADR 0101).
const MaxLSJSenseSummaryLines = 8

// MaxLSJSenseSummaryLineRunes bounds one summary line's length.
const MaxLSJSenseSummaryLineRunes = 160

// LSJSenseIDs returns the set of native sense IDs a summary of these senses may
// cite, so callers validate a citation against the exact pinned senses.
func LSJSenseIDs(senses []LSJSenseSummarySense) map[string]struct{} {
	ids := make(map[string]struct{}, len(senses))
	for _, sense := range senses {
		if id := strings.TrimSpace(sense.ID); id != "" {
			ids[id] = struct{}{}
		}
	}
	return ids
}

// ValidateLSJSenseSummary accepts a provider-written summary only when it is a
// few short lines and every line cites at least one sense ID that exists in the
// summarized entry. A summary that cites an unknown sense ID, or cites none, or
// runs to more than a few short lines, is discarded so the card keeps its full
// LSJ entry rather than a partial one (ADR 0101).
func ValidateLSJSenseSummary(summary LSJSenseSummary, knownSenseIDs map[string]struct{}) (LSJSenseSummary, bool) {
	if len(summary.Lines) == 0 || len(summary.Lines) > MaxLSJSenseSummaryLines {
		return LSJSenseSummary{}, false
	}
	validated := LSJSenseSummary{EntryID: summary.EntryID, Pin: summary.Pin, Lines: make([]LSJSenseSummaryLine, 0, len(summary.Lines))}
	for _, line := range summary.Lines {
		text := strings.TrimSpace(line.Text)
		if text == "" || len([]rune(text)) > MaxLSJSenseSummaryLineRunes || len(line.SenseIDs) == 0 {
			return LSJSenseSummary{}, false
		}
		ids := make([]string, 0, len(line.SenseIDs))
		for _, id := range line.SenseIDs {
			if _, ok := knownSenseIDs[strings.TrimSpace(id)]; !ok {
				return LSJSenseSummary{}, false
			}
			ids = append(ids, strings.TrimSpace(id))
		}
		validated.Lines = append(validated.Lines, LSJSenseSummaryLine{SenseIDs: ids, Text: text})
	}
	return validated, true
}

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
