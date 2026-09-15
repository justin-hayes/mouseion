package cardexport

import (
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/enrichment"
)

// RenderInput is the complete input boundary for card presentation. It contains
// only frozen specification fields and the exact enrichment overlay applied to
// those fields; mutable assembly metadata is deliberately not part of it.
type RenderInput struct {
	Language, CanonicalLemma, UPOS                                            string
	Sentence, Translation, SentenceTranslation, SentenceTranslationTarget     string
	TargetWord, Gloss, Plural, IPA, PrincipalParts, DictionaryProviderVersion string
	Morphology, SourceDocument, Notes                                         string
	CandidateSenses                                                           []enrichment.LexicalSense
	SentenceTokens                                                            []analyzer.Token
	FirstEncounter                                                            int64
	fallbackGlossApplied                                                      bool
}

func renderInputFromEntry(entry Entry) RenderInput {
	return RenderInput{
		Language: entry.Language, CanonicalLemma: entry.CanonicalLemma, UPOS: entry.UPOS,
		Sentence: entry.Sentence, Translation: entry.Translation,
		SentenceTranslation: entry.SentenceTranslation, SentenceTranslationTarget: entry.SentenceTranslationTarget,
		TargetWord: entry.TargetWord, Gloss: entry.Gloss, Plural: entry.Plural, IPA: entry.IPA,
		PrincipalParts: entry.PrincipalParts, DictionaryProviderVersion: entry.DictionaryProviderVersion,
		Morphology: entry.Morphology, SourceDocument: entry.SourceDocument, Notes: entry.Notes,
		CandidateSenses: enrichment.CloneLexicalSenses(entry.CandidateSenses),
		SentenceTokens:  cloneTokens(entry.SentenceTokens), FirstEncounter: entry.FirstEncounter,
		fallbackGlossApplied: entry.fallbackGlossApplied,
	}
}

func (input RenderInput) entry() Entry {
	return Entry{
		Language: input.Language, CanonicalLemma: input.CanonicalLemma, UPOS: input.UPOS,
		Sentence: input.Sentence, Translation: input.Translation,
		SentenceTranslation: input.SentenceTranslation, SentenceTranslationTarget: input.SentenceTranslationTarget,
		TargetWord: input.TargetWord, Gloss: input.Gloss, Plural: input.Plural, IPA: input.IPA,
		PrincipalParts: input.PrincipalParts, DictionaryProviderVersion: input.DictionaryProviderVersion,
		Morphology: input.Morphology, SourceDocument: input.SourceDocument, Notes: input.Notes,
		CandidateSenses: enrichment.CloneLexicalSenses(input.CandidateSenses),
		SentenceTokens:  cloneTokens(input.SentenceTokens), FirstEncounter: input.FirstEncounter,
		fallbackGlossApplied: input.fallbackGlossApplied,
	}
}

func cloneRenderInputs(inputs []RenderInput) []RenderInput {
	if inputs == nil {
		return nil
	}
	result := make([]RenderInput, len(inputs))
	for i, input := range inputs {
		result[i] = input
		result[i].CandidateSenses = enrichment.CloneLexicalSenses(input.CandidateSenses)
		result[i].SentenceTokens = cloneTokens(input.SentenceTokens)
	}
	return result
}

func clearExternalRenderInputFields(input *RenderInput) {
	input.Translation = ""
	input.SentenceTranslation = ""
	input.SentenceTranslationTarget = ""
}
