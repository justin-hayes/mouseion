package webapp

import (
	"cmp"
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/justin-hayes/mouseion/internal/constructions"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// sentenceStudyConstructionEntry is one Construction occurrence as Sentence
// study states it: the concept note's wording, the extract, and how the
// identified target relates to it. Everything but the stored identifiers and
// ordinals is looked up when the page renders (ADR 0124).
type sentenceStudyConstructionEntry struct {
	// Number is the entry's position in the list, counted from 1.
	Number int
	// Title is the note name, with the subtype label in parentheses when the
	// shipping gate labels the subtype.
	Title string
	// ID is the construction id. SubtypeID and SubtypeLabel are set only when
	// the shipping gate labels the subtype; a Parallels link carries no other.
	ID, SubtypeID, SubtypeLabel string
	// Focused marks the entry a learner arrived at from Concordance's Study.
	Focused bool
	// Extract is the whole sentence reprinted in runs.
	Extract []sentenceStudyExtractPart
	// Words lists the extent in textual order, "…" between discontinuous parts.
	Words string
	// InsideOf is the Number of the smallest earlier entry whose extent strictly
	// contains this one, or zero.
	InsideOf      int
	AnchorSurface string
	// TargetPhrase relates the identified target to the entry, or is empty.
	TargetPhrase string
	Definition   string
	CitationText string
	CitationURL  string

	name   string
	anchor int64
	extent map[int64]bool
}

// sentenceStudyExtractPart is one run of an extract: text with the same weight.
// An extent run is at full weight, the rest is muted, and the anchor is
// underlined.
type sentenceStudyExtractPart struct {
	Text     string
	InExtent bool
	Anchor   bool
}

// sentenceStudyConstructionEntries builds the Constructions list of a
// native_agdt run: nothing when the run has no stored occurrences, when the
// sentence has no analyzed words, or for any other syntax state. An occurrence
// whose construction has no note, or whose anchor is not an analyzed word of
// the sentence, is left out rather than guessed.
func sentenceStudyConstructionEntries(study domain.SentenceStudy) []sentenceStudyConstructionEntry {
	if study.SyntaxState != "native_agdt" || len(study.Tokens) == 0 || len(study.Constructions) == 0 {
		return nil
	}
	surfaces := make(map[int64]string, len(study.Tokens))
	for _, token := range study.Tokens {
		surfaces[token.Ordinal] = token.Surface
	}
	var entries []sentenceStudyConstructionEntry
	for _, occurrence := range study.Constructions {
		note, ok := constructions.Notes[constructions.ID(occurrence.ConstructionID)]
		surface, anchored := surfaces[int64(occurrence.Anchor)]
		if !ok || !anchored {
			continue
		}
		entry := sentenceStudyConstructionEntry{
			Title: note.Name, ID: occurrence.ConstructionID, name: note.Name, AnchorSurface: surface, anchor: int64(occurrence.Anchor),
			Definition:   note.Definition,
			CitationText: constructions.CitationText(note.Citation), CitationURL: constructions.CitationURL(note.Citation),
			extent: make(map[int64]bool, len(occurrence.Extent)),
		}
		if label, ok := constructions.SubtypeLabel(constructions.ID(occurrence.ConstructionID), constructions.SubtypeID(occurrence.SubtypeID)); ok {
			entry.Title += " (" + label + ")"
			entry.SubtypeID, entry.SubtypeLabel = occurrence.SubtypeID, label
		}
		for _, ordinal := range occurrence.Extent {
			entry.extent[int64(ordinal)] = true
		}
		entries = append(entries, entry)
	}
	// First extent ordinal, then the outer (larger) extent, then anchor, then
	// name, so the order is deterministic whatever the store returned.
	slices.SortStableFunc(entries, func(a, b sentenceStudyConstructionEntry) int {
		return cmp.Or(
			cmp.Compare(a.first(), b.first()),
			cmp.Compare(len(b.extent), len(a.extent)),
			cmp.Compare(a.anchor, b.anchor),
			cmp.Compare(a.name, b.name),
		)
	})
	items := sentenceStudyExtractItems(study)
	for i := range entries {
		entry := &entries[i]
		entry.Number = i + 1
		entry.InsideOf = sentenceStudyInsideOf(entries, i)
		entry.TargetPhrase = sentenceStudyTargetPhrase(study.TargetOrdinal, *entry)
		entry.Extract = sentenceStudyExtract(items, *entry)
		entry.Words = sentenceStudyConstructionWordsLine(study, *entry)
	}
	return entries
}

// sentenceStudyFocusedConstructionEntries is the entry list with the one entry
// a learner arrived at from Concordance's Study marked: the construction named
// by the request whose anchor is the identified target. The list order never
// changes, and an unmatched focus marks nothing.
func sentenceStudyFocusedConstructionEntries(study domain.SentenceStudy, focusConstruction string) []sentenceStudyConstructionEntry {
	entries := sentenceStudyConstructionEntries(study)
	if focusConstruction == "" {
		return entries
	}
	for i := range entries {
		if entries[i].ID == focusConstruction && entries[i].anchor == study.TargetOrdinal {
			entries[i].Focused = true
			break
		}
	}
	return entries
}

// anchorID is the entry's fragment identifier. It is derived from the
// construction and its anchor, so Concordance's Study link can address it
// without knowing the entry's position.
func (e sentenceStudyConstructionEntry) anchorID() string {
	return fmt.Sprintf("construction-%s-%d", e.ID, e.anchor)
}

// parallelsLabel is the Parallels link's accessible name.
func (e sentenceStudyConstructionEntry) parallelsLabel() string {
	if e.SubtypeLabel != "" {
		return "Parallels for " + e.name + ", " + e.SubtypeLabel
	}
	return "Parallels for " + e.name
}

// parallelsURL opens Concordance with this occurrence's parallels: other
// occurrences of the construction, filtered to the subtype only when it is
// labelled, with this Book as the origin Book and this occurrence as the origin.
func (e sentenceStudyConstructionEntry) parallelsURL(study domain.SentenceStudy) string {
	return vocabularyConcordancePageURL(1, domain.ConcordanceLookup{
		Language: domain.ConcordanceConstructionLanguage, Term: e.ID, Match: domain.ConcordanceMatchConstruction, Subtype: e.SubtypeID, Priority: study.BookID,
		OriginLemma: e.anchorLemma(study),
		Origin:      &domain.ConcordanceConstructionOrigin{RunID: study.AnalysisRunID, Sentence: int(study.SentenceOrdinal), Anchor: int(e.anchor)},
	})
}

// anchorLemma is the anchor word's effective lemma, so Parallels can offer
// "Only with {lemma}". It is empty when the anchor is not an analyzed word.
func (e sentenceStudyConstructionEntry) anchorLemma(study domain.SentenceStudy) string {
	for _, token := range study.Tokens {
		if token.Ordinal == e.anchor {
			return token.EffectiveLemma
		}
	}
	return ""
}

// targetSuffix is the target phrase set after "Signalled by …." with its
// separating space, or empty when the target has no relation to the entry.
func (e sentenceStudyConstructionEntry) targetSuffix() string {
	if e.TargetPhrase == "" {
		return ""
	}
	return " " + e.TargetPhrase
}

// first is the entry's smallest extent ordinal.
func (e sentenceStudyConstructionEntry) first() int64 {
	smallest := int64(-1)
	for ordinal := range e.extent {
		if smallest < 0 || ordinal < smallest {
			smallest = ordinal
		}
	}
	return smallest
}

// sentenceStudyInsideOf returns the Number of the smallest earlier entry whose
// extent strictly contains entries[at]'s, preferring the earliest on a tie.
func sentenceStudyInsideOf(entries []sentenceStudyConstructionEntry, at int) int {
	inner := entries[at]
	best, bestSize := 0, 0
	for _, outer := range entries[:at] {
		if len(outer.extent) <= len(inner.extent) {
			continue
		}
		contained := true
		for ordinal := range inner.extent {
			if !outer.extent[ordinal] {
				contained = false
				break
			}
		}
		if contained && (best == 0 || len(outer.extent) < bestSize) {
			best, bestSize = outer.Number, len(outer.extent)
		}
	}
	return best
}

func sentenceStudyTargetPhrase(target int64, entry sentenceStudyConstructionEntry) string {
	switch {
	case entry.anchor == target:
		return "The identified target signals this construction."
	case entry.extent[target]:
		return "Contains the identified target."
	default:
		return ""
	}
}

// sentenceStudyExtractItem is one piece of the reprinted sentence: an analyzed
// word (Ordinal set) or the source text between words (Ordinal -1).
type sentenceStudyExtractItem struct {
	Text    string
	Ordinal int64
}

const sentenceStudyGapOrdinal = int64(-1)

// sentenceStudyExtractItems reprints the sentence. When the stored sentence can
// be verified against its source slice and every word aligns, the exact source
// text is used, gaps included. Otherwise the stored surfaces are reprinted,
// separated by spaces except before punctuation, so an extract never fails the
// page.
func sentenceStudyExtractItems(study domain.SentenceStudy) []sentenceStudyExtractItem {
	source := sentenceStudySource{runes: []rune(study.UnitText), start: study.SentenceStartOffset, end: study.SentenceEndOffset}
	if source.trusts(study.SentenceText) {
		alignments := sentenceStudyAlignments(study.Tokens, source)
		if items, ok := sentenceStudyAlignedItems(study.Tokens, alignments, source); ok {
			return items
		}
	}
	items := make([]sentenceStudyExtractItem, 0, 2*len(study.Tokens))
	for i, token := range study.Tokens {
		if i > 0 && !sentenceStudyPunctuationOnly(token.Surface) {
			items = append(items, sentenceStudyExtractItem{Text: " ", Ordinal: sentenceStudyGapOrdinal})
		}
		items = append(items, sentenceStudyExtractItem{Text: token.Surface, Ordinal: token.Ordinal})
	}
	return items
}

// sentenceStudyAlignedItems interleaves every word's exact source slice with the
// source text between words. It reports false when any word could not align, so
// the caller falls back to the stored surfaces.
func sentenceStudyAlignedItems(tokens []domain.SentenceStudyToken, alignments []sentenceStudyAlignment, source sentenceStudySource) ([]sentenceStudyExtractItem, bool) {
	order := make([]int, len(tokens))
	for index := range tokens {
		if !alignments[index].safe {
			return nil, false
		}
		order[index] = index
	}
	slices.SortStableFunc(order, func(a, b int) int { return int(alignments[a].start - alignments[b].start) })
	var items []sentenceStudyExtractItem
	cursor := source.start
	for _, index := range order {
		alignment := alignments[index]
		if alignment.start < cursor {
			return nil, false
		}
		if alignment.start > cursor {
			items = append(items, sentenceStudyExtractItem{Text: source.slice(cursor, alignment.start), Ordinal: sentenceStudyGapOrdinal})
		}
		items = append(items, sentenceStudyExtractItem{Text: source.slice(alignment.start, alignment.end), Ordinal: tokens[index].Ordinal})
		cursor = alignment.end
	}
	if cursor < source.end {
		items = append(items, sentenceStudyExtractItem{Text: source.slice(cursor, source.end), Ordinal: sentenceStudyGapOrdinal})
	}
	return items, true
}

// sentenceStudyPunctuationOnly reports text made only of punctuation, symbols
// and white space: such a gap inside an extent is part of the extent.
func sentenceStudyPunctuationOnly(text string) bool {
	for _, r := range text {
		if !unicode.IsPunct(r) && !unicode.IsSymbol(r) && !unicode.IsSpace(r) {
			return false
		}
	}
	return true
}

// sentenceStudyExtract marks each item of the reprinted sentence and merges
// neighbours that carry the same mark. A discontinuous extent gets no other
// notation.
func sentenceStudyExtract(items []sentenceStudyExtractItem, entry sentenceStudyConstructionEntry) []sentenceStudyExtractPart {
	inExtent := sentenceStudyExtentMarks(items, entry)
	var parts []sentenceStudyExtractPart
	for i, item := range items {
		part := sentenceStudyExtractPart{Text: item.Text, InExtent: inExtent[i], Anchor: item.Ordinal == entry.anchor && item.Ordinal != sentenceStudyGapOrdinal}
		last := len(parts) - 1
		if last >= 0 && !part.Anchor && !parts[last].Anchor && parts[last].InExtent == part.InExtent {
			parts[last].Text += part.Text
			continue
		}
		parts = append(parts, part)
	}
	return parts
}

// sentenceStudyExtentMarks reports which items are part of the extent: its own
// words, and any run of punctuation-only items between two of its words.
func sentenceStudyExtentMarks(items []sentenceStudyExtractItem, entry sentenceStudyConstructionEntry) []bool {
	inExtent := make([]bool, len(items))
	for i, item := range items {
		inExtent[i] = item.Ordinal != sentenceStudyGapOrdinal && entry.extent[item.Ordinal]
	}
	for start := 0; start < len(items); {
		if inExtent[start] {
			start++
			continue
		}
		end := start
		for end < len(items) && !inExtent[end] {
			end++
		}
		if start > 0 && end < len(items) && sentenceStudyAllPunctuation(items[start:end]) {
			for i := start; i < end; i++ {
				inExtent[i] = true
			}
		}
		start = end
	}
	return inExtent
}

func sentenceStudyAllPunctuation(items []sentenceStudyExtractItem) bool {
	for _, item := range items {
		if !sentenceStudyPunctuationOnly(item.Text) {
			return false
		}
	}
	return true
}

// sentenceStudyConstructionWordsLine lists the extent's words in textual order,
// with "…" where the sentence continues with words outside the extent. A gap
// of punctuation alone is not a break, and punctuation is not a word.
func sentenceStudyConstructionWordsLine(study domain.SentenceStudy, entry sentenceStudyConstructionEntry) string {
	var words []string
	gap := false
	for _, token := range study.Tokens {
		switch {
		case entry.extent[token.Ordinal]:
			if sentenceStudyPunctuationOnly(token.Surface) {
				continue
			}
			if gap && len(words) > 0 {
				words = append(words, "…")
			}
			gap = false
			words = append(words, token.Surface)
		case !sentenceStudyPunctuationOnly(token.Surface):
			gap = true
		}
	}
	return "Words: " + strings.Join(words, " ")
}

// sentenceStudyConstructionWords is the complete text alternative's addition for
// each word: the entries containing it, and the ones it signals. It is keyed by
// token ordinal and lists nothing for a word in no construction.
func sentenceStudyConstructionWords(study domain.SentenceStudy) map[int64]string {
	entries := sentenceStudyConstructionEntries(study)
	if len(entries) == 0 {
		return nil
	}
	out := make(map[int64]string, len(study.Tokens))
	for _, token := range study.Tokens {
		if text := sentenceStudyConstructionWord(entries, token.Ordinal); text != "" {
			out[token.Ordinal] = text
		}
	}
	return out
}

// sentenceStudyConstructionWord words one word's constructions, then the ones
// it signals, each by its list number and note name.
func sentenceStudyConstructionWord(entries []sentenceStudyConstructionEntry, ordinal int64) string {
	var containing, signalled []string
	for _, entry := range entries {
		label := fmt.Sprintf("%d (%s)", entry.Number, entry.name)
		if entry.extent[ordinal] {
			containing = append(containing, label)
		}
		if entry.anchor == ordinal {
			signalled = append(signalled, label)
		}
	}
	var sentences []string
	if len(containing) > 0 {
		sentences = append(sentences, "Constructions: "+strings.Join(containing, ", ")+".")
	}
	if len(signalled) > 0 {
		sentences = append(sentences, "Signals: "+strings.Join(signalled, ", ")+".")
	}
	return strings.Join(sentences, " ")
}
