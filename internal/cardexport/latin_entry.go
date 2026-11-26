package cardexport

import (
	"context"
	"strconv"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/justin-hayes/mouseion/internal/cardpolicy"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/sourceevidence"
)

// DictionaryEntryIDSeparator joins the native entry IDs a frozen item records
// when its dictionary shows several entries (Lewis & Short homographs). A
// native entry ID never contains it, so the joined value splits back exactly.
const DictionaryEntryIDSeparator = sourceevidence.EntryIDSeparator

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
	_, found := resolveLatinLSEntries(ctx, lexical, entry, workAuthor, workTextgroup)
	return found
}

// resolveLatinLSEntries is resolveLatinLS returning the resolved entries too, so
// the evidence-assisted pass can send their senses as its meaning evidence.
func resolveLatinLSEntries(ctx context.Context, lexical enrichment.LexicalProvider, entry *Entry, workAuthor, workTextgroup string) ([]sourceevidence.Entry, bool) {
	policy := cardpolicy.For("")
	if entry != nil {
		policy = cardpolicy.For(entry.Language)
	}
	if entry == nil || policy.Entries != cardpolicy.EntryLewisShort {
		return nil, false
	}
	resolution, found := sourceevidence.Resolve(ctx, lexical, policy, entry.CanonicalLemma, nil)
	if !found {
		return nil, false
	}
	entry.DictionaryEntryID = resolution.EntryID
	entry.DictionaryPin = resolution.Pin
	entry.DictionaryEntry = renderLatinLSResolved(resolution.Entries, policy.Citations, workAuthor, workTextgroup)
	return resolution.Entries, true
}

// renderLatinLSEntries stacks every entry, each with its own head line, outline
// and collapsed full entry. With several entries each head line carries the
// entry's L&S number, which is the source's own homograph number (dīco¹,
// dīco²), or its place in id order when the source key carries none; the head
// line shows the entry's own <orth>, with its vowel-length marks, in place of
// the bare headword.
//
//nolint:unparam // targets is the cross-reference map a caller of the raw-entry form may supply; production renders resolved entries.
func renderLatinLSEntries(entries []enrichment.LatinLSEntry, targets map[string]enrichment.LatinLSEntry, filter cardpolicy.CitationFilter, workAuthor, workTextgroup string) string {
	return renderLatinLSResolved(sourceevidence.BuildLewisShort(entries, targets, nil), filter, workAuthor, workTextgroup)
}

// renderLatinLSResolved stacks resolved entries; see renderLatinLSEntries.
func renderLatinLSResolved(entries []sourceevidence.Entry, filter cardpolicy.CitationFilter, workAuthor, workTextgroup string) string {
	var b strings.Builder
	for _, resolved := range entries {
		renderLatinLSEntry(&b, resolved, filter, workAuthor, workTextgroup)
	}
	return b.String()
}

func renderLatinLSEntry(b *strings.Builder, resolved sourceevidence.Entry, filter cardpolicy.CitationFilter, workAuthor, workTextgroup string) {
	entry, number := resolved.LewisShort, resolved.Number
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
	if sourceevidence.HasOwnSense(entry.Senses) {
		writeLatinLSOutline(b, resolved.Outline)
		b.WriteString(`<details class="dict-full"><summary>Full Lewis &amp; Short entry</summary>`)
		renderLatinLSSenses(b, entry.Senses, filter, workAuthor, workTextgroup)
		b.WriteString(`</details>`)
	} else {
		// An entry with no sense of its own shows its head line and, when its
		// cross-reference resolved to one entry, that entry's meaning. An
		// unresolved one shows the head line alone.
		writeLatinLSTargetOutlines(b, resolved.Targets)
	}
	b.WriteString(`</div>`)
}

// writeLatinLSOutline writes an entry's compact outline, if it has one.
func writeLatinLSOutline(b *strings.Builder, outline []sourceevidence.OutlineSense) {
	if len(outline) == 0 {
		return
	}
	b.WriteString(`<div class="dict-outline">`)
	for _, item := range outline {
		b.WriteString(`<div class="dict-sense dict-level-`)
		b.WriteString(strconv.Itoa(item.Level))
		b.WriteString(`">`)
		writeDictNumber(b, item.Number)
		writeDictGlosses(b, item.Definitions)
		b.WriteString(`</div>`)
	}
	b.WriteString(`</div>`)
}

// writeLatinLSTargetOutlines writes the outline of each entry a senseless
// entry's resolved cross-references name, in source order, each led by the
// reference as the source writes it.
func writeLatinLSTargetOutlines(b *strings.Builder, targets []sourceevidence.Target) {
	for _, target := range targets {
		b.WriteString(`<div class="dict-crossref-target"><span class="dict-crossref">`)
		b.WriteString(escapeField(target.Reference))
		b.WriteString(`</span>`)
		writeLatinLSOutline(b, target.Outline)
		b.WriteString(`</div>`)
	}
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
