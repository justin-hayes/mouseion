package cardexport

import (
	"context"
	"strconv"
	"strings"

	"github.com/justin-hayes/mouseion/internal/enrichment"
)

// resolveAncientGreekLSJ freezes the matched LSJ dictionary entry for one
// Ancient Greek card. It resolves nothing for another language, a provider
// without the LSJ capability, or an identity whose headword is ambiguous or
// missing: such a card shows no entry rather than a guess. The lexical
// discriminator is never passed through, because an OGA morpheus:N number is not
// LSJ's numbering.
func resolveAncientGreekLSJ(ctx context.Context, lexical enrichment.LexicalProvider, entry *Entry) {
	if entry == nil || !IsAncientGreek(entry.Language) || lexical == nil {
		return
	}
	provider, ok := lexical.(enrichment.AncientGreekLSJProvider)
	if !ok {
		return
	}
	matched, found := provider.LookupAncientGreekLSJ(ctx, entry.CanonicalLemma)
	if !found || matched.Empty() {
		return
	}
	entry.DictionaryEntryID = matched.EntryID
	entry.DictionaryPin = matched.Pin
	entry.DictionaryEntry = renderAncientGreekLSJEntry(matched)
}

// renderAncientGreekLSJEntry renders the full LSJ sense hierarchy in source
// order with LSJ's numbering and each sense's English glosses and Xenophon
// citations. It never reranks or truncates a sense. Every text piece is escaped
// before it is wrapped, so source text cannot inject markup.
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
	renderAncientGreekLSJSenses(&b, entry.Senses, 1)
	b.WriteString(`</div>`)
	return b.String()
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
