package prepareddeck

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/idiom"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	idiomLemmaSentence   = "Luca è davvero un ragazzo in gamba oggi."
	literalGambaSentence = "Mi fa male la gamba oggi."
)

// idiomLemmaDictionary answers the lemma lookup and exposes the in gamba entry
// the matcher needs for the gamba fixture.
type idiomLemmaDictionary struct{}

func (idiomLemmaDictionary) Name() string    { return "dictionary" }
func (idiomLemmaDictionary) Version() string { return "dictionary-v1" }

func (idiomLemmaDictionary) Lookup(context.Context, enrichment.LexicalLookupRequest) (enrichment.LexicalEntry, bool, error) {
	return enrichment.LexicalEntry{Senses: []enrichment.LexicalSense{{Gloss: "leg"}}}, true, nil
}

func (idiomLemmaDictionary) IdiomEntries(_ context.Context, language, word string) ([]idiom.Entry, error) {
	if language != "it" || (word != "gamba" && word != "in") {
		return nil, nil
	}
	return []idiom.Entry{{
		Language: "it", Headword: "in gamba", UPOS: "ADJ",
		Words: []string{"in", "gamba"}, Source: "wiktionary", Version: "fixture-v1",
	}}, nil
}

// idiomLemmaAssembler freezes one Italian gamba candidate. The idiom sentence
// has the higher sentence-quality score; onlyIdiom removes the literal one.
type idiomLemmaAssembler struct {
	onlyIdiom bool
}

func (a idiomLemmaAssembler) AssemblePreparedDeckInputs(_ context.Context, _ pgx.Tx, prep domain.DeckPreparation) ([]cardexport.CandidateProjection, string, error) {
	candidate := domain.SelectionCandidate{
		OwnerID: prep.OwnerID, CorpusID: "corpus-it", Language: "it",
		CanonicalLemma: "gamba", UPOS: "NOUN", OccurrenceCount: 2,
		ObservedForms: []byte(`["gamba"]`),
		SentenceReferences: []byte(`[
			{"sentence_index":0,"location":{"start_offset":10}},
			{"sentence_index":1,"location":{"start_offset":100}}
		]`),
	}
	sentences := map[int64]analyzer.Sentence{
		0: {Text: idiomLemmaSentence, Tokens: []analyzer.Token{
			{Surface: "Luca", CanonicalLemma: "luca", UPOS: "PROPN", Location: analyzer.SourceLocation{StartOffset: 0, EndOffset: 4}},
			{Surface: "è", CanonicalLemma: "essere", UPOS: "VERB", Location: analyzer.SourceLocation{StartOffset: 5, EndOffset: 6}},
			{Surface: "davvero", CanonicalLemma: "davvero", UPOS: "ADV", Location: analyzer.SourceLocation{StartOffset: 7, EndOffset: 14}},
			{Surface: "un", CanonicalLemma: "un", UPOS: "DET", Location: analyzer.SourceLocation{StartOffset: 15, EndOffset: 17}},
			{Surface: "ragazzo", CanonicalLemma: "ragazzo", UPOS: "NOUN", Location: analyzer.SourceLocation{StartOffset: 18, EndOffset: 25}},
			{Surface: "in", CanonicalLemma: "in", UPOS: "ADP", Location: analyzer.SourceLocation{StartOffset: 26, EndOffset: 28}},
			{Surface: "gamba", CanonicalLemma: "gamba", UPOS: "NOUN", Location: analyzer.SourceLocation{StartOffset: 29, EndOffset: 34}},
			{Surface: "oggi", CanonicalLemma: "oggi", UPOS: "ADV", Location: analyzer.SourceLocation{StartOffset: 35, EndOffset: 39}},
		}},
		1: {Text: literalGambaSentence, Tokens: []analyzer.Token{
			{Surface: "Mi", CanonicalLemma: "io", UPOS: "PRON", Location: analyzer.SourceLocation{StartOffset: 100, EndOffset: 102}},
			{Surface: "fa", CanonicalLemma: "fare", UPOS: "VERB", Location: analyzer.SourceLocation{StartOffset: 103, EndOffset: 105}},
			{Surface: "male", CanonicalLemma: "male", UPOS: "ADV", Location: analyzer.SourceLocation{StartOffset: 106, EndOffset: 110}},
			{Surface: "la", CanonicalLemma: "la", UPOS: "DET", Location: analyzer.SourceLocation{StartOffset: 111, EndOffset: 113}},
			{Surface: "gamba", CanonicalLemma: "gamba", UPOS: "NOUN", Location: analyzer.SourceLocation{StartOffset: 114, EndOffset: 119}},
			{Surface: "oggi", CanonicalLemma: "oggi", UPOS: "ADV", Location: analyzer.SourceLocation{StartOffset: 120, EndOffset: 124}},
		}, Location: analyzer.SourceLocation{StartOffset: 100, EndOffset: 125}},
	}
	if a.onlyIdiom {
		candidate.OccurrenceCount = 1
		candidate.SentenceReferences = []byte(`[{"sentence_index":0,"location":{"start_offset":10}}]`)
		delete(sentences, 1)
	}
	projection := cardexport.CandidateProjection{
		OwnerID: prep.OwnerID, DeckName: "Italian Book",
		Candidate: candidate,
		Entry: cardexport.Entry{
			OwnerID: prep.OwnerID, Language: "it", CanonicalLemma: "gamba",
			UPOS: "NOUN", SourceDocument: "Italian Book", TargetWord: "gamba",
		},
		Sentences: sentences,
		Provider:  "llm", ProviderVersion: "prompt-v1", TargetLanguage: "en",
	}
	return []cardexport.CandidateProjection{projection}, "Italian Book", nil
}

// TestPreparedDeckLemmaCardMovesOffAnIdiomSentence pins the leading ranking rule
// through the prepared-deck planner: with Italian's switch on, gamba's card
// moves from the higher-scoring in gamba sentence to the literal one.
func TestPreparedDeckLemmaCardMovesOffAnIdiomSentence(t *testing.T) {
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "fixture-model", BaseURL: "https://api.openai.com/v1"})
	require.NoError(t, err)
	preparation := domain.DeckPreparation{ID: "preparation", OwnerID: "alice", SourceMaterialID: "book"}

	planFor := func(assembler inputAssembler, presentation *cardexport.Presentation) string {
		planner := NewStandardPlanner(assembler, presentation, codec, true, BatchConfig{}, PreparedDeckConfig{TranslationMode: "standard", StandardMaxAttempts: 1})
		plan, planErr := planner.PlanPreparedDeckRun(context.Background(), nil, preparation)
		require.NoError(t, planErr)
		require.Len(t, plan.Projection.Items, 1)
		require.True(t, plan.Projection.Items[0].Quality.Accepted)
		return plan.Projection.Items[0].Entry.Sentence
	}

	baseline := planFor(idiomLemmaAssembler{}, cardexport.NewPresentation(idiomLemmaDictionary{}))
	assert.Equal(t, idiomLemmaSentence, baseline, "with the switch off, today's higher-scoring choice is unchanged")

	withIdioms := planFor(idiomLemmaAssembler{}, cardexport.NewPresentationWithIdiomCards(idiomLemmaDictionary{}, []string{"it"}))
	assert.Equal(t, literalGambaSentence, withIdioms)
}

// TestPreparedDeckLemmaCardKeepsAnIdiomOnlySentence pins the fallback: when the
// target is inside an idiom in every accepted sentence, today's choice stands
// and the later lemma_mismatch path is unchanged.
func TestPreparedDeckLemmaCardKeepsAnIdiomOnlySentence(t *testing.T) {
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "fixture-model", BaseURL: "https://api.openai.com/v1"})
	require.NoError(t, err)
	preparation := domain.DeckPreparation{ID: "preparation", OwnerID: "alice", SourceMaterialID: "book"}
	assembler := idiomLemmaAssembler{onlyIdiom: true}

	planFor := func(presentation *cardexport.Presentation) string {
		planner := NewStandardPlanner(assembler, presentation, codec, true, BatchConfig{}, PreparedDeckConfig{TranslationMode: "standard", StandardMaxAttempts: 1})
		plan, planErr := planner.PlanPreparedDeckRun(context.Background(), nil, preparation)
		require.NoError(t, planErr)
		require.Len(t, plan.Projection.Items, 1)
		require.True(t, plan.Projection.Items[0].Quality.Accepted)
		return plan.Projection.Items[0].Entry.Sentence
	}

	baseline := planFor(cardexport.NewPresentation(idiomLemmaDictionary{}))
	withIdioms := planFor(cardexport.NewPresentationWithIdiomCards(idiomLemmaDictionary{}, []string{"it"}))
	t.Run("the only accepted sentence is the idiom sentence", func(t *testing.T) {
		require.Equal(t, idiomLemmaSentence, baseline)
	})
	t.Run("the switch on keeps the same sentence", func(t *testing.T) {
		require.Equal(t, baseline, withIdioms)
	})
}
