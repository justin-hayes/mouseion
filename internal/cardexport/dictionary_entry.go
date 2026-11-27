package cardexport

import (
	"context"
	"strconv"
	"strings"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/cardpolicy"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/sourceevidence"
)

// resolveAncientGreekLSJ freezes the matched LSJ dictionary entry for one
// Ancient Greek card. It resolves nothing for another language, a provider
// without the LSJ capability, or an identity whose headword is ambiguous or
// missing: such a card shows no entry rather than a guess. The lexical
// discriminator is never passed through, because an OGA morpheus:N number is not
// LSJ's numbering. It returns the matched entry so a caller can freeze it as
// pass evidence.
func resolveAncientGreekLSJ(ctx context.Context, lexical enrichment.LexicalProvider, entry *Entry) (enrichment.AncientGreekLSJEntry, bool) {
	if entry == nil {
		return enrichment.AncientGreekLSJEntry{}, false
	}
	policy := cardpolicy.For(entry.Language)
	if policy.Entries != cardpolicy.EntryLSJ {
		return enrichment.AncientGreekLSJEntry{}, false
	}
	resolution, found := sourceevidence.Resolve(ctx, lexical, policy, entry.CanonicalLemma, nil)
	if !found {
		return enrichment.AncientGreekLSJEntry{}, false
	}
	resolved := resolution.Entries[0]
	entry.DictionaryEntryID = resolution.EntryID
	entry.DictionaryPin = resolution.Pin
	entry.DictionaryEntry = renderLSJEntry(resolved)
	return resolved.LSJ, true
}

// ancientGreekLSJPassSenses is the pinned LSJ meaning evidence the
// evidence-assisted pass transmits for a matched entry.
func ancientGreekLSJPassSenses(entry enrichment.AncientGreekLSJEntry) []enrichment.LexicalSense {
	return sourceevidence.GreekPassSenses(entry)
}

// lsjSenseSummaryLabel labels the provider-written LSJ sense summary shown above
// the full entry, so it is never mistaken for source evidence (ADR 0101).
const lsjSenseSummaryLabel = "Summary of LSJ, provider-written"

// renderValidatedLSJSenseSummary renders a provider-written LSJ sense summary
// above the full entry it condenses. It renders nothing when the summary has no
// line or any line cites a sense ID that is not one of the entry's pinned
// senses: such a summary is discarded and the card keeps its full LSJ entry
// (ADR 0101). Every text piece is escaped before it is wrapped, so provider text
// cannot inject markup.
func renderValidatedLSJSenseSummary(summary enrichment.LSJSenseSummary, senses []enrichment.LexicalSense) string {
	known := make(map[string]struct{}, len(senses))
	for _, sense := range senses {
		if id := strings.TrimSpace(sense.EvidenceID); id != "" {
			known[id] = struct{}{}
		}
	}
	validated, ok := enrichment.ValidateLSJSenseSummary(summary, known)
	if !ok {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<div class="lsj-summary"><div class="lsj-summary-label">`)
	b.WriteString(escapeField(lsjSenseSummaryLabel))
	b.WriteString(`</div>`)
	for _, line := range validated.Lines {
		b.WriteString(`<div class="lsj-summary-line">`)
		b.WriteString(escapeField(line.Text))
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// renderAncientGreekLSJEntry renders the LSJ entry in source order with LSJ's
// numbering. When the entry carries definitions, a compact outline of each
// sense's definitions comes first and the full hierarchy, every gloss and
// Xenophon citation, follows behind a collapsed toggle; otherwise the full
// hierarchy is shown open. It never reranks a sense, and the full entry is
// never truncated. Every text piece is escaped before it is wrapped, so source
// text cannot inject markup.
func renderAncientGreekLSJEntry(entry enrichment.AncientGreekLSJEntry) string {
	return renderLSJEntry(sourceevidence.BuildLSJ(entry, nil))
}

// renderLSJEntry renders a resolved LSJ entry; see renderAncientGreekLSJEntry.
func renderLSJEntry(resolved sourceevidence.Entry) string {
	entry := resolved.LSJ
	if entry.Empty() {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<div class="dictionary-entry lsj-entry">`)
	b.WriteString(`<div class="dictionary-entry-source">LSJ`)
	if headword := strings.TrimSpace(entry.Headword); headword != "" {
		b.WriteString(` <span class="lsj-headword">`)
		// The index stores the folded headword; learners see word-final ς.
		b.WriteString(escapeField(canonicalization.AncientGreekDisplayForm(headword)))
		b.WriteString(`</span>`)
	}
	b.WriteString(`</div>`)
	if outline := resolved.Outline; len(outline) > 0 {
		renderLSJOutline(&b, outline)
		b.WriteString(`<details class="lsj-full"><summary>Full LSJ entry</summary>`)
		renderAncientGreekLSJSenses(&b, entry.Senses, 1)
		b.WriteString(`</details>`)
	} else {
		renderAncientGreekLSJSenses(&b, entry.Senses, 1)
	}
	b.WriteString(`</div>`)
	return b.String()
}

// renderLSJOutline writes the compact outline: each kept sense's number and
// definitions, without usage glosses or citations.
func renderLSJOutline(b *strings.Builder, outline []sourceevidence.OutlineSense) {
	b.WriteString(`<div class="lsj-outline">`)
	for _, item := range outline {
		b.WriteString(`<div class="lsj-sense lsj-level-`)
		b.WriteString(strconv.Itoa(item.Level))
		b.WriteString(`">`)
		writeLSJNumber(b, item.Number)
		writeLSJGlosses(b, item.Definitions)
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)
}

// renderAncientGreekLSJSenses writes one nesting level of senses. A sense with
// no explicit level inherits its depth so the hierarchy never collapses.
func renderAncientGreekLSJSenses(b *strings.Builder, senses []enrichment.AncientGreekLSJSense, depth int) {
	for _, sense := range senses {
		level := sense.Level
		if level <= 0 {
			level = depth
		}
		b.WriteString(`<div class="lsj-sense lsj-level-`)
		b.WriteString(strconv.Itoa(level))
		b.WriteString(`">`)
		writeLSJNumber(b, sense.Number)
		writeLSJGlosses(b, sense.Glosses)
		writeLSJCitations(b, sense.Citations)
		renderAncientGreekLSJSenses(b, sense.Senses, level+1)
		b.WriteString(`</div>`)
	}
}

// writeLSJNumber writes one sense's LSJ numbering label when it has one.
func writeLSJNumber(b *strings.Builder, number string) {
	number = strings.TrimSpace(number)
	if number == "" {
		return
	}
	b.WriteString(`<span class="lsj-number">`)
	b.WriteString(escapeField(number))
	b.WriteString(`</span> `)
}

// writeLSJGlosses writes a sense's English glosses, in source order, separated
// by a semicolon. An empty gloss is skipped rather than left as a gap.
func writeLSJGlosses(b *strings.Builder, glosses []string) {
	written := 0
	for _, gloss := range glosses {
		gloss = strings.TrimSpace(gloss)
		if gloss == "" {
			continue
		}
		if written > 0 {
			b.WriteString(`; `)
		}
		b.WriteString(`<span class="lsj-gloss">`)
		b.WriteString(escapeField(gloss))
		b.WriteString(`</span>`)
		written++
	}
}

// writeLSJCitations writes a sense's kept citations in source order.
func writeLSJCitations(b *strings.Builder, citations []string) {
	for _, citation := range citations {
		citation = strings.TrimSpace(citation)
		if citation == "" {
			continue
		}
		b.WriteString(` <span class="lsj-citation">`)
		b.WriteString(escapeField(citation))
		b.WriteString(`</span>`)
	}
}
