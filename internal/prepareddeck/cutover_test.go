package prepareddeck

import (
	"bytes"
	"context"
	"errors"
	"log"
	"math"
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
	language    string
	languageErr error
}

type inputFactsStore struct {
	facts          persistence.PreparedDeckInputFacts
	candidateFacts []persistence.PreparedDeckCandidateFacts
	requested      []domain.SelectionCandidate
	inputErr       error
	candidateErr   error
}

// languageFactsStore adds the optional language capability to an input-facts
// store.
type languageFactsStore struct {
	*inputFactsStore
	language    string
	languageErr error
}

func (s languageFactsStore) PreparedDeckLanguageTx(context.Context, pgx.Tx, domain.DeckPreparation) (string, error) {
	return s.language, s.languageErr
}

// plainAssembler is an input assembler without the optional language
// capability.
type plainAssembler struct{}

func (plainAssembler) AssemblePreparedDeckInputs(context.Context, pgx.Tx, domain.DeckPreparation) ([]cardexport.CandidateProjection, string, error) {
	return nil, "", nil
}

type plannerDictionary struct{}

func (plannerDictionary) Name() string    { return "dictionary" }
func (plannerDictionary) Version() string { return "dictionary-v1" }
func (plannerDictionary) Lookup(context.Context, enrichment.LexicalLookupRequest) (enrichment.LexicalEntry, bool, error) {
	return enrichment.LexicalEntry{Senses: []enrichment.LexicalSense{{Gloss: "house"}}}, true, nil
}

func (s *inputFactsStore) LoadPreparedDeckInputFactsTx(context.Context, pgx.Tx, domain.DeckPreparation) (persistence.PreparedDeckInputFacts, error) {
	if s.inputErr != nil {
		return persistence.PreparedDeckInputFacts{}, s.inputErr
	}
	facts := s.facts
	for _, fact := range s.candidateFacts {
		facts.Candidates = append(facts.Candidates, fact.Candidate)
	}
	return facts, nil
}

func (s *inputFactsStore) LoadPreparedDeckCandidateFactsTx(_ context.Context, _ pgx.Tx, _ domain.DeckPreparation, selected []domain.SelectionCandidate) ([]persistence.PreparedDeckCandidateFacts, error) {
	if s.candidateErr != nil {
		return nil, s.candidateErr
	}
	s.requested = selected
	result := make([]persistence.PreparedDeckCandidateFacts, 0, len(selected))
	for _, wanted := range selected {
		for _, fact := range s.candidateFacts {
			if fact.Candidate.CanonicalLemma == wanted.CanonicalLemma && fact.Candidate.UPOS == wanted.UPOS {
				fact.Candidate = wanted
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

func (a *cutoverAssembler) DeckLanguage(context.Context, pgx.Tx, domain.DeckPreparation) (string, error) {
	return a.language, a.languageErr
}

func cutoverProjection() cardexport.CandidateProjection {
	return cardexport.CandidateProjection{
		OwnerID: "alice", DeckName: "Book",
		Candidate: domain.SelectionCandidate{OwnerID: "alice", CorpusID: "corpus", Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", ObservedForms: []byte(`["Haus"]`), SentenceReferences: []byte(`[{"sentence_index":0,"location":{"start_offset":8}}]`), FirstEncounter: 10, OccurrenceCount: 3},
		Entry:     cardexport.Entry{OwnerID: "alice", Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", SourceDocument: "Book"},
		Sentences: map[int64]analyzer.Sentence{0: {Text: "Das alte Haus ist überraschend groß.", Tokens: []analyzer.Token{{Surface: "Das", UPOS: "DET", Dependency: "det", Head: 2}, {Surface: "alte", UPOS: "ADJ", Dependency: "amod", Head: 2}, {Surface: "Haus", UPOS: "NOUN", Dependency: "nsubj", Head: 3}, {Surface: "ist", UPOS: "VERB", Dependency: "root", Head: 3, Morphology: map[string]string{"VerbForm": "Fin"}}, {Surface: "überraschend", UPOS: "ADV", Dependency: "advmod", Head: 5}, {Surface: "groß", UPOS: "ADJ", Dependency: "xcomp", Head: 3}}}},
	}
}

func TestBatchPlannerRejectsMissingProvider(t *testing.T) {
	assembler := &cutoverAssembler{projections: []cardexport.CandidateProjection{cutoverProjection()}, deckName: "Book"}
	cases := []struct {
		name    string
		enabled bool
		codec   *enrichment.TranslationCodec
	}{
		{name: "provider disabled", enabled: false},
		{name: "codec missing", enabled: true},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			_, err := NewBatchPlanner(assembler, cardexport.NewPresentation(nil), test.codec, test.enabled, BatchConfig{}).PlanPreparedDeckRun(context.Background(), nil, domain.DeckPreparation{ID: "preparation", OwnerID: "alice", SourceMaterialID: "book"})
			require.Error(t, err)
			assert.Contains(t, err.Error(), "configured translation provider")
		})
	}
}

func TestInputAssemblerFreezesOccurrenceCorrectionIntoRecurringDeckIdentity(t *testing.T) {
	corpus := analyzer.Result{Language: "de", Sentences: []analyzer.Sentence{{Text: "Ein Drache sieht zwei Drachen.", Tokens: []analyzer.Token{
		{Surface: "Drachen", RawLemma: "Drach", CanonicalLemma: "drach", UPOS: "NOUN", Location: analyzer.SourceLocation{SourceDocumentID: "unit", StartOffset: 10, EndOffset: 17}},
		{Surface: "Drachen", RawLemma: "Drache", CanonicalLemma: "drache", UPOS: "NOUN", Location: analyzer.SourceLocation{SourceDocumentID: "unit", StartOffset: 20, EndOffset: 27}},
		{Surface: "Drachen", RawLemma: "Drache", CanonicalLemma: "drache", UPOS: "NOUN", Location: analyzer.SourceLocation{SourceDocumentID: "unit", StartOffset: 30, EndOffset: 37}},
	}}}}
	store := &inputFactsStore{
		facts: persistence.PreparedDeckInputFacts{
			DeckName: "Book", CorpusID: "corpus", Analysis: &corpus,
			Corrections: []domain.OccurrenceLemmaCorrection{{SourceDocumentID: "unit", StartOffset: 10, EndOffset: 17, CanonicalLemma: "drache"}},
		},
		candidateFacts: []persistence.PreparedDeckCandidateFacts{{
			Candidate: domain.SelectionCandidate{OwnerID: "alice", CorpusID: "corpus", Language: "de", CanonicalLemma: "drache", UPOS: "NOUN"},
			Entry:     cardexport.Entry{OwnerID: "alice", Language: "de", CanonicalLemma: "drache", UPOS: "NOUN", SourceDocument: "Book"},
			Sentences: map[int64]analyzer.Sentence{0: corpus.Sentences[0]},
		}},
	}
	projections, _, err := NewInputAssembler(store).AssemblePreparedDeckInputs(context.Background(), nil, domain.DeckPreparation{OwnerID: "alice", SourceMaterialID: "book"})
	require.NoError(t, err)
	require.Len(t, projections, 1, "corrected identity merges into the two-occurrence canonical lemma and crosses the recurring floor")
	assert.Equal(t, "drache", projections[0].Candidate.CanonicalLemma)
	assert.Equal(t, 3, projections[0].Candidate.OccurrenceCount)
}

// cutoverGreekProjection is a valid Ancient Greek projection whose sentence
// passes the Greek quality gate.
func cutoverGreekProjection() cardexport.CandidateProjection {
	return cardexport.CandidateProjection{
		OwnerID: "alice", DeckName: "Book",
		Candidate: domain.SelectionCandidate{OwnerID: "alice", CorpusID: "corpus", Language: "grc", CanonicalLemma: "λόγος", UPOS: "NOUN", ObservedForms: []byte(`["λόγον"]`), SentenceReferences: []byte(`[{"sentence_index":0,"location":{"start_offset":12}}]`), FirstEncounter: 10, OccurrenceCount: 3},
		Entry:     cardexport.Entry{OwnerID: "alice", Language: "grc", CanonicalLemma: "λόγος", UPOS: "NOUN", Headword: "λόγος", SourceDocument: "Book"},
		Sentences: map[int64]analyzer.Sentence{0: {Text: "ἄνθρωπος ὁρᾷ τὸν λόγον καὶ δέω βραδέως;"}},
	}
}

func TestSourceEvidenceDeckDetectsEveryGreekCandidate(t *testing.T) {
	cases := []struct {
		name        string
		projections []cardexport.CandidateProjection
		want        bool
	}{
		{name: "no candidates is not source evidence by projection", want: false},
		{name: "an Ancient Greek candidate is source evidence", projections: []cardexport.CandidateProjection{cutoverGreekProjection()}, want: true},
		{name: "a modern candidate keeps the provider", projections: []cardexport.CandidateProjection{cutoverProjection()}, want: false},
		{name: "a mixed deck is not source evidence", projections: []cardexport.CandidateProjection{cutoverGreekProjection(), cutoverProjection()}, want: false},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			assert.Equal(t, test.want, sourceEvidenceDeck(test.projections))
		})
	}
}

func TestInputAssemblerDeckLanguage(t *testing.T) {
	var nilAssembler *InputAssembler
	cases := []struct {
		name     string
		assember *InputAssembler
		want     string
	}{
		{name: "a nil assembler reports no language", assember: nilAssembler},
		{name: "a nil store reports no language", assember: NewInputAssembler(nil)},
		{name: "a store without the capability reports no language", assember: NewInputAssembler(&inputFactsStore{})},
		{name: "a store with the capability reports its language", assember: NewInputAssembler(languageFactsStore{inputFactsStore: &inputFactsStore{}, language: "grc"}), want: "grc"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			language, err := test.assember.DeckLanguage(context.Background(), nil, domain.DeckPreparation{})
			require.NoError(t, err)
			assert.Equal(t, test.want, language)
		})
	}
}

func TestPlannerResolvesDeckLanguage(t *testing.T) {
	cases := []struct {
		name      string
		assembler inputAssembler
		want      string
		wantErr   string
	}{
		{name: "the assembler language is reported", assembler: &cutoverAssembler{language: "grc"}, want: "grc"},
		{name: "a modern language is reported", assembler: &cutoverAssembler{language: "de"}, want: "de"},
		{name: "an assembler without the capability reports no language", assembler: plainAssembler{}},
		{name: "an assembler failure is surfaced", assembler: &cutoverAssembler{languageErr: errors.New("language lookup failed")}, wantErr: "resolve prepared deck language"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			planner := NewBatchPlanner(test.assembler, cardexport.NewPresentation(nil), nil, false, BatchConfig{})
			language, err := planner.deckLanguage(context.Background(), nil, domain.DeckPreparation{})
			if test.wantErr != "" {
				require.ErrorContains(t, err, test.wantErr)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, test.want, language)
		})
	}
}

// A Greek deck is planned with no provider, no cache identity, no contextual
// Gloss and no local meaning evidence, so it freezes directly to finalization.
func TestBatchPlannerPlansProviderlessSourceEvidenceDeck(t *testing.T) {
	assembler := &cutoverAssembler{projections: []cardexport.CandidateProjection{cutoverGreekProjection()}, deckName: "Book", language: "grc"}
	plan, err := NewBatchPlanner(assembler, cardexport.NewPresentation(plannerDictionary{}), nil, false, BatchConfig{}).PlanPreparedDeckRun(context.Background(), nil, domain.DeckPreparation{ID: "preparation", OwnerID: "alice", SourceMaterialID: "book"})
	require.NoError(t, err)

	assert.False(t, plan.Config.ExternalTranslationConsent)
	assert.False(t, plan.Config.ExternalTranslationConfigured)
	assert.Empty(t, plan.Config.Provider)
	assert.Empty(t, plan.Config.ProviderVersion)
	assert.Empty(t, plan.Chunks)
	require.Len(t, plan.Projection.Items, 1)
	assert.Nil(t, plan.Projection.Items[0].CacheKey)
	assert.Empty(t, plan.Projection.Items[0].Entry.Gloss, "a source-evidence deck resolves no local dictionary entry")
	assert.Empty(t, plan.Projection.Items[0].Entry.CandidateSenses)

	frozen, err := cardexport.NewPresentation(nil).Restore(plan.Projection)
	require.NoError(t, err)
	assert.Empty(t, frozen.WorkProjection(), "a source-evidence deck freezes no external work")
}

// An Ancient Greek deck with no selected identity is still planned locally: its
// language, not its projection count, decides the provider requirement.
func TestBatchPlannerPlansEmptySourceEvidenceDeck(t *testing.T) {
	assembler := &cutoverAssembler{deckName: "Book", language: "grc"}
	plan, err := NewBatchPlanner(assembler, cardexport.NewPresentation(nil), nil, false, BatchConfig{}).PlanPreparedDeckRun(context.Background(), nil, domain.DeckPreparation{ID: "preparation", OwnerID: "alice", SourceMaterialID: "book"})
	require.NoError(t, err)
	assert.Empty(t, plan.Projection.Items)
	assert.False(t, plan.Config.ExternalTranslationConsent)
	assert.Empty(t, plan.Chunks)
}

func TestBatchPlannerSetsContextualGlossAsRequired(t *testing.T) {
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "gpt-test", BaseURL: "https://api.openai.com/v1"})
	require.NoError(t, err)
	assembler := &cutoverAssembler{projections: []cardexport.CandidateProjection{cutoverProjection()}, deckName: "Book"}
	plan, err := NewBatchPlanner(assembler, cardexport.NewPresentation(plannerDictionary{}), codec, true, BatchConfig{}).PlanPreparedDeckRun(context.Background(), nil, domain.DeckPreparation{ID: "preparation", OwnerID: "alice", SourceMaterialID: "book"})

	require.NoError(t, err)
	require.Len(t, plan.Projection.Items, 1)
	assert.Equal(t, "house", plan.Projection.Items[0].Entry.Gloss)
	assert.NotNil(t, plan.Projection.Items[0].CacheKey)
	assert.True(t, plan.Config.ExternalTranslationConsent)
	assert.True(t, plan.Config.ExternalTranslationConfigured)
	deck, err := cardexport.NewPresentation(nil).Restore(plan.Projection)
	require.NoError(t, err)
	work := deck.WorkProjection()
	require.Len(t, work, 1)
	assert.True(t, work[0].Request.RequireContextualGloss)
}

func TestBatchPlannerBuildsExactEligibleBatchContract(t *testing.T) {
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "gpt-test", BaseURL: "https://api.openai.com/v1"})
	require.NoError(t, err)
	assembler := &cutoverAssembler{projections: []cardexport.CandidateProjection{cutoverProjection()}, deckName: "Book"}
	plan, err := NewBatchPlanner(assembler, cardexport.NewPresentation(nil), codec, true, BatchConfig{MaxRequests: 1}).PlanPreparedDeckRun(context.Background(), nil, domain.DeckPreparation{ID: "preparation", OwnerID: "alice", SourceMaterialID: "book"})
	require.NoError(t, err)
	assert.True(t, plan.Config.ExternalTranslationConfigured)
	assert.Equal(t, enrichment.OpenAIChatCompletionsEndpoint, plan.Config.Endpoint)
	assert.Equal(t, codec.Model(), plan.Config.Model)
	assert.Equal(t, codec.ProviderName(), plan.Config.Provider)
	assert.Equal(t, codec.ContextualGlossProviderVersion(), plan.Config.ProviderVersion)
	require.Len(t, plan.Chunks, 1)
	require.Len(t, plan.Chunks[0].Ordinals, 1)
	assert.Equal(t, 1, plan.Chunks[0].Generation)
	assert.Equal(t, "run", plan.Chunks[0].SplitReason)
	require.NotNil(t, plan.Projection.Items[0].CacheKey)
	assert.Equal(t, enrichment.SentenceHash("Das alte Haus ist überraschend groß."), plan.Projection.Items[0].CacheKey.SentenceHash)
	assert.Equal(t, enrichment.MeaningEvidenceHash(plan.Projection.Items[0].Entry.CandidateSenses), plan.Projection.Items[0].CacheKey.MeaningEvidenceHash)
	frozen, err := cardexport.NewPresentation(nil).Restore(plan.Projection)
	require.NoError(t, err)
	work := frozen.WorkProjection()
	require.Len(t, work, 1)
	assert.True(t, work[0].Request.RequireContextualGloss)
	assert.NotEmpty(t, plan.RunID)
	assert.Equal(t, 1, plan.Config.BatchMaxRequests)
	assert.Equal(t, persistence.DefaultBatchMaxBytes, plan.Config.BatchMaxBytes)
}

func TestPreparedDeckPlannerDefaultsToStandardAndRequiresConfiguredProvider(t *testing.T) {
	assembler := &cutoverAssembler{projections: []cardexport.CandidateProjection{cutoverProjection()}, deckName: "Book"}
	planner := NewPreparedDeckPlanner(assembler, cardexport.NewPresentation(nil), nil, false, BatchConfig{}, PreparedDeckConfig{TranslationMode: DefaultTranslationMode})
	_, err := planner.PlanPreparedDeckRun(context.Background(), nil, domain.DeckPreparation{ID: "preparation", OwnerID: "alice", SourceMaterialID: "book"})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "configured translation provider")
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
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "gpt-test", BaseURL: "https://api.openai.com/v1"})
	require.NoError(t, err)
	_, err = NewBatchPlanner(assembler, cardexport.NewPresentation(nil), codec, true, BatchConfig{}).PlanPreparedDeckRun(context.Background(), nil, domain.DeckPreparation{ID: "preparation", OwnerID: "alice", SourceMaterialID: "book"})
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
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "gpt-test", BaseURL: "https://api.openai.com/v1"})
	require.NoError(t, err)
	_, err = NewBatchPlanner(assembler, cardexport.NewPresentation(nil), codec, true, BatchConfig{}).PlanPreparedDeckRun(context.Background(), nil, domain.DeckPreparation{ID: "preparation", OwnerID: "alice", SourceMaterialID: "book"})
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
	otherBook := "other-book"
	facts := persistence.PreparedDeckInputFacts{
		DeckName:  "Book",
		Known:     []domain.KnownVocabulary{{Language: "de", CanonicalLemma: "known", UPOS: "NOUN"}},
		Generated: []domain.GeneratedVocabulary{{Language: "de", CanonicalLemma: "generated", UPOS: "NOUN", FirstSourceMaterialID: &otherBook}},
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

func TestGreekInputAssemblerLeavesKnownVocabularyUnchangedUntilGraduation(t *testing.T) {
	projection := cutoverProjection()
	projection.Candidate.Language = "el"
	projection.Candidate.CanonicalLemma = "σπίτι"
	projection.Candidate.ObservedForms = []byte(`["σπίτι"]`)
	projection.Entry.Language = "el"
	projection.Entry.CanonicalLemma = "σπίτι"
	projection.Entry.SourceDocument = "Το σπίτι"
	projection.Sentences = map[int64]analyzer.Sentence{0: {Text: "Το σπίτι είναι μεγάλο.", Tokens: []analyzer.Token{{Surface: "Το", UPOS: "DET"}, {Surface: "σπίτι", UPOS: "NOUN"}, {Surface: "είναι", UPOS: "VERB"}, {Surface: "μεγάλο", UPOS: "ADJ"}}}}

	makeFact := func(lemma string, occurrences int) persistence.PreparedDeckCandidateFacts {
		candidate := projection.Candidate
		candidate.CanonicalLemma = lemma
		candidate.OccurrenceCount = occurrences
		entry := projection.Entry
		entry.CanonicalLemma = lemma
		return persistence.PreparedDeckCandidateFacts{Candidate: candidate, Entry: entry, Sentences: projection.Sentences}
	}
	otherBook := "other-greek-book"
	facts := persistence.PreparedDeckInputFacts{
		DeckName:  "Mouseion::el::Το σπίτι",
		Known:     []domain.KnownVocabulary{{Language: "el", CanonicalLemma: "γνωστό", UPOS: "NOUN"}},
		Generated: []domain.GeneratedVocabulary{{Language: "el", CanonicalLemma: "παλιό", UPOS: "ADJ", FirstSourceMaterialID: &otherBook}},
		Reserved:  []domain.DeckPreparationVocabulary{{Language: "el", CanonicalLemma: "δεσμευμένο", UPOS: "NOUN"}},
	}
	store := inputFactsStore{facts: facts, candidateFacts: []persistence.PreparedDeckCandidateFacts{
		makeFact("σπίτι", 3), makeFact("γνωστό", 5), makeFact("παλιό", 5), makeFact("δεσμευμένο", 5),
	}}
	projections, deckName, err := NewInputAssembler(&store).AssemblePreparedDeckInputs(context.Background(), nil, domain.DeckPreparation{OwnerID: "alice", SourceMaterialID: "greek-book"})

	require.NoError(t, err)
	assert.Equal(t, "Mouseion::el::Το σπίτι", deckName)
	assert.Len(t, projections, 2)
	assert.Equal(t, "σπίτι", projections[0].Candidate.CanonicalLemma)
	assert.Equal(t, "παλιό", projections[1].Candidate.CanonicalLemma)
	assert.Equal(t, "γνωστό", store.facts.Known[0].CanonicalLemma)
}

func TestInputAssemblerFailsClosedForMissingCurrentReadingSnapshot(t *testing.T) {
	store := inputFactsStore{facts: persistence.PreparedDeckInputFacts{
		DeckName:   "Book",
		Candidates: []domain.SelectionCandidate{{OwnerID: "alice", CorpusID: "corpus", Language: "de", CanonicalLemma: "mutable", UPOS: "NOUN", OccurrenceCount: 3}},
	}}
	_, _, err := NewInputAssembler(&store).AssemblePreparedDeckInputs(context.Background(), nil, domain.DeckPreparation{
		ID: "preparation", OwnerID: "alice", SourceMaterialID: "book", SnapshotID: "frozen-snapshot",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "Current reading snapshot")
}

func TestInputAssemblerSurfacesFactLoadFailures(t *testing.T) {
	inputErr := errors.New("input facts unavailable")
	candidateErr := errors.New("candidate facts unavailable")
	cases := map[string]struct {
		store *inputFactsStore
		want  error
	}{
		"input facts":     {store: &inputFactsStore{inputErr: inputErr}, want: inputErr},
		"candidate facts": {store: &inputFactsStore{candidateErr: candidateErr}, want: candidateErr},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			_, _, err := NewInputAssembler(test.store).AssemblePreparedDeckInputs(context.Background(), nil, domain.DeckPreparation{OwnerID: "alice", SourceMaterialID: "book"})
			require.ErrorIs(t, err, test.want)
		})
	}
}

func TestInputAssemblerRejectsMissingFactStore(t *testing.T) {
	_, _, err := NewInputAssembler(nil).AssemblePreparedDeckInputs(context.Background(), nil, domain.DeckPreparation{OwnerID: "alice", SourceMaterialID: "book"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "input fact store is unavailable")
}

func TestInputAssemblerSelectsFrozenCurrentReadingSnapshot(t *testing.T) {
	snapshot := domain.SelectionCandidate{OwnerID: "alice", CorpusID: "corpus", Language: "de", CanonicalLemma: "frozen", UPOS: "NOUN", OccurrenceCount: 3}
	store := &inputFactsStore{
		facts: persistence.PreparedDeckInputFacts{
			DeckName:                     "Book",
			CurrentReadingSnapshotActive: true,
			CurrentReadingSnapshot:       []domain.SelectionCandidate{snapshot},
		},
		candidateFacts: []persistence.PreparedDeckCandidateFacts{{
			Candidate: snapshot,
			Entry:     cardexport.Entry{OwnerID: "alice", Language: "de", CanonicalLemma: "frozen", UPOS: "NOUN", SourceDocument: "Book"},
		}},
	}
	projections, deckName, err := NewInputAssembler(store).AssemblePreparedDeckInputs(context.Background(), nil, domain.DeckPreparation{ID: "preparation", OwnerID: "alice", SourceMaterialID: "book", SnapshotID: "snapshot"})

	require.NoError(t, err)
	assert.Equal(t, "Book", deckName)
	require.Len(t, projections, 1)
	assert.Equal(t, "frozen", projections[0].Candidate.CanonicalLemma)
	require.NotEmpty(t, store.requested)
	assert.Equal(t, "frozen", store.requested[0].CanonicalLemma, "the frozen snapshot bypasses recurring eligibility")
}

func TestInputAssemblerRejectsInvalidCorrectionOffsets(t *testing.T) {
	store := &inputFactsStore{facts: persistence.PreparedDeckInputFacts{
		DeckName:    "Book",
		CorpusID:    "corpus",
		Analysis:    &analyzer.Result{Language: "de"},
		Corrections: []domain.OccurrenceLemmaCorrection{{SourceDocumentID: "unit", StartOffset: -1, EndOffset: 4, CanonicalLemma: "haus"}},
	}}
	_, _, err := NewInputAssembler(store).AssemblePreparedDeckInputs(context.Background(), nil, domain.DeckPreparation{OwnerID: "alice", SourceMaterialID: "book"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid source offsets")
}

func TestInputAssemblerRejectsCorpusWithoutProjectableLanguage(t *testing.T) {
	store := &inputFactsStore{facts: persistence.PreparedDeckInputFacts{
		DeckName: "Book",
		CorpusID: "corpus",
		Analysis: &analyzer.Result{},
	}}
	_, _, err := NewInputAssembler(store).AssemblePreparedDeckInputs(context.Background(), nil, domain.DeckPreparation{OwnerID: "alice", SourceMaterialID: "book"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "project effective prepared-deck vocabulary")
}

func TestInputAssemblerRejectsSourceOffsetBeyondPostgresRange(t *testing.T) {
	corpus := analyzer.Result{Language: "de", Sentences: []analyzer.Sentence{{Text: "Haus", Tokens: []analyzer.Token{{
		Surface: "Haus", CanonicalLemma: "haus", UPOS: "NOUN",
		Location: analyzer.SourceLocation{SourceDocumentID: "unit", StartOffset: math.MaxUint64, EndOffset: math.MaxUint64},
	}}}}}
	store := &inputFactsStore{facts: persistence.PreparedDeckInputFacts{DeckName: "Book", CorpusID: "corpus", Analysis: &corpus}}
	_, _, err := NewInputAssembler(store).AssemblePreparedDeckInputs(context.Background(), nil, domain.DeckPreparation{OwnerID: "alice", SourceMaterialID: "book"})

	require.Error(t, err)
	assert.Contains(t, err.Error(), "source offset exceeds PostgreSQL range")
}

// recordedPassAssessment is a recorded Ancient Greek evidence-assisted pass
// payload assessment, so a test can exercise the pass without changing the
// compiled disposition.
func recordedPassAssessment(t *testing.T, codec *enrichment.TranslationCodec) []enrichment.ProviderAssessment {
	t.Helper()
	return []enrichment.ProviderAssessment{{
		Language: "grc", Provider: codec.ProviderName(),
		PromptVersion: codec.EvidenceAssistedPassPromptContract(),
		Fields:        enrichment.EvidenceAssistedPassPayloadFields(),
		Disposition:   "recorded for this test", Ticket: "#2171", Recorded: true,
	}}
}

// Without a matching recorded assessment a Greek deck is the providerless
// default even with a provider configured: nothing is sent (ADR 0105).
func TestBatchPlannerPlansDefaultGreekDeckWithoutAssessment(t *testing.T) {
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "gpt-test", BaseURL: "https://api.openai.com/v1"})
	require.NoError(t, err)
	assembler := &cutoverAssembler{projections: []cardexport.CandidateProjection{cutoverGreekProjection()}, deckName: "Book", language: "grc"}

	plan, err := NewBatchPlanner(assembler, cardexport.NewPresentation(plannerDictionary{}), codec, true, BatchConfig{}).PlanPreparedDeckRun(context.Background(), nil, domain.DeckPreparation{ID: "preparation", OwnerID: "alice", SourceMaterialID: "book"})
	require.NoError(t, err)
	assertDefaultGreekPlan(t, plan)
}

// A recorded assessment that no longer matches the configured provider sends
// nothing: the deck falls back to the default card.
func TestBatchPlannerPlansDefaultGreekDeckWhenAssessmentDrifted(t *testing.T) {
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "gpt-test", BaseURL: "https://api.openai.com/v1"})
	require.NoError(t, err)
	assembler := &cutoverAssembler{projections: []cardexport.CandidateProjection{cutoverGreekProjection()}, deckName: "Book", language: "grc"}
	planner := NewBatchPlanner(assembler, cardexport.NewPresentation(plannerDictionary{}), codec, true, BatchConfig{})
	planner.Assessments = recordedPassAssessment(t, codec)
	planner.Assessments[0].PromptVersion = "an-older-prompt"

	plan, err := planner.PlanPreparedDeckRun(context.Background(), nil, domain.DeckPreparation{ID: "preparation", OwnerID: "alice", SourceMaterialID: "book"})
	require.NoError(t, err)
	assertDefaultGreekPlan(t, plan)
}

// With a configured provider and a matching recorded assessment, every Greek
// deck freezes a standard provider run carrying the evidence-assisted contract.
func TestBatchPlannerPlansEvidenceAssistedPassWithRecordedAssessment(t *testing.T) {
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "gpt-test", BaseURL: "https://api.openai.com/v1"})
	require.NoError(t, err)
	assembler := &cutoverAssembler{projections: []cardexport.CandidateProjection{cutoverGreekProjection()}, deckName: "Book", language: "grc"}
	planner := NewBatchPlanner(assembler, cardexport.NewPresentation(plannerDictionary{}), codec, true, BatchConfig{})
	planner.Assessments = recordedPassAssessment(t, codec)
	plan, err := planner.PlanPreparedDeckRun(context.Background(), nil, domain.DeckPreparation{ID: "preparation", OwnerID: "alice", SourceMaterialID: "book"})
	require.NoError(t, err)

	assert.True(t, plan.Config.ExternalTranslationConsent)
	assert.True(t, plan.Config.ExternalTranslationConfigured)
	assert.Equal(t, "standard", plan.Config.ExecutionMode, "the pass uses the standard executor, never the Batch omission machinery")
	assert.Equal(t, codec.EvidenceAssistedPassProviderVersion(), plan.Config.ProviderVersion)
	assert.Equal(t, "en", plan.Config.TargetLanguage)
	require.Len(t, plan.Projection.Items, 1)
	assert.NotNil(t, plan.Projection.Items[0].CacheKey)

	deck, err := cardexport.NewPresentation(nil).Restore(plan.Projection)
	require.NoError(t, err)
	work := deck.WorkProjection()
	require.Len(t, work, 1)
	assert.True(t, work[0].Request.RequireEvidenceAssistedPass)
	assert.False(t, work[0].Request.RequireContextualGloss)
}

// Without a configured provider a Greek deck is the providerless default even
// when the assessment is recorded.
func TestBatchPlannerPlansDefaultGreekDeckWithoutProvider(t *testing.T) {
	assembler := &cutoverAssembler{projections: []cardexport.CandidateProjection{cutoverGreekProjection()}, deckName: "Book", language: "grc"}
	planner := NewBatchPlanner(assembler, cardexport.NewPresentation(plannerDictionary{}), nil, false, BatchConfig{})
	planner.Assessments = []enrichment.ProviderAssessment{{Language: "grc", Recorded: true}}

	plan, err := planner.PlanPreparedDeckRun(context.Background(), nil, domain.DeckPreparation{ID: "preparation", OwnerID: "alice", SourceMaterialID: "book"})
	require.NoError(t, err)
	assertDefaultGreekPlan(t, plan)
}

// assertDefaultGreekPlan asserts a providerless source-evidence Greek plan.
func assertDefaultGreekPlan(t *testing.T, plan persistence.FreezePreparedDeckRunParams) {
	t.Helper()
	assert.False(t, plan.Config.ExternalTranslationConsent)
	assert.Empty(t, plan.Config.Provider)
	require.Len(t, plan.Projection.Items, 1)
	assert.Nil(t, plan.Projection.Items[0].CacheKey, "a default Greek item has no provider cache identity")
}
