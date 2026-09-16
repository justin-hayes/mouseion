package prepareddeck

import (
	"bytes"
	"context"
	"log"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type cutoverAssembler struct {
	projections []cardexport.CandidateProjection
	deckName    string
}

type inputFactsStore struct {
	facts          persistence.PreparedDeckInputFacts
	candidateFacts []persistence.PreparedDeckCandidateFacts
	requested      []domain.SelectionCandidate
}

type plannerDictionary struct{}

func (plannerDictionary) Name() string    { return "dictionary" }
func (plannerDictionary) Version() string { return "dictionary-v1" }
func (plannerDictionary) Lookup(context.Context, enrichment.LexicalLookupRequest) (enrichment.LexicalEntry, bool, error) {
	return enrichment.LexicalEntry{Senses: []enrichment.LexicalSense{{Gloss: "house"}}}, true, nil
}

func (s inputFactsStore) LoadPreparedDeckInputFactsTx(context.Context, pgx.Tx, domain.DeckPreparation) (persistence.PreparedDeckInputFacts, error) {
	facts := s.facts
	for _, fact := range s.candidateFacts {
		facts.Candidates = append(facts.Candidates, fact.Candidate)
	}
	return facts, nil
}

func (s *inputFactsStore) LoadPreparedDeckCandidateFactsTx(_ context.Context, _ pgx.Tx, _ domain.DeckPreparation, selected []domain.SelectionCandidate) ([]persistence.PreparedDeckCandidateFacts, error) {
	s.requested = selected
	result := make([]persistence.PreparedDeckCandidateFacts, 0, len(selected))
	for _, wanted := range selected {
		for _, fact := range s.candidateFacts {
			if fact.Candidate.CanonicalLemma == wanted.CanonicalLemma && fact.Candidate.UPOS == wanted.UPOS {
				result = append(result, fact)
				break
			}
		}
	}
	return result, nil
}

func (a *cutoverAssembler) AssemblePreparedDeckInputs(context.Context, pgx.Tx, domain.DeckPreparation) ([]cardexport.CandidateProjection, string, error) {
	return a.projections, a.deckName, nil
}

func cutoverProjection() cardexport.CandidateProjection {
	return cardexport.CandidateProjection{
		OwnerID: "alice", DeckName: "Book",
		Candidate: domain.SelectionCandidate{OwnerID: "alice", CorpusID: "corpus", Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", ObservedForms: []byte(`["Haus"]`), SentenceReferences: []byte(`[{"sentence_index":0,"location":{"start_offset":8}}]`), FirstEncounter: 10, OccurrenceCount: 3},
		Entry:     cardexport.Entry{OwnerID: "alice", Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", SourceDocument: "Book"},
		Sentences: map[int64]analyzer.Sentence{0: {Text: "Das alte Haus ist überraschend groß.", Tokens: []analyzer.Token{{Surface: "Das", UPOS: "DET", Dependency: "det", Head: 2}, {Surface: "alte", UPOS: "ADJ", Dependency: "amod", Head: 2}, {Surface: "Haus", UPOS: "NOUN", Dependency: "nsubj", Head: 3}, {Surface: "ist", UPOS: "VERB", Dependency: "root", Head: 3, Morphology: map[string]string{"VerbForm": "Fin"}}, {Surface: "überraschend", UPOS: "ADV", Dependency: "advmod", Head: 5}, {Surface: "groß", UPOS: "ADJ", Dependency: "xcomp", Head: 3}}}},
	}
}

func TestBatchPlannerKeepsDisabledAndNoConsentRunsProviderFree(t *testing.T) {
	assembler := &cutoverAssembler{projections: []cardexport.CandidateProjection{cutoverProjection()}, deckName: "Book"}
	cases := []struct {
		name, wantProvider string
		enabled, consent   bool
	}{
		{name: "disabled", enabled: false, consent: true},
		{name: "no consent", enabled: true, consent: false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			plan, err := NewBatchPlanner(assembler, cardexport.NewPresentation(nil), nil, test.enabled, BatchConfig{}).PlanPreparedDeckRun(context.Background(), nil, domain.DeckPreparation{ID: "preparation", OwnerID: "alice", SourceMaterialID: "book"}, test.consent)
			require.NoError(t, err)
			assert.False(t, plan.Config.ExternalTranslationConfigured)
			assert.Equal(t, test.wantProvider, plan.Config.Provider)
			assert.Len(t, plan.Chunks, 0)
			assert.Nil(t, plan.Projection.Items[0].CacheKey, "disabled/no-consent projection has a cache identity")
		})
	}
}

func TestBatchPlannerKeepsNoConsentDictionaryDeterministic(t *testing.T) {
	assembler := &cutoverAssembler{projections: []cardexport.CandidateProjection{cutoverProjection()}, deckName: "Book"}
	plan, err := NewBatchPlanner(assembler, cardexport.NewPresentation(plannerDictionary{}), nil, true, BatchConfig{}).PlanPreparedDeckRun(context.Background(), nil, domain.DeckPreparation{ID: "preparation", OwnerID: "alice", SourceMaterialID: "book"}, false)

	require.NoError(t, err)
	require.Len(t, plan.Projection.Items, 1)
	assert.Equal(t, "house", plan.Projection.Items[0].Entry.Gloss)
	assert.Nil(t, plan.Projection.Items[0].CacheKey)
	assert.False(t, plan.Config.ExternalTranslationConfigured)
}

func TestBatchPlannerBuildsExactEligibleBatchContract(t *testing.T) {
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "gpt-test", BaseURL: "https://api.openai.com/v1"})
	require.NoError(t, err)
	assembler := &cutoverAssembler{projections: []cardexport.CandidateProjection{cutoverProjection()}, deckName: "Book"}
	plan, err := NewBatchPlanner(assembler, cardexport.NewPresentation(nil), codec, true, BatchConfig{MaxRequests: 1}).PlanPreparedDeckRun(context.Background(), nil, domain.DeckPreparation{ID: "preparation", OwnerID: "alice", SourceMaterialID: "book"}, true)
	require.NoError(t, err)
	assert.True(t, plan.Config.ExternalTranslationConfigured)
	assert.Equal(t, enrichment.OpenAIChatCompletionsEndpoint, plan.Config.Endpoint)
	assert.Equal(t, codec.Model(), plan.Config.Model)
	assert.Equal(t, codec.ProviderName(), plan.Config.Provider)
	assert.Equal(t, codec.ProviderVersion(), plan.Config.ProviderVersion)
	require.Len(t, plan.Chunks, 1)
	require.Len(t, plan.Chunks[0].Ordinals, 1)
	assert.Equal(t, 1, plan.Chunks[0].Generation)
	assert.Equal(t, "run", plan.Chunks[0].SplitReason)
	require.NotNil(t, plan.Projection.Items[0].CacheKey)
	assert.Equal(t, enrichment.SentenceHash("Das alte Haus ist überraschend groß."), plan.Projection.Items[0].CacheKey.SentenceHash)
	assert.NotEmpty(t, plan.RunID)
	assert.Equal(t, 1, plan.Config.BatchMaxRequests)
	assert.Equal(t, persistence.DefaultBatchMaxBytes, plan.Config.BatchMaxBytes)
}

func TestPreparedDeckPlannerDefaultsToStandardWithoutBatchChunks(t *testing.T) {
	assembler := &cutoverAssembler{projections: []cardexport.CandidateProjection{cutoverProjection()}, deckName: "Book"}
	planner := NewPreparedDeckPlanner(assembler, cardexport.NewPresentation(nil), nil, false, BatchConfig{}, PreparedDeckConfig{TranslationMode: DefaultTranslationMode})
	plan, err := planner.PlanPreparedDeckRun(context.Background(), nil, domain.DeckPreparation{ID: "preparation", OwnerID: "alice", SourceMaterialID: "book"}, true)
	require.NoError(t, err)
	assert.Equal(t, string(domain.PreparedDeckExecutionStandard), plan.Config.ExecutionMode)
	assert.Len(t, plan.Chunks, 0)
	assert.False(t, plan.Config.ExternalTranslationConfigured)
}

func TestPreparedDeckPlannerLogsGlossCoverageAtFreezeSeam(t *testing.T) {
	var output bytes.Buffer
	previousWriter, previousFlags := log.Writer(), log.Flags()
	log.SetOutput(&output)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
	}()

	assembler := &cutoverAssembler{projections: []cardexport.CandidateProjection{cutoverProjection()}, deckName: "Book"}
	_, err := NewBatchPlanner(assembler, cardexport.NewPresentation(nil), nil, false, BatchConfig{}).PlanPreparedDeckRun(context.Background(), nil, domain.DeckPreparation{ID: "preparation", OwnerID: "alice", SourceMaterialID: "book"}, false)
	require.NoError(t, err)
	assert.Contains(t, output.String(), `gloss_coverage {"event":"gloss_coverage","groups":[{"language":"de","pos":"NOUN","selected":1,"with_gloss":0,"without_gloss":1}]}`)
}

func TestPreparedDeckPlannerPreservesEmptyGlossCoverageEvent(t *testing.T) {
	var output bytes.Buffer
	previousWriter, previousFlags := log.Writer(), log.Flags()
	log.SetOutput(&output)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
	}()

	assembler := &cutoverAssembler{deckName: "Book"}
	_, err := NewBatchPlanner(assembler, cardexport.NewPresentation(nil), nil, false, BatchConfig{}).PlanPreparedDeckRun(context.Background(), nil, domain.DeckPreparation{ID: "preparation", OwnerID: "alice", SourceMaterialID: "book"}, false)
	require.NoError(t, err)
	assert.Contains(t, output.String(), `gloss_coverage {"event":"gloss_coverage","groups":[]}`)
}

func TestInputAssemblerSelectsRecurringUnknownVocabularyFromFacts(t *testing.T) {
	projection := cutoverProjection()
	makeFact := func(lemma string, occurrences int) persistence.PreparedDeckCandidateFacts {
		candidate := projection.Candidate
		candidate.CanonicalLemma = lemma
		candidate.OccurrenceCount = occurrences
		entry := projection.Entry
		entry.CanonicalLemma = lemma
		return persistence.PreparedDeckCandidateFacts{Candidate: candidate, Entry: entry, Sentences: projection.Sentences}
	}
	book := "book"
	facts := persistence.PreparedDeckInputFacts{
		DeckName:  "Book",
		Known:     []domain.KnownVocabulary{{Language: "de", CanonicalLemma: "known", UPOS: "NOUN"}},
		Generated: []domain.GeneratedVocabulary{{Language: "de", CanonicalLemma: "generated", UPOS: "NOUN", FirstSourceMaterialID: &book}},
		Reserved:  []domain.DeckPreparationVocabulary{{Language: "de", CanonicalLemma: "reserved", UPOS: "NOUN"}},
	}

	candidateFacts := []persistence.PreparedDeckCandidateFacts{
		makeFact("keep", 3), makeFact("rare", 2), makeFact("known", 5), makeFact("generated", 5), makeFact("reserved", 5),
	}
	store := inputFactsStore{facts: facts, candidateFacts: candidateFacts}
	projections, deckName, err := NewInputAssembler(&store).AssemblePreparedDeckInputs(context.Background(), nil, domain.DeckPreparation{OwnerID: "alice", SourceMaterialID: book})

	require.NoError(t, err)
	assert.Equal(t, "Book", deckName)
	require.Len(t, projections, 2)
	assert.Equal(t, "keep", projections[0].Candidate.CanonicalLemma)
	assert.Equal(t, "generated", projections[1].Candidate.CanonicalLemma)
	assert.Equal(t, []string{"keep", "generated"}, []string{store.requested[0].CanonicalLemma, store.requested[1].CanonicalLemma})
}
