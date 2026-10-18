package testutil

import (
	"strings"
	"unicode/utf8"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/checked"
)

// CorpusWord is one vocabulary identity seeded into a corpus's analysis
// evidence. Occurrences is how many sentences carry it. Each occurrence is a
// gate-passing sentence unless Fragment asks for a one-token verbless sentence,
// which the sentence-quality gate rejects.
type CorpusWord struct {
	Lemma                string
	UPOS                 string
	LexicalDiscriminator string
	Headword             string
	Fragment             bool
	Occurrences          int
}

// SeededSentence is a sentence the corpus seeder writes, with the dependency
// tokens the sentence-quality gate scores. Token offsets are relative to the
// sentence's own start, in Unicode code points.
type SeededSentence struct {
	Text   string
	Tokens []analyzer.Token
}

// CorpusSentence returns the sentence the seeder writes for word: a fragment
// when the word declares one, a gate-passing sentence otherwise.
func CorpusSentence(language string, word CorpusWord) SeededSentence {
	if word.Fragment {
		return fragmentSentence(word)
	}
	return GatePassingSentence(language, word)
}

// GatePassingSentence returns a deterministic sentence that contains word's
// lemma as a word and passes the sentence-quality gate for language. Its only
// vocabulary-eligible token is the target, so seeding it adds no other
// candidate identity.
func GatePassingSentence(language string, word CorpusWord) SeededSentence {
	template := sentenceTemplate(canonicalization.NormalizeLanguage(language))
	tokens := make([]analyzer.Token, 0, len(template))
	for _, part := range template {
		token := analyzer.Token{
			Surface: part.surface, RawLemma: part.surface, CanonicalLemma: part.surface,
			UPOS: part.upos, Dependency: part.dependency, Head: mustUint32(part.head),
			Morphology: part.morphology,
		}
		if token.Morphology == nil {
			token.Morphology = map[string]string{}
		}
		if part.target {
			token.Surface, token.RawLemma, token.CanonicalLemma = word.Lemma, word.Lemma, word.Lemma
			token.UPOS = word.UPOS
			token.LexicalDiscriminator = word.LexicalDiscriminator
			token.Headword = word.Headword
		}
		tokens = append(tokens, token)
	}
	return layoutSentence(tokens)
}

// fragmentSentence returns a one-token verbless sentence that the
// sentence-quality gate rejects: no terminal boundary and too short.
func fragmentSentence(word CorpusWord) SeededSentence {
	return SeededSentence{
		Text: word.Lemma,
		Tokens: []analyzer.Token{{
			Surface: word.Lemma, RawLemma: word.Lemma, CanonicalLemma: word.Lemma,
			UPOS: word.UPOS, Dependency: "root", Head: 0, Morphology: map[string]string{},
			LexicalDiscriminator: word.LexicalDiscriminator, Headword: word.Headword,
			Location: analyzer.SourceLocation{StartOffset: 0, EndOffset: mustUint64(utf8.RuneCountInString(word.Lemma))},
		}},
	}
}

// templatePart is one word of a seeded sentence before the target is
// substituted. Exactly one part per template is the target.
type templatePart struct {
	target                    bool
	surface, upos, dependency string
	head                      int
	morphology                map[string]string
}

// sentenceTemplate is the per-language word order. Every non-target token takes
// a part of speech outside the vocabulary-eligible set, so the sentence adds the
// target identity and nothing else. The German template carries the finite verb
// with a nominal subject its GDEX overlay requires; the others satisfy the
// generic boundary and length rules.
func sentenceTemplate(language string) []templatePart {
	switch language {
	case "de":
		return []templatePart{
			{surface: "Anna", upos: "PROPN", dependency: "nsubj", head: 1},
			{surface: "ist", upos: "AUX", dependency: "root", head: 1, morphology: map[string]string{"VerbForm": "Fin"}},
			{surface: "nicht", upos: "PART", dependency: "advmod", head: 1},
			{surface: "mit", upos: "ADP", dependency: "case", head: 5},
			{surface: "dem", upos: "DET", dependency: "det", head: 5},
			{target: true, dependency: "obl", head: 1},
		}
	case "el":
		return []templatePart{
			{surface: "Αυτό", upos: "PRON", dependency: "nsubj", head: 1},
			{surface: "είναι", upos: "AUX", dependency: "root", head: 1, morphology: map[string]string{"VerbForm": "Fin"}},
			{surface: "το", upos: "DET", dependency: "det", head: 3},
			{target: true, dependency: "obl", head: 1},
			{surface: "και", upos: "CCONJ", dependency: "cc", head: 5},
			{surface: "αυτό", upos: "PRON", dependency: "conj", head: 3},
		}
	case "grc":
		return []templatePart{
			{surface: "οὗτος", upos: "PRON", dependency: "nsubj", head: 1},
			{surface: "ἐστὶ", upos: "AUX", dependency: "root", head: 1, morphology: map[string]string{"VerbForm": "Fin"}},
			{surface: "τὸ", upos: "DET", dependency: "det", head: 3},
			{target: true, dependency: "obl", head: 1},
			{surface: "καὶ", upos: "CCONJ", dependency: "cc", head: 5},
			{surface: "τοῦτο", upos: "PRON", dependency: "conj", head: 3},
		}
	default:
		return []templatePart{
			{surface: "This", upos: "DET", dependency: "nsubj", head: 1},
			{surface: "is", upos: "AUX", dependency: "root", head: 1, morphology: map[string]string{"VerbForm": "Fin"}},
			{surface: "the", upos: "DET", dependency: "det", head: 3},
			{target: true, dependency: "obl", head: 1},
			{surface: "and", upos: "CCONJ", dependency: "cc", head: 5},
			{surface: "that", upos: "PRON", dependency: "conj", head: 3},
		}
	}
}

// layoutSentence joins the template's words with spaces, appends the terminal
// full stop and records each token's span relative to the sentence start.
func layoutSentence(tokens []analyzer.Token) SeededSentence {
	var builder strings.Builder
	offset := 0
	for index := range tokens {
		if index > 0 {
			builder.WriteByte(' ')
			offset++
		}
		start := offset
		builder.WriteString(tokens[index].Surface)
		offset += utf8.RuneCountInString(tokens[index].Surface)
		tokens[index].Location = analyzer.SourceLocation{StartOffset: mustUint64(start), EndOffset: mustUint64(offset)}
	}
	builder.WriteByte('.')
	return SeededSentence{Text: builder.String(), Tokens: tokens}
}

// mustUint64 converts a sentence-relative offset. Seeded offsets never exceed
// the range of uint64, so a failure here is a programming error.
func mustUint64(value int) uint64 {
	converted, err := checked.Uint64FromInt(value)
	if err != nil {
		panic(err)
	}
	return converted
}

// mustUint32 converts a seeded token head ordinal.
func mustUint32(value int) uint32 {
	converted, err := checked.Uint32FromInt(value)
	if err != nil {
		panic(err)
	}
	return converted
}
