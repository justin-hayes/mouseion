package cardexport

import (
	"context"
	"maps"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/justin-hayes/mouseion/internal/cardpolicy"
	"github.com/justin-hayes/mouseion/internal/enrichment"
)

// DictionaryEntryIDSeparator joins the native entry IDs a frozen item records
// when its dictionary shows several entries (Lewis & Short homographs). A
// native entry ID never contains it, so the joined value splits back exactly.
const DictionaryEntryIDSeparator = ","

// resolveLatinLS freezes every Lewis & Short entry the lookup returns for one
// Latin card, in entry id order, with no cap and no reordering (ADR 0133). It
// freezes nothing for another language, a provider without the Lewis & Short
// capability, or a lemma with no entry: such a card omits the dictionary block
// with no placeholder. The frozen item records every entry ID, joined, and the
// one pin they share; the rendered block is frozen with them, so a re-render
// shows the same entries after the index changes. workAuthor is the card Work's
// author and workTextgroup its CTS textgroup, which the full entry's
// citations are filtered against.
func resolveLatinLS(ctx context.Context, lexical enrichment.LexicalProvider, entry *Entry, workAuthor, workTextgroup string) bool {
	policy := cardpolicy.For("")
	if entry != nil {
		policy = cardpolicy.For(entry.Language)
	}
	if entry == nil || policy.Entries != cardpolicy.EntryLewisShort || lexical == nil {
		return false
	}
	provider, ok := lexical.(enrichment.LatinLSProvider)
	if !ok {
		return false
	}
	found, ok := provider.LookupLatinLS(ctx, entry.CanonicalLemma)
	if !ok {
		return false
	}
	entries := make([]enrichment.LatinLSEntry, 0, len(found))
	ids := make([]string, 0, len(found))
	targets := map[string]enrichment.LatinLSEntry{}
	for _, candidate := range found {
		if candidate.Empty() {
			continue
		}
		entries = append(entries, candidate)
		ids = append(ids, candidate.EntryID)
		maps.Copy(targets, resolveLatinLSTargets(ctx, provider, candidate))
	}
	if len(entries) == 0 {
		return false
	}
	entry.DictionaryEntryID = strings.Join(ids, DictionaryEntryIDSeparator)
	entry.DictionaryPin = entries[0].Pin
	entry.DictionaryEntry = renderLatinLSEntries(entries, targets, policy.Citations, workAuthor, workTextgroup)
	return true
}

// resolveLatinLSTargets returns the entries a senseless entry's resolved
// cross-references name ("v. X"), by entry ID, so the card can show the target's
// meaning in the entry's place (ADR 0132). An entry with a sense of its own, or
// a reference that does not resolve to exactly one entry, contributes nothing.
func resolveLatinLSTargets(ctx context.Context, provider enrichment.LatinLSProvider, entry enrichment.LatinLSEntry) map[string]enrichment.LatinLSEntry {
	if hasLatinLSSense(entry.Senses) {
		return nil
	}
	targets := map[string]enrichment.LatinLSEntry{}
	for _, sense := range entry.Senses {
		for _, ref := range sense.CrossRefs {
			if ref.EntryID == "" || ref.EntryID == entry.EntryID {
				continue
			}
			if target, ok := lookupLatinLSEntry(ctx, provider, ref); ok {
				targets[ref.EntryID] = target
			}
		}
	}
	return targets
}

// lookupLatinLSEntry finds the entry a resolved cross-reference names among the
// entries under its target headword.
func lookupLatinLSEntry(ctx context.Context, provider enrichment.LatinLSProvider, ref enrichment.LatinLSCrossRef) (enrichment.LatinLSEntry, bool) {
	candidates, ok := provider.LookupLatinLS(ctx, ref.Headword)
	if !ok {
		return enrichment.LatinLSEntry{}, false
	}
	for _, candidate := range candidates {
		if candidate.EntryID == ref.EntryID {
			return candidate, true
		}
	}
	return enrichment.LatinLSEntry{}, false
}

// hasLatinLSSense reports whether the entry has a sense of its own: any sense
// below the level-0 pseudo-sense that carries only its citations and redirects.
func hasLatinLSSense(senses []enrichment.LatinLSSense) bool {
	for _, sense := range senses {
		if sense.Level > 0 {
			return true
		}
	}
	return false
}

// renderLatinLSEntries stacks every entry, each with its own head line, outline
// and collapsed full entry. With several entries each head line carries the
// entry's L&S number, which is the source's own homograph number (dīco¹,
// dīco²), or its place in id order when the source key carries none; the head
// line shows the entry's own <orth>, with its vowel-length marks, in place of
// the bare headword.
func renderLatinLSEntries(entries []enrichment.LatinLSEntry, targets map[string]enrichment.LatinLSEntry, filter cardpolicy.CitationFilter, workAuthor, workTextgroup string) string {
	var b strings.Builder
	for i, entry := range entries {
		number := 0
		if len(entries) > 1 {
			number = i + 1
			if entry.Homograph > 0 {
				number = entry.Homograph
			}
		}
		renderLatinLSEntry(&b, entry, targets, number, filter, workAuthor, workTextgroup)
	}
	return b.String()
}

func renderLatinLSEntry(b *strings.Builder, entry enrichment.LatinLSEntry, targets map[string]enrichment.LatinLSEntry, number int, filter cardpolicy.CitationFilter, workAuthor, workTextgroup string) {
	b.WriteString(`<div class="dictionary-entry ls-entry">`)
	b.WriteString(`<div class="dictionary-entry-source">Lewis &amp; Short`)
	headword := strings.TrimSpace(entry.Orth)
	if headword == "" {
		headword = strings.TrimSpace(entry.Headword)
	}
	if headword != "" || number > 0 {
		b.WriteString(` <span class="dict-headword">`)
		b.WriteString(escapeField(headword))
		if number > 0 {
			b.WriteString(`<sup>`)
			b.WriteString(strconv.Itoa(number))
			b.WriteString(`</sup>`)
		}
		b.WriteString(`</span>`)
	}
	b.WriteString(`</div>`)
	if hasLatinLSSense(entry.Senses) {
		writeLatinLSOutline(b, entry.Senses)
		b.WriteString(`<details class="dict-full"><summary>Full Lewis &amp; Short entry</summary>`)
		renderLatinLSSenses(b, entry.Senses, filter, workAuthor, workTextgroup)
		b.WriteString(`</details>`)
	} else {
		// An entry with no sense of its own shows its head line and, when its
		// cross-reference resolved to one entry, that entry's meaning. An
		// unresolved one shows the head line alone.
		writeLatinLSTargetOutlines(b, entry, targets)
	}
	b.WriteString(`</div>`)
}

// writeLatinLSOutline writes an entry's compact outline, if it has one.
func writeLatinLSOutline(b *strings.Builder, senses []enrichment.LatinLSSense) {
	outline := latinLSOutline(senses)
	if len(outline) == 0 {
		return
	}
	b.WriteString(`<div class="dict-outline">`)
	for _, item := range outline {
		b.WriteString(`<div class="dict-sense dict-level-`)
		b.WriteString(strconv.Itoa(item.level))
		b.WriteString(`">`)
		writeDictNumber(b, item.sense.Number)
		writeDictGlosses(b, item.sense.Definitions)
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)
}

// writeLatinLSTargetOutlines writes the outline of each entry a senseless
// entry's resolved cross-references name, in source order, each led by the
// reference as the source writes it.
func writeLatinLSTargetOutlines(b *strings.Builder, entry enrichment.LatinLSEntry, targets map[string]enrichment.LatinLSEntry) {
	seen := map[string]struct{}{}
	for _, sense := range entry.Senses {
		for _, ref := range sense.CrossRefs {
			target, ok := targets[ref.EntryID]
			if _, done := seen[ref.EntryID]; !ok || done || len(latinLSOutline(target.Senses)) == 0 {
				continue
			}
			seen[ref.EntryID] = struct{}{}
			b.WriteString(`<div class="dict-crossref-target"><span class="dict-crossref">`)
			b.WriteString(escapeField(strings.TrimSpace(ref.Text)))
			b.WriteString(`</span>`)
			writeLatinLSOutline(b, target.Senses)
			b.WriteString(`</div>`)
		}
	}
}

type latinLSOutlineSense struct {
	sense enrichment.LatinLSSense
	level int
}

// latinLSOutline keeps each sense that has definitions, plus each heading whose
// span holds one (ADR 0104, per entry). Senses are flat and in id order; a
// sense's span is the run of deeper senses after it. The level-0 pseudo-sense
// that carries an entry's own citations and redirects is never outlined.
func latinLSOutline(senses []enrichment.LatinLSSense) []latinLSOutlineSense {
	var flat []latinLSOutlineSense
	for _, sense := range senses {
		if sense.Level <= 0 {
			continue
		}
		flat = append(flat, latinLSOutlineSense{sense: sense, level: sense.Level})
	}
	var outline []latinLSOutlineSense
	for i, item := range flat {
		if len(item.sense.Definitions) > 0 {
			outline = append(outline, item)
			continue
		}
		for j := i + 1; j < len(flat) && flat[j].level > item.level; j++ {
			if len(flat[j].sense.Definitions) > 0 {
				outline = append(outline, item)
				break
			}
		}
	}
	return outline
}

// renderLatinLSSenses writes the full entry in source order. A sense that has
// nothing left to show after the citation filter is skipped.
func renderLatinLSSenses(b *strings.Builder, senses []enrichment.LatinLSSense, filter cardpolicy.CitationFilter, workAuthor, workTextgroup string) {
	for _, sense := range senses {
		citations := filterLatinLSCitations(sense.Citations, filter, workAuthor, workTextgroup)
		glosses := nonBlank(sense.Glosses)
		number := strings.TrimSpace(sense.Number)
		if len(glosses) == 0 && len(citations) == 0 && len(sense.CrossRefs) == 0 && (number == "" || sense.Level <= 0) {
			continue
		}
		level := max(sense.Level, 1)
		b.WriteString(`<div class="dict-sense dict-level-`)
		b.WriteString(strconv.Itoa(level))
		b.WriteString(`">`)
		writeDictNumber(b, number)
		writeDictGlosses(b, glosses)
		for _, citation := range citations {
			b.WriteString(` <span class="dict-citation">`)
			b.WriteString(escapeField(citation))
			b.WriteString(`</span>`)
		}
		for _, ref := range sense.CrossRefs {
			if text := strings.TrimSpace(ref.Text); text != "" {
				b.WriteString(` <span class="dict-crossref">`)
				b.WriteString(escapeField(text))
				b.WriteString(`</span>`)
			}
		}
		b.WriteString(`</div>`)
	}
}

func nonBlank(values []string) []string {
	var kept []string
	for _, value := range values {
		if value = strings.TrimSpace(value); value != "" {
			kept = append(kept, value)
		}
	}
	return kept
}

func writeDictNumber(b *strings.Builder, number string) {
	if number = strings.TrimSpace(number); number != "" {
		b.WriteString(`<span class="dict-number">`)
		b.WriteString(escapeField(number))
		b.WriteString(`</span> `)
	}
}

func writeDictGlosses(b *strings.Builder, glosses []string) {
	for i, gloss := range nonBlank(glosses) {
		if i > 0 {
			b.WriteString(`; `)
		}
		b.WriteString(`<span class="dict-gloss">`)
		b.WriteString(escapeField(gloss))
		b.WriteString(`</span>`)
	}
}

// filterLatinLSCitations keeps a sense's citations in source order. Under the
// Work-author filter a citation is kept only when its author reference names
// the card's Work's author; one with no usable author reference is dropped,
// never guessed (ADR 0133).
func filterLatinLSCitations(citations []enrichment.LatinLSCitation, filter cardpolicy.CitationFilter, workAuthor, workTextgroup string) []string {
	var kept []string
	for _, citation := range citations {
		text := strings.TrimSpace(citation.Text)
		if text == "" {
			continue
		}
		if filter == cardpolicy.CitationsWorkAuthor && !latinAuthorMatches(citation.Author, workAuthor, workTextgroup) {
			continue
		}
		kept = append(kept, text)
	}
	return kept
}

// latinAuthorMatches reports whether an L&S citation's author reference names
// the Work's author. The reference is either the dictionary's abbreviation
// ("Liv.", "Cic.") or, when the source gave none, the CTS URN author ("phi0474").
// It matches when it equals the Work author as written, or when the
// abbreviation (at least three letters) begins one word of the Work author, so
// "Liv." names Livy and "Cic." names Marcus Tullius Cicero.
func latinAuthorMatches(reference, workAuthor, workTextgroup string) bool {
	reference = authorKey(reference)
	if textgroup := authorKey(workTextgroup); textgroup != "" && reference == textgroup {
		return true
	}
	workAuthor = authorKey(workAuthor)
	if reference == "" || workAuthor == "" {
		return false
	}
	if reference == workAuthor {
		return true
	}
	if len([]rune(reference)) < 3 {
		return false
	}
	for word := range strings.FieldsSeq(workAuthor) {
		if strings.HasPrefix(word, reference) {
			return true
		}
	}
	return false
}

// authorKey lowercases an author name, drops accents and punctuation, and
// keeps word breaks.
func authorKey(value string) string {
	var b strings.Builder
	for _, r := range norm.NFD.String(strings.ToLower(value)) {
		switch {
		case unicode.IsLetter(r) || unicode.IsDigit(r):
			b.WriteRune(r)
		case unicode.IsSpace(r) || r == '-':
			b.WriteRune(' ')
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
