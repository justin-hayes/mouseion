package dictionary

import (
	"context"
	"database/sql"
	"strings"

	"github.com/justin-hayes/mouseion/internal/enrichment"
)

// lookupFrenchCrossPOSFallback returns weak senses for a French lookup whose
// exact-or-untagged row missed. It covers the two shapes where the analyzer's
// part of speech differs from Wiktionary's: prepositional adverbs filed under
// ADP, and participial adjectives lemmatized to the verb.
func lookupFrenchCrossPOSFallback(ctx context.Context, db *sql.DB, lemma, targetWord, upos string, withHStatus bool) ([]enrichment.LexicalSense, error) {
	switch upos {
	case "ADV":
		// Stanza analyses French prepositional adverbs as ADV while Wiktionary
		// files them under ADP only.
		_, weakSenses, _, _, err := readEntryRows(ctx, db, "fr", lemma, upos, "ADP", withHStatus)
		return weakSenses, err
	case "ADJ":
		return frenchParticipialAdjectiveFallback(ctx, db, lemma, targetWord, withHStatus)
	}
	return nil, nil
}

// frenchParticipialAdjectiveFallback resolves weak evidence for a French ADJ
// identity whose lemma is a verb. Stanza often lemmatizes a participial
// adjective to the infinitive, while Wiktionary files the adjective under the
// participle surface. It prefers that surface's ADJ row when the same surface's
// VERB row names the requested lemma as its past participle, and otherwise
// returns the lemma's VERB senses. Exactly one of the two contributes senses.
func frenchParticipialAdjectiveFallback(ctx context.Context, db *sql.DB, lemma, targetWord string, withHStatus bool) ([]enrichment.LexicalSense, error) {
	verbSenses, err := readExactEntryRows(ctx, db, "fr", lemma, "VERB", withHStatus)
	if err != nil {
		return nil, err
	}
	if len(verbSenses) == 0 {
		return nil, nil
	}
	for _, candidate := range frenchParticipleCandidates(targetWord) {
		adjectiveSenses, err := readExactEntryRows(ctx, db, "fr", candidate, "ADJ", withHStatus)
		if err != nil {
			return nil, err
		}
		if len(adjectiveSenses) == 0 {
			continue
		}
		candidateVerbSenses, err := readExactEntryRows(ctx, db, "fr", candidate, "VERB", withHStatus)
		if err != nil {
			return nil, err
		}
		if !hasPastParticipleGloss(candidateVerbSenses, lemma) {
			continue
		}
		markWeakEvidence(adjectiveSenses)
		return adjectiveSenses, nil
	}
	markWeakEvidence(verbSenses)
	return verbSenses, nil
}

// frenchParticipleCandidates derives the surfaces to try for a French ADJ
// identity whose lemma is a verb: the occurrence surface itself, then the
// surface with one trailing inflection suffix removed. The French profile
// normalizes case and accents exactly as it does for stored lemmas.
func frenchParticipleCandidates(targetWord string) []string {
	surface := normalizeLemma("fr", targetWord)
	if surface == "" {
		return nil
	}
	candidates := make([]string, 0, 4)
	seen := make(map[string]bool, 4)
	appendCandidate := func(candidate string) {
		if candidate != "" && !seen[candidate] {
			seen[candidate] = true
			candidates = append(candidates, candidate)
		}
	}
	appendCandidate(surface)
	for _, suffix := range []string{"es", "e", "s"} {
		if trimmed, found := strings.CutSuffix(surface, suffix); found {
			appendCandidate(trimmed)
		}
	}
	return candidates
}

// hasPastParticipleGloss reports whether a row's senses name lemma as the verb
// this surface is the past participle of, which ties the participle to this
// verb rather than an unrelated homograph.
func hasPastParticipleGloss(senses []enrichment.LexicalSense, lemma string) bool {
	expected := "past participle of " + lemma
	for _, sense := range senses {
		if strings.EqualFold(strings.TrimSpace(sense.Gloss), expected) {
			return true
		}
	}
	return false
}
