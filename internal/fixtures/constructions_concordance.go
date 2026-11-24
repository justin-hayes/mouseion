package fixtures

// Contract status: illustrative. Canned state for browser scenarios; not held
// to internal/storecontract parity (ADR 0088).

import "github.com/justin-hayes/mouseion/internal/domain"

// fixtureConstructionRow is one synthetic parallel: where it sits and the
// Book it belongs to.
type fixtureConstructionRow struct {
	tier, book, title string
	sentence          int64
}

// fixtureConstructionRows are the synthetic parallels, already in the order a
// real page lists them: two in the origin Book, two more in another Book of the
// same author, and two by another author. They are not a claim about the Greek.
var fixtureConstructionRows = []fixtureConstructionRow{
	{domain.ConcordanceTierThisBook, fixtureSyntaxBookID, "Synthetic Greek syntax", 1},
	{domain.ConcordanceTierThisBook, fixtureSyntaxBookID, "Synthetic Greek syntax", 2},
	{domain.ConcordanceTierThisAuthor, "fixture-kwic-syntax-two", "Synthetic Greek syntax, part two", 1},
	{domain.ConcordanceTierThisAuthor, "fixture-kwic-syntax-two", "Synthetic Greek syntax, part two", 2},
	{domain.ConcordanceTierOtherAuthors, "fixture-kwic-other", "Synthetic Greek other", 1},
	{domain.ConcordanceTierOtherAuthors, "fixture-kwic-other", "Synthetic Greek other", 2},
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
		WorkTitle: "Synthetic Work", Author: "Synthetic Author", Current: true,
	}
	occurrence, ok := fixtureConstructionOccurrence(query.Term)
	if !ok {
		return result
	}
	var matches []domain.ConcordanceResultOccurrence
	for _, row := range fixtureConstructionRows {
		if query.Subtype != "" && query.Subtype != occurrence.SubtypeID {
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
