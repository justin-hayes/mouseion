package agdt

import "github.com/justin-hayes/mouseion/internal/syntax"

// wording is Mouseion's own plain-English naming of each category. The Name
// fills the Relation column of Word relations; the RolePhrase completes
// "‹an unprinted word›, which is {phrase} {word}". Compiled, reviewed data
// (ADR 0124): wording changes need Justin's approval.
var wording = map[syntax.Category]struct{ Name, RolePhrase string }{
	syntax.CategoryPredicate:                {"main verb", "the main verb of"},
	syntax.CategorySubject:                  {"subject", "the subject of"},
	syntax.CategoryObject:                   {"object", "the object of"},
	syntax.CategoryAttribute:                {"describes a noun", "a word describing"},
	syntax.CategoryAdverbial:                {"adverbial (how, when, why…)", "an adverbial of"},
	syntax.CategoryPredicateNominal:         {"predicate noun or adjective", "the predicate noun or adjective of"},
	syntax.CategoryObjectComplement:         {"object complement", "the object complement of"},
	syntax.CategoryPredicativeComplement:    {"predicative complement", "a predicative complement of"},
	syntax.CategoryApposition:               {"in apposition", "in apposition to"},
	syntax.CategoryCoordination:             {"joins coordinated words", "a word joining coordinated words under"},
	syntax.CategorySubordinatingConjunction: {"subordinating conjunction", "the conjunction under"},
	syntax.CategoryPreposition:              {"preposition", "the preposition under"},
	syntax.CategorySentenceParticle:         {"sentence particle", "a sentence particle in"},
	syntax.CategoryEmphasisingParticle:      {"emphasising or negating particle", "a particle on"},
	syntax.CategoryAuxiliary:                {"auxiliary", "an auxiliary of"},
	syntax.CategoryOutsideStructure:         {"stands outside the sentence structure", "outside the sentence structure, beside"},
	syntax.CategoryOther:                    {"relation not labelled", "attached to"},
}

// RelationName is the plain-English name shown for a category. Punctuation has
// no row, and falls back to the unlabelled wording.
func RelationName(category syntax.Category) string {
	if w, ok := wording[category]; ok {
		return w.Name
	}
	return wording[syntax.CategoryOther].Name
}

// RolePhrase is the phrase that names a category in the unprinted-head
// sentence. Punctuation falls back to the unlabelled wording.
func RolePhrase(category syntax.Category) string {
	if w, ok := wording[category]; ok {
		return w.RolePhrase
	}
	return wording[syntax.CategoryOther].RolePhrase
}
