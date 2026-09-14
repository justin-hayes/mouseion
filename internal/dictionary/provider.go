// Package dictionary provides the local, consent-free lexical lookup used by
// recognition card export.
package dictionary

import (
	"context"
	"strings"
	"unicode"

	"github.com/justin-hayes/mouseion/internal/analyzer"
)

// LookupRequest contains only the lexical context needed to resolve one
// candidate. Sentence tokens are optional for providers that do not have NLP
// context available.
type LookupRequest struct {
	Language        string
	CanonicalLemma  string
	UPOS            string
	TargetWord      string
	ExampleSentence string
	SentenceTokens  []analyzer.Token
}

// Sense is the compact subset of a Wiktionary sense retained by the index.
type Sense struct {
	Gloss    string
	Examples []string
	Topics   []string
	Tags     []string
	Phrase   string
	Gender   string
	Article  string
	Plural   string
	IPA      string
}

type Result struct {
	Senses     []Sense
	Gloss      string
	Morphology map[string]string
	Gender     string
	Article    string
	Plural     string
}

// Provider is the single lexical-provider seam. found=false is a normal
// result for an unindexed lemma and must not prevent card export.
type Provider interface {
	Name() string
	Version() string
	Lookup(context.Context, LookupRequest) (Result, bool, error)
}

const (
	DefaultMaxSenses = 3
	DefaultMaxTokens = 10
)

// OrderSenses applies a deterministic, small Lesk-style context score. The
// original order is retained for ties, which makes the primary Wiktionary
// sense the fallback when the sentence provides no useful signal.
func OrderSenses(request LookupRequest, senses []Sense) []Sense {
	ordered := cloneSenses(senses)
	if len(ordered) < 2 {
		return ordered
	}
	contextWords := sentenceContext(request)
	type scoredSense struct {
		sense Sense
		score int
	}
	scored := make([]scoredSense, len(ordered))
	for i, sense := range ordered {
		score := 0
		for _, word := range senseWords(sense) {
			if contextWords[word] {
				score++
			}
		}
		if phrase := normalizePhrase(sense.Phrase); phrase != "" && containsFolded(request.ExampleSentence, phrase) {
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
	if maxScore > 0 {
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
func RenderGloss(senses []Sense, maxSenses, maxTokens int) string {
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
	return strings.Join(parts, " · ")
}

func sentenceContext(request LookupRequest) map[string]bool {
	words := make(map[string]bool)
	if len(request.SentenceTokens) == 0 {
		fields := strings.Fields(request.ExampleSentence)
		target := normalizeWord(request.TargetWord)
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
				if word := normalizeWord(candidate); word != "" {
					words[word] = true
				}
			}
			return words
		}
		for _, field := range fields {
			if word := normalizeWord(field); word != "" {
				words[word] = true
			}
		}
		return words
	}
	target := normalizeWord(request.TargetWord)
	index := -1
	for i, token := range request.SentenceTokens {
		if normalizeWord(token.Surface) == target || normalizeWord(token.CanonicalLemma) == target {
			index = i
			break
		}
	}
	if index < 0 {
		index = len(request.SentenceTokens) / 2
	}
	start, end := index-5, index+6
	if start < 0 {
		start = 0
	}
	if end > len(request.SentenceTokens) {
		end = len(request.SentenceTokens)
	}
	for _, token := range request.SentenceTokens[start:end] {
		if word := normalizeWord(token.CanonicalLemma); word != "" && contentPOS(token.UPOS) {
			words[word] = true
		}
		if relation := normalizeWord(token.Dependency); relation != "" {
			words[relation] = true
		}
	}
	return words
}

func senseWords(sense Sense) []string {
	values := append([]string{sense.Gloss}, sense.Examples...)
	values = append(values, sense.Topics...)
	values = append(values, sense.Tags...)
	if sense.Phrase != "" {
		values = append(values, sense.Phrase)
	}
	var words []string
	for _, value := range values {
		for _, field := range strings.Fields(value) {
			if word := normalizeWord(field); word != "" {
				words = append(words, word)
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

func normalizePhrase(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

func containsFolded(haystack, needle string) bool {
	return strings.Contains(strings.ToLower(strings.Join(strings.Fields(haystack), " ")), needle)
}

func cloneSenses(senses []Sense) []Sense {
	result := make([]Sense, len(senses))
	for i, sense := range senses {
		result[i] = sense
		result[i].Examples = append([]string(nil), sense.Examples...)
		result[i].Topics = append([]string(nil), sense.Topics...)
		result[i].Tags = append([]string(nil), sense.Tags...)
	}
	return result
}
