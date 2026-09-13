// Package gdex scores German sentences as representative vocabulary examples.
package gdex

import (
	"strings"

	"github.com/justin-hayes/mouseion/internal/analyzer"
)

const (
	optimalMinimumLength = 10
	optimalMaximumLength = 20

	deixisPenalty      = 1.0 / 32.0
	namedEntityPenalty = 1.0 / 6.0
	subordinatePenalty = 1.0 / 8.0
)

const (
	reasonNoFiniteVerbAndSubject = "no finite verb and subject"
	reasonSubordinateTarget      = "target in subordinate clause"
	reasonDeicticContext         = "deictic context"
	reasonNamedEntityDensity     = "named-entity density"
	reasonOptimalLength          = "optimal length"
	reasonOutsideOptimalLength   = "outside optimal length"
)

// SentenceQuality is the deterministic result of scoring one sentence.
// Score is in [0, 1]. A sentence that fails the syntactic knock-out has a zero
// score and is not accepted; gradual factors rank accepted sentences.
type SentenceQuality struct {
	Accepted bool
	Score    float64
	Reasons  []string
}

// ScoreSentenceQuality scores a German sentence using its normalized tokens
// and dependency structure. targetIndices are zero-based token ordinals for
// all occurrences of the selected target. Identifying those occurrences is
// deliberately left to the caller.
func ScoreSentenceQuality(sentence analyzer.Sentence, targetIndices []int) SentenceQuality {
	quality := SentenceQuality{Reasons: make([]string, 0, 4)}
	if !hasFiniteRootWithSubject(sentence.Tokens) {
		quality.Reasons = append(quality.Reasons, reasonNoFiniteVerbAndSubject)
		return quality
	}

	quality.Accepted = true
	factor := 1.0
	targetIndicesSet, validTargets := targetSet(sentence.Tokens, targetIndices)
	if allTargetsAreSubordinate(sentence.Tokens, validTargets) {
		factor *= 1 - 2*subordinatePenalty
		quality.Reasons = append(quality.Reasons, reasonSubordinateTarget)
	}

	deicticCount := 0
	entityCount := 0
	for index, token := range sentence.Tokens {
		if !targetIndicesSet[index] && isGermanDeixis(index, token) {
			deicticCount++
		}
		if strings.EqualFold(strings.TrimSpace(token.UPOS), "PROPN") {
			entityCount++
		}
	}
	if deicticCount > 0 {
		factor *= maxFactor(1 - deixisPenalty*float64(deicticCount))
		quality.Reasons = append(quality.Reasons, reasonDeicticContext)
	}
	if entityCount > 0 {
		factor *= maxFactor(1 - namedEntityPenalty*float64(entityCount))
		quality.Reasons = append(quality.Reasons, reasonNamedEntityDensity)
	}
	lengthFactor := optimalLengthFactor(len(sentence.Tokens))
	factor *= lengthFactor
	if lengthFactor == 1 {
		quality.Reasons = append(quality.Reasons, reasonOptimalLength)
	} else {
		quality.Reasons = append(quality.Reasons, reasonOutsideOptimalLength)
	}

	// The knock-out and gradual halves have equal weight, as in GDEX.
	quality.Score = 0.5 + 0.5*maxFactor(factor)
	return quality
}

func hasFiniteRootWithSubject(tokens []analyzer.Token) bool {
	for index, token := range tokens {
		if !strings.EqualFold(strings.TrimSpace(token.Dependency), "root") || !isFiniteVerb(token) {
			continue
		}
		for _, dependent := range tokens {
			if dependent.Head != uint32(index) {
				continue
			}
			switch strings.ToLower(strings.TrimSpace(dependent.Dependency)) {
			case "nsubj", "nsubj:pass", "csubj":
				return true
			}
		}
	}
	return false
}

func isFiniteVerb(token analyzer.Token) bool {
	upos := strings.ToUpper(strings.TrimSpace(token.UPOS))
	if upos != "AUX" && upos != "VERB" {
		return false
	}
	return strings.EqualFold(strings.TrimSpace(token.Morphology["VerbForm"]), "Fin")
}

func targetSet(tokens []analyzer.Token, indices []int) (map[int]bool, []int) {
	set := make(map[int]bool, len(indices))
	valid := make([]int, 0, len(indices))
	for _, index := range indices {
		if index < 0 || index >= len(tokens) || set[index] {
			continue
		}
		set[index] = true
		valid = append(valid, index)
	}
	return set, valid
}

var subordinateDependencies = map[string]struct{}{
	"acl": {}, "advcl": {}, "ccomp": {}, "csubj": {},
}

func allTargetsAreSubordinate(tokens []analyzer.Token, targets []int) bool {
	if len(targets) == 0 {
		return false
	}
	for _, target := range targets {
		if !isInSubordinateSubtree(tokens, target) {
			return false
		}
	}
	return true
}

func isInSubordinateSubtree(tokens []analyzer.Token, index int) bool {
	seen := make(map[int]bool)
	for index >= 0 && index < len(tokens) && !seen[index] {
		seen[index] = true
		if _, ok := subordinateDependencies[strings.ToLower(strings.TrimSpace(tokens[index].Dependency))]; ok {
			return true
		}
		head := int(tokens[index].Head)
		if head == index || head < 0 || head >= len(tokens) {
			break
		}
		index = head
	}
	return false
}

// These terms follow the German space/time and sentence-initial deixis terms
// used by GDEX. SCONJ is excluded because words such as "damit" and "da" can
// function as conjunctions rather than deictic adverbs.
var germanSpaceTimeDeixis = map[string]struct{}{
	"hier": {}, "dort": {}, "dorthin": {}, "da": {}, "dahin": {},
	"jetzt": {}, "gestern": {}, "daraufhin": {}, "bald": {},
	"anschließend": {}, "damals": {}, "einst": {}, "kürzlich": {},
	"neulich": {},
}

var germanSentenceInitialDeixis = map[string]struct{}{
	"später": {}, "spätere": {}, "heute": {}, "vorgestern": {},
	"morgen": {}, "übermorgen": {}, "dann": {}, "danach": {},
	"hinterher": {}, "deshalb": {}, "deswegen": {}, "damit": {},
}

var germanPersonalDeixis = map[string]struct{}{
	"prs": {}, "dem": {}, "ind": {}, "neg": {}, "tot": {},
}

func isGermanDeixis(index int, token analyzer.Token) bool {
	if strings.EqualFold(strings.TrimSpace(token.UPOS), "PRON") {
		if _, ok := germanPersonalDeixis[strings.ToLower(strings.TrimSpace(token.Morphology["PronType"]))]; ok {
			return true
		}
	}
	if strings.EqualFold(strings.TrimSpace(token.UPOS), "SCONJ") {
		return false
	}
	surface := strings.ToLower(strings.TrimSpace(token.Surface))
	if _, ok := germanSpaceTimeDeixis[surface]; ok {
		return true
	}
	if index == 0 {
		_, ok := germanSentenceInitialDeixis[surface]
		return ok
	}
	return false
}

func optimalLengthFactor(length int) float64 {
	switch {
	case length >= optimalMinimumLength && length <= optimalMaximumLength:
		return 1
	case length < optimalMinimumLength:
		if length < optimalMinimumLength/2 {
			return 0
		}
		return 1 - float64(optimalMinimumLength-length)/float64(optimalMinimumLength/2)
	case length > 2*optimalMaximumLength:
		return 0
	default:
		return float64(2*optimalMaximumLength-length) / float64(optimalMaximumLength)
	}
}

func maxFactor(factor float64) float64 {
	if factor < 0 {
		return 0
	}
	return factor
}
