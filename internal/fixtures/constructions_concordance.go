package fixtures

// Contract status: illustrative. Canned state for browser scenarios; not held
// to internal/storecontract parity (ADR 0088).

import (
	"strings"

	"github.com/justin-hayes/mouseion/internal/domain"
)

// fixtureConstructionRow is one synthetic parallel: where it sits and the
// Book it belongs to.
type fixtureConstructionRow struct {
	tier, book, title string
	sentence          int64
	// otherLemma marks a row whose anchor has a different effective lemma, so
	// the "Only with" filter drops it.
	otherLemma bool
}

// fixtureConstructionRows are the synthetic parallels, already in the order a
// real page lists them: two in the origin Book, two more in another Book of the
// same author, and two by another author. They are not a claim about the Greek.
var fixtureConstructionRows = []fixtureConstructionRow{
	{domain.ConcordanceTierThisBook, fixtureSyntaxBookID, "Synthetic Greek syntax", 1, false},
	{domain.ConcordanceTierThisBook, fixtureSyntaxBookID, "Synthetic Greek syntax", 2, true},
	{domain.ConcordanceTierThisAuthor, "fixture-kwic-syntax-two", "Synthetic Greek syntax, part two", 1, false},
	{domain.ConcordanceTierThisAuthor, "fixture-kwic-syntax-two", "Synthetic Greek syntax, part two", 2, true},
	{domain.ConcordanceTierOtherAuthors, "fixture-kwic-other", "Synthetic Greek other", 1, false},
	{domain.ConcordanceTierOtherAuthors, "fixture-kwic-other", "Synthetic Greek other", 2, true},
}

// fixtureConstructionConcordance serves a small parallels page that shows all
// three tiers, so browser smoke can exercise them without PostgreSQL. Its only
// valid origin is the synthetic syntax sentence of the origin Book; any other
// origin is invalid, as an unowned or mismatched one is for the real store.
func fixtureConstructionConcordance(result domain.ConcordanceResult, query domain.ConcordanceLookup) domain.ConcordanceResult {
	result.Match, result.Term = domain.ConcordanceMatchConstruction, query.Term
	if query.Origin == nil || query.Origin.RunID != fixtureSyntaxRunID || query.Priority != fixtureSyntaxBookID {
		result.InvalidOrigin = true
		return result
	}
	result.PriorityBookID, result.PriorityTitle = fixtureSyntaxBookID, "Synthetic Greek syntax"
	result.Origin = domain.ConcordanceConstructionOriginSentence{
		BookID: fixtureSyntaxBookID, BookTitle: "Synthetic Greek syntax", CorpusID: "fixture-corpus",
		UnitID: fixtureSyntaxUnit, AnchorSurface: fixtureConstructionAnchorSurface(query.Origin.Anchor),
		AnchorLemma: fixtureLemmaKey(fixtureConstructionAnchorSurface(query.Origin.Anchor)), AnchorHeadword: fixtureConstructionAnchorSurface(query.Origin.Anchor),
		WorkTitle: "Synthetic Work", Author: "Synthetic Author", Current: true,
	}
	// Two Books cannot be searched: one with an Analysis update offer, one
	// without, so the line's two wordings are both reachable.
	result.UnsearchedBooks = []domain.ConcordanceUnsearchedBook{
		{BookID: "fixture-unsearched-apology", Title: "Synthetic Apology", UpdateAvailable: true},
		{BookID: "fixture-unsearched-crito", Title: "Synthetic Crito"},
	}
	occurrence, ok := fixtureConstructionOccurrence(query.Term)
	if !ok {
		return result
	}
	anchorLemma := fixtureLemmaKey(fixtureConstructionAnchorSurface(occurrence.Anchor))
	var matches []domain.ConcordanceResultOccurrence
	for _, row := range fixtureConstructionRows {
		if query.Subtype != "" && query.Subtype != occurrence.SubtypeID {
			continue
		}
		if query.Lemma != "" && (row.otherLemma || query.Lemma != anchorLemma) {
			continue
		}
		matches = append(matches, fixtureConstructionParallel(row, occurrence))
	}
	fixtureConcordancePage(&result, matches, query.Page)
	return result
}

// fixtureConstructionAnchorSurface is the surface of one printed word of the
// synthetic syntax sentence.
func fixtureConstructionAnchorSurface(ordinal int) string {
	surfaces := []string{"λέγει", "ὅτι", "ἄνδρας", ",", "καλούς", "."}
	if ordinal < 0 || ordinal >= len(surfaces) {
		return ""
	}
	return surfaces[ordinal]
}

// fixtureConstructionOccurrence picks the synthetic occurrence of the
// construction from the syntax sentence's three.
func fixtureConstructionOccurrence(construction string) (domain.SentenceStudyConstruction, bool) {
	for _, occurrence := range fixtureSyntaxConstructions() {
		if occurrence.ConstructionID == construction {
			return occurrence, true
		}
	}
	return domain.SentenceStudyConstruction{}, false
}

// fixtureConstructionParallel builds one row: a key-word-in-context occurrence
// centred on the anchor, with the sentence's words and the one construction so
// the expanded row reprints the Sentence study extract.
func fixtureConstructionParallel(row fixtureConstructionRow, occurrence domain.SentenceStudyConstruction) domain.ConcordanceResultOccurrence {
	study := fixtureSyntaxStudy(row.book, row.sentence, -1, "")
	study.BookTitle = row.title
	study.Constructions = []domain.SentenceStudyConstruction{occurrence}
	anchor := study.Tokens[occurrence.Anchor]
	return domain.ConcordanceResultOccurrence{
		Surface: anchor.Surface, CanonicalLemma: anchor.Surface, UPOS: anchor.UPOS, Headword: anchor.Surface,
		SentenceText: fixtureSyntaxSentence, SentenceStartOffset: anchor.StartOffset, SentenceEndOffset: anchor.EndOffset,
		UnitStartOffset: anchor.StartOffset, UnitEndOffset: anchor.EndOffset,
		BookID: row.book, BookTitle: row.title, SourceMaterialID: "fixture-source", AnalysisRunID: fixtureSyntaxRunID,
		CorpusID: "fixture-corpus", UnitID: fixtureSyntaxUnit, ChapterTitle: "Synthetic 1.1",
		SentenceOrdinal: row.sentence, TokenOrdinal: anchor.Ordinal,
		EffectiveLemma: anchor.Surface, Tier: row.tier, Anchor: occurrence.Anchor, ConstructionExtent: occurrence.Extent,
		ConstructionID: occurrence.ConstructionID, SubtypeID: occurrence.SubtypeID, Study: study,
	}
}

// fixtureLemmaKey is the stored lemma key of a surface: the Greek profile folds
// the final sigma, so the key is not what a learner reads.
func fixtureLemmaKey(surface string) string { return strings.ReplaceAll(surface, "ς", "σ") }
