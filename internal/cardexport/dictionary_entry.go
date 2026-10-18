package cardexport

import (
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/justin-hayes/mouseion/internal/enrichment"
)

// resolveAncientGreekLSJ freezes the matched LSJ dictionary entry for one
// Ancient Greek card. It resolves nothing for another language, a provider
// without the LSJ capability, or an identity whose headword is ambiguous or
// missing: such a card shows no entry rather than a guess. The lexical
// discriminator is never passed through, because an OGA morpheus:N number is not
// LSJ's numbering. It returns the matched entry so a caller can freeze it as
// pass evidence.
func resolveAncientGreekLSJ(ctx context.Context, lexical enrichment.LexicalProvider, entry *Entry) (enrichment.AncientGreekLSJEntry, bool) {
	if entry == nil || !IsAncientGreek(entry.Language) || lexical == nil {
		return enrichment.AncientGreekLSJEntry{}, false
	}
	provider, ok := lexical.(enrichment.AncientGreekLSJProvider)
	if !ok {
		return enrichment.AncientGreekLSJEntry{}, false
	}
	matched, found := provider.LookupAncientGreekLSJ(ctx, entry.CanonicalLemma)
	if !found || matched.Empty() {
		return enrichment.AncientGreekLSJEntry{}, false
	}
	entry.DictionaryEntryID = matched.EntryID
	entry.DictionaryPin = matched.Pin
	entry.DictionaryEntry = renderAncientGreekLSJEntry(matched)
	return matched, true
}

// ancientGreekLSJPassSenses flattens a matched LSJ entry's sense hierarchy, in
// source order, into the pinned LSJ meaning evidence the evidence-assisted pass
// transmits. Each sense carries its native LSJ sense ID as its evidence ID, so
// the provider cites a pinned sense and decoding maps the short handle back to
// that exact ID. Each sense sends its definitions (ADR 0104), not the usage
// glosses that translate quoted passages; an entry derived before definitions
// existed sends every gloss instead. A sense with no native ID cannot be cited,
// and a sense with nothing to send gives no evidence, so both are skipped.
func ancientGreekLSJPassSenses(entry enrichment.AncientGreekLSJEntry) []enrichment.LexicalSense {
	useDefinitions := lsjHasDefinitions(entry.Senses)
	var senses []enrichment.LexicalSense
	var walk func([]enrichment.AncientGreekLSJSense)
	walk = func(level []enrichment.AncientGreekLSJSense) {
		for _, sense := range level {
			glosses := sense.Glosses
			if useDefinitions {
				glosses = sense.Definitions
			}
			gloss := strings.Join(glosses, "; ")
			if strings.TrimSpace(sense.ID) != "" && strings.TrimSpace(gloss) != "" {
				senses = append(senses, enrichment.LexicalSense{EvidenceID: sense.ID, Source: "lsj", Gloss: gloss})
			}
			walk(sense.Senses)
		}
	}
	walk(entry.Senses)
	return senses
}

// lsjHasDefinitions reports whether any sense, at any depth, has definitions.
func lsjHasDefinitions(senses []enrichment.AncientGreekLSJSense) bool {
	return slices.ContainsFunc(senses, func(sense enrichment.AncientGreekLSJSense) bool {
		return len(sense.Definitions) > 0 || lsjHasDefinitions(sense.Senses)
	})
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
	if entry.Empty() {
		return ""
	}
	var b strings.Builder
	b.WriteString(`<div class="dictionary-entry lsj-entry">`)
	b.WriteString(`<div class="dictionary-entry-source">LSJ`)
	if headword := strings.TrimSpace(entry.Headword); headword != "" {
		b.WriteString(` <span class="lsj-headword">`)
		b.WriteString(escapeField(headword))
		b.WriteString(`</span>`)
	}
	b.WriteString(`</div>`)
	if outline := lsjOutline(entry.Senses); len(outline) > 0 {
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

// lsjOutlineSense is one sense in the compact outline, at its effective level.
type lsjOutlineSense struct {
	sense enrichment.AncientGreekLSJSense
	level int
}

// lsjOutline flattens the senses in source order and keeps each sense that has
// definitions, plus each heading whose span holds one. Perseus LSJ senses are
// mostly flat siblings that carry their level, so a sense's span is the run of
// deeper-level senses after it, nested or not.
func lsjOutline(senses []enrichment.AncientGreekLSJSense) []lsjOutlineSense {
	var flat []lsjOutlineSense
	var walk func([]enrichment.AncientGreekLSJSense, int)
	walk = func(level []enrichment.AncientGreekLSJSense, depth int) {
		for _, sense := range level {
			effective := sense.Level
			if effective <= 0 {
				effective = depth
			}
			flat = append(flat, lsjOutlineSense{sense: sense, level: effective})
			walk(sense.Senses, effective+1)
		}
	}
	walk(senses, 1)
	var outline []lsjOutlineSense
	for i, item := range flat {
		if lsjSpanHasDefinitions(flat, i) {
			outline = append(outline, item)
		}
	}
	return outline
}

// lsjSpanHasDefinitions reports whether flat[i] or any deeper sense in its span
// has a definition.
func lsjSpanHasDefinitions(flat []lsjOutlineSense, i int) bool {
	if len(flat[i].sense.Definitions) > 0 {
		return true
	}
	for j := i + 1; j < len(flat) && flat[j].level > flat[i].level; j++ {
		if len(flat[j].sense.Definitions) > 0 {
			return true
		}
	}
	return false
}

// renderLSJOutline writes the compact outline: each kept sense's number and
// definitions, without usage glosses or citations.
func renderLSJOutline(b *strings.Builder, outline []lsjOutlineSense) {
	b.WriteString(`<div class="lsj-outline">`)
	for _, item := range outline {
		b.WriteString(`<div class="lsj-sense lsj-level-`)
		b.WriteString(strconv.Itoa(item.level))
		b.WriteString(`">`)
		writeLSJNumber(b, item.sense.Number)
		writeLSJGlosses(b, item.sense.Definitions)
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
