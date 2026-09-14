package enrichment

import (
	"context"
	"strings"
	"unicode"

	"github.com/justin-hayes/mouseion/internal/analyzer"
)

// LexicalLookupRequest contains the lexical context needed to resolve one
// candidate. Sentence tokens are optional for providers without NLP context.
type LexicalLookupRequest struct {
	Language, CanonicalLemma, UPOS string
	TargetWord                     string
	RepresentativeSentence         string
	SentenceTokens                 []analyzer.Token
}

// LexicalSense is one ordered gloss sense with the context metadata retained
// by the local dictionary index.
type LexicalSense struct {
	Gloss    string   `json:"gloss"`
	Examples []string `json:"examples,omitempty"`
	Topics   []string `json:"topics,omitempty"`
	Tags     []string `json:"tags,omitempty"`
	Phrase   string   `json:"phrase,omitempty"`
	Gender   string   `json:"gender,omitempty"`
	Article  string   `json:"article,omitempty"`
	Plural   string   `json:"plural,omitempty"`
	IPA      string   `json:"ipa,omitempty"`
}

// LexicalEntry is the structured result of a local lexical lookup. Senses are
// ordered for display; the leading sense supplies the preferred morphology.
type LexicalEntry struct {
	Senses          []LexicalSense
	CandidateSenses []LexicalSense
	Gender          string
	Article         string
	Plural          string
	IPA             string
	PrincipalParts  string
}

// LexicalProvider is the consent-free local lexical-provider seam. found=false
// is a normal result for an unindexed lemma and must not prevent card export.
type LexicalProvider interface {
	Name() string
	Version() string
	Lookup(context.Context, LexicalLookupRequest) (LexicalEntry, bool, error)
}

const (
	DefaultMaxSenses          = 3
	DefaultMaxTokens          = 10
	DefaultMaxCandidateSenses = 8
	glossSeparator            = " · "
	minimumContextScore       = 2
)

// OrderSenses applies a deterministic, small Lesk-style context score. The
// original order is retained for ties, which makes the primary Wiktionary
// sense the fallback when the sentence provides no useful signal.
func OrderSenses(request LexicalLookupRequest, senses []LexicalSense) []LexicalSense {
	ordered := cloneSenses(senses)
	if len(ordered) < 2 {
		return ordered
	}
	contextWords := sentenceContext(request)
	type scoredSense struct {
		sense LexicalSense
		score int
	}
	scored := make([]scoredSense, len(ordered))
	for i, sense := range ordered {
		score := 0
		for word := range senseWords(sense) {
			if contextWords[word] {
				score++
			}
		}
		if phrase := sensePhrase(sense.Phrase); len(phrase) > 0 && containsPhrase(request.RepresentativeSentence, phrase) {
			score += 100
		}
		scored[i] = scoredSense{sense: sense, score: score}
	}
	maxScore := scored[0].score
	for _, item := range scored[1:] {
		if item.score > maxScore {
			maxScore = item.score
		}
	}
	// Stable insertion order is intentional. In particular, a low-signal
	// sentence must not displace the dictionary's primary sense.
	if maxScore >= minimumContextScore {
		for i := 1; i < len(scored); i++ {
			item := scored[i]
			j := i
			for j > 0 && scored[j-1].score < item.score {
				scored[j] = scored[j-1]
				j--
			}
			scored[j] = item
		}
	}
	for i, item := range scored {
		ordered[i] = item.sense
	}
	return ordered
}

// RenderGloss renders the compact, stable field used on the card back.
func RenderGloss(senses []LexicalSense, maxSenses, maxTokens int) string {
	if maxSenses <= 0 {
		maxSenses = DefaultMaxSenses
	} else if maxSenses > DefaultMaxSenses {
		maxSenses = DefaultMaxSenses
	}
	if maxTokens <= 0 {
		maxTokens = DefaultMaxTokens
	} else if maxTokens > DefaultMaxTokens {
		maxTokens = DefaultMaxTokens
	}
	parts := make([]string, 0, min(maxSenses, len(senses)))
	for _, sense := range senses {
		words := strings.Fields(strings.TrimSpace(sense.Gloss))
		if len(words) == 0 {
			continue
		}
		if len(words) > maxTokens {
			words = words[:maxTokens]
		}
		parts = append(parts, strings.Join(words, " "))
		if len(parts) == maxSenses {
			break
		}
	}
	return strings.Join(parts, glossSeparator)
}

func sentenceContext(request LexicalLookupRequest) map[string]bool {
	words := make(map[string]bool)
	if len(request.SentenceTokens) == 0 {
		fields := strings.Fields(request.RepresentativeSentence)
		target := normalizeWord(request.TargetWord)
		if target == "" {
			return words
		}
		for i, field := range fields {
			if normalizeWord(field) != target {
				continue
			}
			start, end := i-5, i+6
			if start < 0 {
				start = 0
			}
			if end > len(fields) {
				end = len(fields)
			}
			for _, candidate := range fields[start:end] {
				if word := normalizeWord(candidate); isContextWord(word) && word != target {
					words[word] = true
				}
			}
			return words
		}
		return words
	}
	target := normalizeWord(request.TargetWord)
	if target == "" {
		return words
	}
	index := -1
	for i, token := range request.SentenceTokens {
		if normalizeWord(token.Surface) == target || normalizeWord(token.CanonicalLemma) == target {
			index = i
			break
		}
	}
	if index < 0 {
		return words
	}
	start, end := index-5, index+6
	if start < 0 {
		start = 0
	}
	if end > len(request.SentenceTokens) {
		end = len(request.SentenceTokens)
	}
	for _, token := range request.SentenceTokens[start:end] {
		if word := normalizeWord(token.CanonicalLemma); isContextWord(word) && word != target && contentPOS(token.UPOS) {
			words[word] = true
		}
		if relation := normalizeWord(token.Dependency); relation != "" {
			words[relation] = true
		}
	}
	return words
}

func senseWords(sense LexicalSense) map[string]bool {
	values := append([]string{sense.Gloss}, sense.Examples...)
	values = append(values, sense.Topics...)
	values = append(values, sense.Tags...)
	if sense.Phrase != "" {
		values = append(values, sense.Phrase)
	}
	words := make(map[string]bool)
	for _, value := range values {
		for _, field := range strings.Fields(value) {
			if word := normalizeWord(field); word != "" {
				words[word] = true
			}
		}
	}
	return words
}

func contentPOS(pos string) bool {
	switch strings.ToUpper(strings.TrimSpace(pos)) {
	case "", "PUNCT", "SYM", "SPACE", "X", "DET", "AUX", "PRON":
		return false
	default:
		return true
	}
}

func normalizeWord(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.TrimFunc(value, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
	return value
}

var contextStopWords = map[string]struct{}{
	// German, Italian, and English function words are not useful Lesk signal.
	"a": {}, "ai": {}, "al": {}, "alla": {}, "alle": {}, "agli": {}, "an": {}, "and": {}, "auf": {}, "aus": {}, "auch": {}, "bei": {}, "bin": {}, "bis": {}, "con": {}, "come": {}, "da": {}, "dal": {}, "dalle": {}, "dass": {}, "das": {}, "de": {}, "dei": {}, "degli": {}, "del": {}, "della": {}, "delle": {}, "der": {}, "des": {}, "die": {}, "di": {}, "e": {}, "ein": {}, "eine": {}, "einer": {}, "eines": {}, "el": {}, "en": {}, "er": {}, "es": {}, "è": {}, "for": {}, "from": {}, "gli": {}, "haben": {}, "hat": {}, "he": {}, "i": {}, "ich": {}, "il": {}, "im": {}, "in": {}, "is": {}, "ist": {}, "la": {}, "le": {}, "lo": {}, "ma": {}, "man": {}, "mein": {}, "mit": {}, "nach": {}, "nei": {}, "nel": {}, "nella": {}, "nelle": {}, "nicht": {}, "non": {}, "o": {}, "of": {}, "on": {}, "oder": {}, "per": {}, "she": {}, "si": {}, "sie": {}, "sind": {}, "so": {}, "sono": {}, "su": {}, "sugli": {}, "sui": {}, "sul": {}, "the": {}, "to": {}, "tra": {}, "tu": {}, "und": {}, "un": {}, "una": {}, "uno": {}, "was": {}, "we": {}, "wie": {}, "wir": {}, "with": {}, "wo": {}, "wird": {}, "zu": {},
}

func isContextWord(word string) bool {
	if word == "" {
		return false
	}
	_, stop := contextStopWords[word]
	return !stop
}

func sensePhrase(value string) []string {
	var words []string
	for _, field := range strings.Fields(value) {
		if word := normalizeWord(field); word != "" {
			words = append(words, word)
		}
	}
	return words
}

func containsPhrase(sentence string, phrase []string) bool {
	if len(phrase) == 0 {
		return false
	}
	var words []string
	for _, field := range strings.Fields(sentence) {
		if word := normalizeWord(field); word != "" {
			words = append(words, word)
		}
	}
	for start := 0; start+len(phrase) <= len(words); start++ {
		matched := true
		for offset, word := range phrase {
			if words[start+offset] != word {
				matched = false
				break
			}
		}
		if matched {
			return true
		}
	}
	return false
}

func cloneSenses(senses []LexicalSense) []LexicalSense {
	if senses == nil {
		return nil
	}
	result := make([]LexicalSense, len(senses))
	for i, sense := range senses {
		result[i] = sense
		result[i].Examples = append([]string(nil), sense.Examples...)
		result[i].Topics = append([]string(nil), sense.Topics...)
		result[i].Tags = append([]string(nil), sense.Tags...)
	}
	return result
}
