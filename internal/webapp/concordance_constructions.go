package webapp

import (
	"net/url"
	"strconv"

	"github.com/justin-hayes/mouseion/internal/constructions"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// concordanceConstruction reports whether the applied lookup lists parallels of
// a construction rather than a lemma or a word form.
func concordanceConstruction(lookup domain.ConcordanceLookup) bool {
	return lookup.Match == domain.ConcordanceMatchConstruction
}

// concordanceConstructionName is the concept note's name of the lookup's
// construction, or the id when a note is missing.
func concordanceConstructionName(lookup domain.ConcordanceLookup) string {
	if note, ok := constructions.Notes[constructions.ID(lookup.Term)]; ok {
		return note.Name
	}
	return lookup.Term
}

// concordanceConstructionHeading names the construction, with the subtype label
// in parentheses when the lookup is filtered to one.
func concordanceConstructionHeading(lookup domain.ConcordanceLookup) string {
	name := concordanceConstructionName(lookup)
	if label, ok := constructions.SubtypeLabel(constructions.ID(lookup.Term), constructions.SubtypeID(lookup.Subtype)); ok {
		return name + " (" + label + ")"
	}
	return name
}

// concordanceTermValue is what the search box shows. A construction id is never
// a search term, so a parallels page leaves the box empty.
func concordanceTermValue(lookup domain.ConcordanceLookup) string {
	if concordanceConstruction(lookup) {
		return ""
	}
	return lookup.Term
}

// concordanceTierLabel is the heading of one scope tier.
func concordanceTierLabel(tier string) string {
	switch tier {
	case domain.ConcordanceTierThisBook:
		return "This Book"
	case domain.ConcordanceTierThisAuthor:
		return "This author"
	default:
		return "Other authors"
	}
}

// concordanceTierGroup is a run of consecutive results in one tier.
type concordanceTierGroup struct {
	Tier        string
	Occurrences []domain.ConcordanceResultOccurrence
}

// concordanceTierGroups splits a page into its consecutive tiers, so each tier
// is announced once, before its first row, on every page.
func concordanceTierGroups(occurrences []domain.ConcordanceResultOccurrence) []concordanceTierGroup {
	var groups []concordanceTierGroup
	for _, occurrence := range occurrences {
		if len(groups) == 0 || groups[len(groups)-1].Tier != occurrence.Tier {
			groups = append(groups, concordanceTierGroup{Tier: occurrence.Tier})
		}
		last := &groups[len(groups)-1]
		last.Occurrences = append(last.Occurrences, occurrence)
	}
	return groups
}

// concordanceWidenURL drops the subtype filter, keeping the construction, the
// origin Book, the origin occurrence and any lemma filter.
func concordanceWidenURL(lookup domain.ConcordanceLookup) string {
	lookup.Subtype, lookup.Revision = "", ""
	return vocabularyConcordancePageURL(1, lookup)
}

// concordanceOnlyWithURL narrows the lookup to parallels signalled by the
// origin anchor's lemma.
func concordanceOnlyWithURL(lookup domain.ConcordanceLookup) string {
	lookup.Lemma, lookup.Revision = lookup.OriginLemma, ""
	return vocabularyConcordancePageURL(1, lookup)
}

// concordanceAllLemmasURL clears the lemma filter, keeping the rest of the
// lookup.
func concordanceAllLemmasURL(lookup domain.ConcordanceLookup) string {
	lookup.Lemma, lookup.Revision = "", ""
	return vocabularyConcordancePageURL(1, lookup)
}

// concordanceLemmaDisplay is the lemma a learner sees for a lemma key of the
// parallels page: the citation form when the key is the origin anchor's own
// effective lemma and a citation form is known, otherwise the key itself.
func concordanceLemmaDisplay(result domain.ConcordanceResult, key string) string {
	if key != "" && key == result.Origin.AnchorLemma && result.Origin.AnchorHeadword != "" {
		return result.Origin.AnchorHeadword
	}
	return key
}

// concordanceOnlyWithLabel is the lemma filter's accessible name.
func concordanceOnlyWithLabel(lemma string) string {
	return "Only with " + lemma + ": parallels signalled by this lemma"
}

// concordanceAllLemmasLabel is the accessible name of the link that clears the
// lemma filter.
func concordanceAllLemmasLabel(lookup domain.ConcordanceLookup) string {
	return "All " + concordanceConstructionHeading(lookup) + ": clear the lemma filter"
}

// concordanceUnsearchedPrefix opens the line that names the Books a parallels
// lookup could not search.
const concordanceUnsearchedPrefix = "Not searched for constructions: "

// concordanceUnsearchedTail is the text after one unsearched Book's title: the
// update suffix when it has an offer, and the comma and space that separate it from the
// next Book. It is built here so no space can fall before the comma.
func concordanceUnsearchedTail(book domain.ConcordanceUnsearchedBook, last bool) string {
	tail := ""
	if book.UpdateAvailable {
		tail = concordanceUnsearchedSuffix
	}
	if !last {
		tail += ", "
	}
	return tail
}

// concordanceUnsearchedSuffix follows the title of a Book with an Analysis
// update offer.
const concordanceUnsearchedSuffix = " — Analysis update available"

// concordanceOriginStudyURL reopens Sentence study on the origin sentence, with
// the origin construction focused.
func concordanceOriginStudyURL(lookup domain.ConcordanceLookup, result domain.ConcordanceResult) string {
	origin := lookup.Origin
	if origin == nil {
		return ""
	}
	values := url.Values{
		"book": {result.Origin.BookID}, "run": {origin.RunID}, "corpus": {result.Origin.CorpusID},
		"unit": {result.Origin.UnitID}, "sentence": {strconv.Itoa(origin.Sentence)},
		"target": {strconv.Itoa(origin.Anchor)}, "target_surface": {result.Origin.AnchorSurface},
		"construction": {lookup.Term},
	}
	if lookup.Language != "" {
		values.Set("language", lookup.Language)
	}
	return "/vocabulary/concordance/sentence?" + values.Encode() + "#" + url.PathEscape(constructionAnchorID(lookup.Term, origin.Anchor))
}

// constructionAnchorID is the fragment identifier of a construction entry in
// Sentence study.
func constructionAnchorID(construction string, anchor int) string {
	return "construction-" + construction + "-" + strconv.Itoa(anchor)
}

// concordanceConstructionEntry is the one construction of a parallels row, in
// the shape Sentence study's entry uses, so the expanded row reuses its extract.
func concordanceConstructionEntry(occurrence domain.ConcordanceResultOccurrence) (sentenceStudyConstructionEntry, bool) {
	entries := sentenceStudyConstructionEntries(occurrence.Study)
	if len(entries) == 0 {
		return sentenceStudyConstructionEntry{}, false
	}
	return entries[0], true
}

// concordanceNoParallelsText says a parallels lookup found nothing, as a corpus
// result and not a failure.
func concordanceNoParallelsText(lookup domain.ConcordanceLookup, language string) string {
	return "No other analyzed occurrences of " + concordanceConstructionHeading(lookup) + " in your " + language + " Books. This is a corpus result, not a lookup failure."
}
