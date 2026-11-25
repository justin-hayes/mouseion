package ancientgreek

import (
	"errors"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/classical"
)

// LatinOccurrence is one printed token of a Latin Work as the dictionary-match
// gate reads it: the identity the reading path would give it. It carries no
// source text beyond the lemma.
type LatinOccurrence struct {
	// CanonicalLemma is the latin-profile key of the headword.
	CanonicalLemma string
	// Headword is the source lemma without its artefact digits.
	Headword   string
	UPOS       string
	Dependency string
}

// LatinWorkEvidence is what the translation-join and dictionary-match gates
// read from one parsed Latin Work.
type LatinWorkEvidence struct {
	// SentenceCitations lists, per sentence, every distinct Citation it touches
	// in Citation order.
	SentenceCitations [][]string
	// Occurrences lists every printed token in reading order.
	Occurrences []LatinOccurrence
}

// LatinEvidence derives the gate evidence of a Latin Work. Part of speech
// follows the same mapping and proper-name rule as the reading path, so the
// gate counts the occurrences preparation will.
func (s *ManifestSource) LatinEvidence() (LatinWorkEvidence, error) {
	if s == nil || s.doc == nil || !s.doc.latin {
		return LatinWorkEvidence{}, errors.New("ancientgreek: a parsed Latin Work is required")
	}
	profile, err := canonicalization.For(classical.LanguageLatin)
	if err != nil {
		return LatinWorkEvidence{}, err
	}
	names := properNameStats(s.doc, profile)
	var evidence LatinWorkEvidence
	for _, sentence := range s.doc.sentences {
		evidence.SentenceCitations = append(evidence.SentenceCitations, sentenceCitations(sentence))
		for _, token := range sentence.tokens {
			if isEllipsis(token.pos) {
				continue
			}
			headword, discriminator := latinDialect.decodeLemma(token.lemma)
			canonical := profile.Canonical(headword)
			upos, err := mapUPOS(token.pos, token.relation, names[canonical+"\x00"+discriminator].isProperName())
			if err != nil {
				return LatinWorkEvidence{}, err
			}
			evidence.Occurrences = append(evidence.Occurrences, LatinOccurrence{
				CanonicalLemma: canonical, Headword: headword, UPOS: upos, Dependency: repairRelation(token.relation),
			})
		}
	}
	return evidence, nil
}
