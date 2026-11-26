// Package sourceevidence resolves the imported-source evidence of a
// source-evidence language: its dictionary entries, their compact outlines and
// the translated passage a representative sentence sits in. It branches only on
// the card policy value, never on a language code (ADR 0133), so the Prepared
// deck and Study share one resolver (ADR 0138).
package sourceevidence

import (
	"context"
	"maps"
	"slices"
	"sort"
	"strings"

	"github.com/justin-hayes/mouseion/internal/cardpolicy"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/enrichment"
)

// EntryIDSeparator joins the native entry IDs a frozen item records when its
// dictionary shows several entries (Lewis & Short homographs). A native entry ID
// never contains it, so the joined value splits back exactly.
const EntryIDSeparator = ","

// SenseRef names one sense of one entry. A native sense ID can be shared by
// stacked homograph entries, so a sense is only ever cited together with its
// entry.
type SenseRef struct{ EntryID, SenseID string }

// OutlineSense is one sense of an entry's compact outline: its label, its
// nesting level, its definitions and whether the caller cited it.
type OutlineSense struct {
	// ID is the source's native sense ID.
	ID string
	// Number is the source's numbering label for the sense, when it has one.
	Number string
	// Level is the sense's effective nesting level.
	Level int
	// Definitions are the sense's definitions, in source order.
	Definitions []string
	// Cited reports whether the sense, of its entry, is in the cited set the
	// caller gave.
	Cited bool
}

// Sense is one flattened dictionary sense with its native ID.
type Sense struct {
	ID          string
	Number      string
	Level       int
	Definitions []string
	Glosses     []string
}

// Target is the outline of the entry a senseless Lewis & Short entry's resolved
// cross-reference names, led by the reference as the source writes it.
type Target struct {
	Reference string
	Outline   []OutlineSense
}

// Entry is one resolved dictionary entry. Exactly one of LSJ and LewisShort is
// set, as the card policy's entry source says.
type Entry struct {
	// ID is the source's native entry ID.
	ID string
	// Number is the entry number shown when several entries stack (the L&S
	// number, or an LSJ entry's place in order), or zero.
	Number int
	// Senses is every sense with a native ID, flattened in source order, so a
	// caller never branches on the dictionary's own hierarchy.
	Senses []Sense
	// Outline is the entry's compact outline, in source order.
	Outline []OutlineSense
	// Targets are the resolved cross-reference outlines of a Lewis & Short
	// entry with no sense of its own, in source order.
	Targets []Target

	LSJ        enrichment.AncientGreekLSJEntry
	LewisShort enrichment.LatinLSEntry
}

// Resolution is every dictionary entry found for one lemma.
type Resolution struct {
	// Entries are the entries in source order, with no cap.
	Entries []Entry
	// EntryID is every native entry ID, joined by EntryIDSeparator.
	EntryID string
	// Pin is the pinned dictionary revision the entries share.
	Pin string
}

// Resolve looks the lemma up for a Prepared deck, by the policy's deck lookup
// rule, and returns every entry with its structured outline; cited is the set of
// entry senses to flag. It resolves nothing for a policy with no dictionary, a
// provider without the capability, or a lemma with no usable entry.
func Resolve(ctx context.Context, lexical enrichment.LexicalProvider, policy cardpolicy.Policy, lemma string, cited map[SenseRef]struct{}) (Resolution, bool) {
	return resolve(ctx, lexical, policy, policy.DictionaryLookup, lemma, cited)
}

// ResolveStudy is Resolve under the policy's Study lookup rule (ADR 0139): Greek
// Study takes every competing LSJ entry in source order where the deck takes
// exactly one.
func ResolveStudy(ctx context.Context, lexical enrichment.LexicalProvider, policy cardpolicy.Policy, lemma string, cited map[SenseRef]struct{}) (Resolution, bool) {
	return resolve(ctx, lexical, policy, policy.StudyLookup, lemma, cited)
}

func resolve(ctx context.Context, lexical enrichment.LexicalProvider, policy cardpolicy.Policy, rule cardpolicy.Lookup, lemma string, cited map[SenseRef]struct{}) (Resolution, bool) {
	if lexical == nil {
		return Resolution{}, false
	}
	switch policy.Entries {
	case cardpolicy.EntryLSJ:
		if rule == cardpolicy.LookupEveryEntry {
			return resolveEveryLSJ(ctx, lexical, lemma, cited)
		}
		return resolveLSJ(ctx, lexical, lemma, cited)
	case cardpolicy.EntryLewisShort:
		return resolveLewisShort(ctx, lexical, lemma, cited)
	case cardpolicy.EntryNone:
	}
	return Resolution{}, false
}

func resolveLSJ(ctx context.Context, lexical enrichment.LexicalProvider, lemma string, cited map[SenseRef]struct{}) (Resolution, bool) {
	provider, ok := lexical.(enrichment.AncientGreekLSJProvider)
	if !ok {
		return Resolution{}, false
	}
	matched, found := provider.LookupAncientGreekLSJ(ctx, lemma)
	if !found || matched.Empty() {
		return Resolution{}, false
	}
	return Resolution{Entries: []Entry{BuildLSJ(matched, cited)}, EntryID: matched.EntryID, Pin: matched.Pin}, true
}

// resolveEveryLSJ stacks every competing LSJ entry of the lemma, in source
// order, each with its position as its label: the index carries no LSJ homograph
// number of its own.
func resolveEveryLSJ(ctx context.Context, lexical enrichment.LexicalProvider, lemma string, cited map[SenseRef]struct{}) (Resolution, bool) {
	provider, ok := lexical.(enrichment.AncientGreekLSJEntriesProvider)
	if !ok {
		return Resolution{}, false
	}
	found, ok := provider.LookupAncientGreekLSJEntries(ctx, lemma)
	if !ok {
		return Resolution{}, false
	}
	var kept []enrichment.AncientGreekLSJEntry
	for _, candidate := range found {
		if !candidate.Empty() {
			kept = append(kept, candidate)
		}
	}
	if len(kept) == 0 {
		return Resolution{}, false
	}
	entries := make([]Entry, 0, len(kept))
	ids := make([]string, 0, len(kept))
	for i, candidate := range kept {
		entry := BuildLSJ(candidate, cited)
		if len(kept) > 1 {
			entry.Number = i + 1
		}
		entries = append(entries, entry)
		ids = append(ids, candidate.EntryID)
	}
	return Resolution{Entries: entries, EntryID: strings.Join(ids, EntryIDSeparator), Pin: kept[0].Pin}, true
}

// BuildLSJ gives a matched LSJ entry its structured outline, flagging the senses
// of the entry that are in cited.
func BuildLSJ(matched enrichment.AncientGreekLSJEntry, cited map[SenseRef]struct{}) Entry {
	senses := lsjSenses(matched.Senses, 1)
	return Entry{ID: matched.EntryID, LSJ: matched, Senses: senses, Outline: outlineOf(matched.EntryID, senses, cited)}
}

func resolveLewisShort(ctx context.Context, lexical enrichment.LexicalProvider, lemma string, cited map[SenseRef]struct{}) (Resolution, bool) {
	provider, ok := lexical.(enrichment.LatinLSProvider)
	if !ok {
		return Resolution{}, false
	}
	found, ok := provider.LookupLatinLS(ctx, lemma)
	if !ok {
		return Resolution{}, false
	}
	var kept []enrichment.LatinLSEntry
	targets := map[string]enrichment.LatinLSEntry{}
	for _, candidate := range found {
		if candidate.Empty() {
			continue
		}
		kept = append(kept, candidate)
		maps.Copy(targets, resolveTargets(ctx, provider, candidate))
	}
	if len(kept) == 0 {
		return Resolution{}, false
	}
	entries := BuildLewisShort(kept, targets, cited)
	ids := make([]string, 0, len(kept))
	for _, candidate := range kept {
		ids = append(ids, candidate.EntryID)
	}
	return Resolution{Entries: entries, EntryID: strings.Join(ids, EntryIDSeparator), Pin: kept[0].Pin}, true
}

// BuildLewisShort stacks entries in the order given, each with its L&S number
// (the source's own homograph number, or its place in order when the source key
// carries none; zero for a lone entry), its structured outline and the outlines
// of its resolved cross-reference targets. cited flags the entry senses it
// holds.
func BuildLewisShort(found []enrichment.LatinLSEntry, targets map[string]enrichment.LatinLSEntry, cited map[SenseRef]struct{}) []Entry {
	entries := make([]Entry, 0, len(found))
	for i, candidate := range found {
		number := 0
		if len(found) > 1 {
			number = i + 1
			if candidate.Homograph > 0 {
				number = candidate.Homograph
			}
		}
		entries = append(entries, Entry{
			ID:         candidate.EntryID,
			Number:     number,
			LewisShort: candidate,
			Senses:     lewisShortSenses(candidate.Senses),
			Outline:    outlineOf(candidate.EntryID, lewisShortSenses(candidate.Senses), cited),
			Targets:    targetOutlines(candidate, targets, cited),
		})
	}
	return entries
}

// PassSenses are the pinned dictionary senses an entry-carrying evidence pass
// may cite across every resolved entry, by the dictionary the policy names. Each
// sense's evidence ID names its entry and its sense (enrichment.EntryEvidenceID),
// so a cited sense is exactly one sense of one entry across homographs.
func PassSenses(policy cardpolicy.Policy, entries []Entry) []enrichment.LexicalSense {
	if policy.Entries == cardpolicy.EntryLSJ {
		// Greek Study sends every sense, as it did for one entry: the cap is
		// Latin's, and a frequent lemma's entries need their later senses citable.
		var senses []enrichment.LexicalSense
		for _, entry := range entries {
			senses = append(senses, lsjEntryPassSenses(entry)...)
		}
		return senses
	}
	return LatinPassSenses(entries)
}

// LatinPassSenses are the pinned Lewis & Short senses the Latin evidence-assisted
// pass may cite, across every resolved entry. Each sense's evidence ID names its
// entry and its sense, so a cited sense is exactly one sense of one entry, even
// across stacked homographs. A sense sends its definitions, or its glosses when
// it has none; a sense with no native ID or nothing to send gives no evidence and
// is skipped. The pass carries at most enrichment.DefaultMaxCandidateSenses
// senses, taken round-robin across the entries so every returned entry is
// represented, and kept in source order.
func LatinPassSenses(entries []Entry) []enrichment.LexicalSense {
	perEntry := make([][]enrichment.LexicalSense, len(entries))
	for i, entry := range entries {
		perEntry[i] = latinEntryPassSenses(entry)
	}
	return capRoundRobin(perEntry)
}

// capRoundRobin keeps at most enrichment.DefaultMaxCandidateSenses senses, taken
// round-robin across the entries so every entry is represented, and each entry's
// senses stay in source order.
func capRoundRobin(perEntry [][]enrichment.LexicalSense) []enrichment.LexicalSense {
	keep := make([]int, len(perEntry))
	for taken := 0; taken < enrichment.DefaultMaxCandidateSenses; {
		before := taken
		for i := range perEntry {
			if keep[i] < len(perEntry[i]) && taken < enrichment.DefaultMaxCandidateSenses {
				keep[i]++
				taken++
			}
		}
		if taken == before {
			break
		}
	}
	var senses []enrichment.LexicalSense
	for i := range perEntry {
		senses = append(senses, perEntry[i][:keep[i]]...)
	}
	return senses
}

// latinEntryPassSenses are one entry's senses that have an ID and text to send.
func latinEntryPassSenses(entry Entry) []enrichment.LexicalSense {
	var senses []enrichment.LexicalSense
	for _, sense := range entry.Senses {
		text := sense.Definitions
		if len(text) == 0 {
			text = sense.Glosses
		}
		gloss := strings.Join(text, "; ")
		if strings.TrimSpace(sense.ID) != "" && strings.TrimSpace(gloss) != "" {
			senses = append(senses, enrichment.LexicalSense{EvidenceID: enrichment.EntryEvidenceID(entry.ID, sense.ID), Source: "ls", Gloss: gloss})
		}
	}
	return senses
}

// lsjEntryPassSenses are one LSJ entry's senses that have an ID and text to send:
// its definitions (ADR 0104), or every gloss when no sense of the entry has a
// definition. Each sense's evidence ID names the entry.
func lsjEntryPassSenses(entry Entry) []enrichment.LexicalSense {
	useDefinitions := slices.ContainsFunc(entry.Senses, func(sense Sense) bool { return len(sense.Definitions) > 0 })
	var senses []enrichment.LexicalSense
	for _, sense := range entry.Senses {
		text := sense.Glosses
		if useDefinitions {
			text = sense.Definitions
		}
		gloss := strings.Join(text, "; ")
		if strings.TrimSpace(sense.ID) != "" && strings.TrimSpace(gloss) != "" {
			senses = append(senses, enrichment.LexicalSense{EvidenceID: enrichment.EntryEvidenceID(entry.ID, sense.ID), Source: "lsj", Gloss: gloss})
		}
	}
	return senses
}

// LatinCitedSenseIDs returns the native sense IDs a Latin pass answer cited, in
// order. It reports false when a cited ID names no entry, or a sense the named
// entry does not hold.
func LatinCitedSenseIDs(entries []Entry, citedIDs []string) ([]string, bool) {
	native := make([]string, 0, len(citedIDs))
	for _, id := range citedIDs {
		entryID, senseID, ok := enrichment.SplitEntryEvidenceID(id)
		if !ok {
			return nil, false
		}
		held := slices.ContainsFunc(entries, func(entry Entry) bool {
			return entry.ID == entryID && slices.ContainsFunc(entry.Senses, func(sense Sense) bool { return sense.ID == senseID })
		})
		if !held {
			return nil, false
		}
		native = append(native, senseID)
	}
	return native, true
}

// HasOwnSense reports whether a Lewis & Short entry has a sense of its own: any
// sense below the level-0 pseudo-sense that carries only its citations and
// redirects.
func HasOwnSense(senses []enrichment.LatinLSSense) bool {
	return slices.ContainsFunc(senses, func(sense enrichment.LatinLSSense) bool { return sense.Level > 0 })
}

// resolveTargets returns the entries a senseless entry's resolved
// cross-references name ("v. X"), by entry ID, so the card can show the target's
// meaning in the entry's place (ADR 0132). An entry with a sense of its own, or
// a reference that does not resolve to exactly one entry, contributes nothing.
func resolveTargets(ctx context.Context, provider enrichment.LatinLSProvider, entry enrichment.LatinLSEntry) map[string]enrichment.LatinLSEntry {
	if HasOwnSense(entry.Senses) {
		return nil
	}
	targets := map[string]enrichment.LatinLSEntry{}
	for _, sense := range entry.Senses {
		for _, ref := range sense.CrossRefs {
			if ref.EntryID == "" || ref.EntryID == entry.EntryID {
				continue
			}
			if target, ok := lookupEntry(ctx, provider, ref); ok {
				targets[ref.EntryID] = target
			}
		}
	}
	return targets
}

// lookupEntry finds the entry a resolved cross-reference names among the
// entries under its target headword.
func lookupEntry(ctx context.Context, provider enrichment.LatinLSProvider, ref enrichment.LatinLSCrossRef) (enrichment.LatinLSEntry, bool) {
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

// targetOutlines returns the outline of each entry a senseless entry's resolved
// cross-references name, in source order, once per target, skipping a target
// with no outline.
func targetOutlines(entry enrichment.LatinLSEntry, targets map[string]enrichment.LatinLSEntry, cited map[SenseRef]struct{}) []Target {
	if HasOwnSense(entry.Senses) {
		return nil
	}
	var out []Target
	seen := map[string]struct{}{}
	for _, sense := range entry.Senses {
		for _, ref := range sense.CrossRefs {
			target, ok := targets[ref.EntryID]
			if _, done := seen[ref.EntryID]; !ok || done {
				continue
			}
			outline := outlineOf(ref.EntryID, lewisShortSenses(target.Senses), cited)
			if len(outline) == 0 {
				continue
			}
			seen[ref.EntryID] = struct{}{}
			out = append(out, Target{Reference: strings.TrimSpace(ref.Text), Outline: outline})
		}
	}
	return out
}

// lsjSenses flattens the LSJ hierarchy in source order with each sense's native
// ID and effective level.
func lsjSenses(senses []enrichment.AncientGreekLSJSense, depth int) []Sense {
	var out []Sense
	for _, sense := range senses {
		level := sense.Level
		if level <= 0 {
			level = depth
		}
		out = append(out, Sense{ID: sense.ID, Number: sense.Number, Level: level, Definitions: sense.Definitions, Glosses: sense.Glosses})
		out = append(out, lsjSenses(sense.Senses, level+1)...)
	}
	return out
}

// lewisShortSenses returns the entry's own senses in source order, leaving out
// the level-0 pseudo-sense.
func lewisShortSenses(senses []enrichment.LatinLSSense) []Sense {
	var out []Sense
	for _, sense := range senses {
		if sense.Level > 0 {
			out = append(out, Sense{ID: sense.ID, Number: sense.Number, Level: sense.Level, Definitions: sense.Definitions, Glosses: sense.Glosses})
		}
	}
	return out
}

// outlineOf keeps each flattened sense that has definitions, plus each heading
// whose span holds one (ADR 0104, per entry). Perseus LSJ senses are mostly flat
// siblings that carry their level, and L&S senses are flat and in id order, so a
// sense's span is the run of deeper-level senses after it, nested or not.
func outlineOf(entryID string, senses []Sense, cited map[SenseRef]struct{}) []OutlineSense {
	flat := make([]OutlineSense, 0, len(senses))
	for _, sense := range senses {
		flat = append(flat, newOutlineSense(entryID, sense.ID, sense.Number, sense.Level, sense.Definitions, cited))
	}
	return spanOutline(flat)
}

func newOutlineSense(entryID, id, number string, level int, definitions []string, cited map[SenseRef]struct{}) OutlineSense {
	_, isCited := cited[SenseRef{EntryID: entryID, SenseID: strings.TrimSpace(id)}]
	return OutlineSense{ID: id, Number: number, Level: level, Definitions: definitions, Cited: isCited && strings.TrimSpace(id) != ""}
}

// spanOutline keeps each sense that has definitions and each heading whose span
// of deeper senses holds one.
func spanOutline(flat []OutlineSense) []OutlineSense {
	var outline []OutlineSense
	for i, item := range flat {
		if len(item.Definitions) > 0 {
			outline = append(outline, item)
			continue
		}
		for j := i + 1; j < len(flat) && flat[j].Level > item.Level; j++ {
			if len(flat[j].Definitions) > 0 {
				outline = append(outline, item)
				break
			}
		}
	}
	return outline
}

// GreekPassSenses flattens a matched LSJ entry's sense hierarchy, in source
// order, into the pinned LSJ meaning evidence the evidence-assisted pass
// transmits. Each sense carries its native LSJ sense ID as its evidence ID, so
// the provider cites a pinned sense and decoding maps the short handle back to
// that exact ID. Each sense sends its definitions (ADR 0104), not the usage
// glosses that translate quoted passages; an entry derived before definitions
// existed sends every gloss instead. A sense with no native ID cannot be cited,
// and a sense with nothing to send gives no evidence, so both are skipped.
func GreekPassSenses(entry enrichment.AncientGreekLSJEntry) []enrichment.LexicalSense {
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

// Passage is the translated passage a representative sentence sits in.
type Passage struct {
	// Text is the translation of every cited section, in citation order.
	Text string
	// Citation is the cited sections as one range label.
	Citation string
	// Pin is the pinned translation the text came from.
	Pin string
}

// Empty reports whether no passage resolved.
func (p Passage) Empty() bool { return p.Text == "" }

// TranslationStore is the resolver's seam to the installation's imported
// passage translations. A Book's reading unit names the translation file it was
// imported from; the store returns that file's translations by cited section and
// the pin to record beside them, or an empty map and pin when the Book has none.
type TranslationStore interface {
	BookReferenceTranslations(ctx context.Context, owner, bookID string) (translations map[string]string, pin string, err error)
}

// ResolvePassageOfBook looks the Book's translations up in the store and
// resolves the passage the citations touch, exactly as ResolvePassage does. A
// store failure is returned; a Book with no translations, or a policy without
// them, resolves an empty Passage.
func ResolvePassageOfBook(ctx context.Context, store TranslationStore, policy cardpolicy.Policy, owner, bookID string, citations []string) (Passage, error) {
	if store == nil || policy.Translations == cardpolicy.TranslationNone || len(citations) == 0 {
		return Passage{}, nil
	}
	translations, pin, err := store.BookReferenceTranslations(ctx, owner, bookID)
	if err != nil {
		return Passage{}, err
	}
	return ResolvePassage(policy, citations, translations, pin), nil
}

// ResolvePassage returns the passage text, its citation label and the pin for a
// representative sentence's citations, or an empty Passage when the policy has
// no passage translations or the card must show no English. Every cited section
// must have a translation; a single missing key withholds the whole passage,
// never a partial one.
func ResolvePassage(policy cardpolicy.Policy, citations []string, translations map[string]string, pin string) Passage {
	if policy.Translations == cardpolicy.TranslationNone {
		return Passage{}
	}
	if strings.TrimSpace(pin) == "" || len(citations) == 0 || len(translations) == 0 {
		return Passage{}
	}
	// Every segment whose Citation equals or contains a touched one is shown
	// (ADR 0101): a 1.2 translation covers a sentence touching 1.2.3.
	shown := map[string]struct{}{}
	for _, section := range citations {
		if !collectSegments(section, translations, shown) {
			return Passage{}
		}
	}
	keys := make([]string, 0, len(shown))
	for key := range shown {
		keys = append(keys, key)
	}
	sort.Slice(keys, func(i, j int) bool {
		order, err := classical.CompareCitations(keys[i], keys[j])
		if err != nil {
			return keys[i] < keys[j]
		}
		return order < 0
	})
	sections := make([]string, 0, len(keys))
	for _, key := range keys {
		sections = append(sections, strings.TrimSpace(translations[key]))
	}
	return Passage{Text: strings.Join(sections, " "), Citation: classical.FormatCitationRange(keys), Pin: pin}
}

// collectSegments adds every segment key that equals or contains section and
// reports whether there is at least one.
func collectSegments(section string, translations map[string]string, shown map[string]struct{}) bool {
	found := false
	for key, translation := range translations {
		if strings.TrimSpace(translation) == "" || !classical.CitationContains(key, section) {
			continue
		}
		shown[key] = struct{}{}
		found = true
	}
	return found
}
