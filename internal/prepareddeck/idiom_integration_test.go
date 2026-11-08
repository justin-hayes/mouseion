//go:build integration

package prepareddeck

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/idiom"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// integrationIdiomLexicon is a lexical provider that also exposes a small
// dictionary idiom lookup, so the tracer test runs without deriving an index.
type integrationIdiomLexicon struct {
	entry idiom.Entry
}

func (integrationIdiomLexicon) Name() string    { return "fixture" }
func (integrationIdiomLexicon) Version() string { return "fixture-v1" }
func (integrationIdiomLexicon) Lookup(context.Context, enrichment.LexicalLookupRequest) (enrichment.LexicalEntry, bool, error) {
	return enrichment.LexicalEntry{}, false, nil
}
func (l integrationIdiomLexicon) IdiomEntries(_ context.Context, language, word string) ([]idiom.Entry, error) {
	if language != "it" || (word != "in" && word != "gamba") {
		return nil, nil
	}
	return []idiom.Entry{l.entry}, nil
}

func integrationGambaSentence(start uint64) (string, []analyzer.Token) {
	words := []string{"È", "davvero", "un", "ragazzo", "in", "gamba."}
	lemmas := []string{"è", "davvero", "un", "ragazzo", "in", "gamba"}
	upos := []string{"AUX", "ADV", "DET", "NOUN", "ADP", "NOUN"}
	tokens := make([]analyzer.Token, len(words))
	offset := start
	for i := range words {
		tokens[i] = analyzer.Token{
			Surface: words[i], CanonicalLemma: lemmas[i], UPOS: upos[i],
			Location: analyzer.SourceLocation{StartOffset: offset, EndOffset: offset + uint64(len(words[i]))},
		}
		offset += uint64(len(words[i])) + 1
	}
	return "È davvero un ragazzo in gamba.", tokens
}

func integrationIdiomProjection(t *testing.T) cardexport.CandidateProjection {
	t.Helper()
	sentence, tokens := integrationGambaSentence(100)
	observed, err := json.Marshal([]string{"gamba"})
	require.NoError(t, err)
	return cardexport.CandidateProjection{
		OwnerID: "", DeckName: "Libro",
		Candidate: domain.SelectionCandidate{
			OwnerID: "", CorpusID: "corpus-1", Language: "it",
			CanonicalLemma: "gamba", UPOS: "NOUN", OccurrenceCount: 3, FirstEncounter: 100,
			ObservedForms: observed,
		},
		Entry: cardexport.Entry{
			Language: "it", CanonicalLemma: "gamba", UPOS: "NOUN",
			CorpusID: "corpus-1", SentenceOrdinal: 1, Sentence: sentence, TargetWord: "gamba",
			SentenceTokens: tokens, SentenceStartOffset: 100, FirstEncounter: 100,
			SourceDocument: "Libro",
		},
		Sentences: map[int64]analyzer.Sentence{1: {Text: sentence, Tokens: tokens, Location: analyzer.SourceLocation{StartOffset: 100}}},
	}
}

func TestIdiomCardsRunFromFreezeToAnkiWithoutGeneratedVocabulary(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	url, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	owner, err := store.CreateUser(ctx, "idiom-"+uuid.NewString(), false)
	require.NoError(t, err)
	source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "it", SourceIdentifier: uuid.NewString(), Title: "Libro", MediaType: "text/plain", ContentHash: uuid.NewString(), Content: []byte("testo"), FullText: "testo"})
	require.NoError(t, err)
	prep, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: source.ID, Filename: cardexport.DownloadFilename("Libro"), DeckName: "Libro", ContentHash: source.ContentHash})
	require.NoError(t, err)

	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "integration-model"})
	require.NoError(t, err)
	lexicon := integrationIdiomLexicon{entry: idiom.Entry{
		Language: "it", Headword: "in gamba", UPOS: "ADJ", Words: []string{"in", "gamba"},
		Senses: []enrichment.LexicalSense{{EvidenceID: "wikt:capable", Gloss: "capable, on the ball"}},
		Source: "wiktionary", Version: "fixture-v1",
	}}
	projection := integrationIdiomProjection(t)
	projection.OwnerID = owner.ID
	projection.Candidate.OwnerID = owner.ID
	projection.Candidate.CorpusID = uuid.NewString()
	projection.Entry.CorpusID = projection.Candidate.CorpusID
	projection.Provider = codec.ProviderName()
	projection.ProviderVersion = codec.ContextualGlossProviderVersion()
	projection.TargetLanguage = "en"
	projection.RequireContextualGloss = true
	presentation := cardexport.NewPresentation(lexicon)
	deck, _, err := presentation.Freeze(ctx, owner.ID, "Libro", []cardexport.CandidateProjection{projection}, cardexport.WithIdiomCards([]string{"it"}, codec.IdiomCardProviderVersion()))
	require.NoError(t, err)
	require.Len(t, deck.StorageProjection().Items, 2, "the lemma and the idiom item freeze together")
	work := deck.WorkProjection()
	require.Len(t, work, 2)
	require.True(t, work[1].Request.RequireIdiomCard)

	provider := &barrierTranslationProvider{}
	provider.onCall = func(_ int, request enrichment.TranslationRequest) (enrichment.TranslationResponse, error) {
		if request.RequireIdiomCard {
			return enrichment.TranslationResponse{Translation: "capable", Gloss: "capable", ContextOnly: true, SentenceTranslation: "He is really a capable young man.", SentenceTranslationTargets: []string{"capable young man"}}, nil
		}
		return enrichment.TranslationResponse{Translation: "leg", Gloss: "leg", ContextOnly: true, SentenceTranslation: "He is really a boy with a good leg.", SentenceTranslationTargets: []string{"leg"}}, nil
	}
	workers := river.NewWorkers()
	client, err := river.NewClient(riverpgxv5.New(store.Pool()), &river.Config{Queues: map[string]river.QueueConfig{Queue: {MaxWorkers: 1}, TranslationQueue: {MaxWorkers: 2}}, Workers: workers})
	require.NoError(t, err)
	AddStandardTranslationWorkerWithDependencies(workers, store, client, provider, PreparedDeckConfig{StandardRetryBaseDelay: time.Millisecond, StandardRetryMaxDelay: time.Millisecond}, time.Second)
	river.AddWorker(workers, &integrationFinalizeWorker{})
	config := persistence.PreparedDeckRunConfig{ExternalTranslationConsent: true, ExternalTranslationConfigured: true, ExecutionMode: string(domain.PreparedDeckExecutionStandard), TargetLanguage: "en", ContextMode: string(enrichment.SentenceContext), Provider: codec.ProviderName(), ProviderVersion: codec.ContextualGlossProviderVersion(), MaxProviderAttempts: 1}
	config.Endpoint = enrichment.OpenAIChatCompletionsEndpoint
	config.Model = codec.Model()
	runID := uuid.NewString()
	coordinator := NewDurableCoordinator(store, client, fixedStandardPlanner{params: persistence.FreezePreparedDeckRunParams{RunID: runID, Projection: deck.StorageProjection(), Config: config}})
	result, err := coordinator.Freeze(ctx, DurableFreezeRequest{OwnerID: owner.ID, PreparationID: prep.ID})
	require.NoError(t, err)
	run := standardIntegrationRun{store: store, owner: owner.ID, prep: prep, run: result.Run, keys: []enrichment.CacheKey{work[0].CacheKey, work[1].CacheKey}}
	require.NoError(t, client.Start(ctx))
	testutil.StopOnCleanup(t, "River client", client.Stop)
	outcomes := waitStandardOutcomes(t, ctx, run, 2)
	for _, outcome := range outcomes {
		assert.Equal(t, domain.PreparedDeckOutcomeCompleted, outcome.State)
	}
	_, err = (&DurableFinalizer{Store: store, Renderer: cardexport.NewPresentation(nil)}).Finalize(ctx, owner.ID, prep.ID, runID, 0)
	require.NoError(t, err)

	status, err := store.GetDeckPreparationStatus(ctx, owner.ID, prep.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.DeckPreparationReady, status.State)
	assert.Equal(t, 2, status.TotalCards)
	assert.Equal(t, 1, status.VocabularyCards)
	assert.Equal(t, 1, status.IdiomCards)

	var generated int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM generated_vocabulary WHERE owner_id=$1`, owner.ID).Scan(&generated))
	assert.Equal(t, 1, generated, "the idiom card records no Generated vocabulary")
	var assigned int
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT count(*) FROM deck_preparation_vocabulary WHERE owner_id=$1 AND deck_preparation_id=$2`, owner.ID, prep.ID).Scan(&assigned))
	assert.Equal(t, 1, assigned, "the idiom card is not part of the deck's vocabulary assignment")

	finalRun, err := store.GetPreparedDeckRun(ctx, owner.ID, prep.ID, runID)
	require.NoError(t, err)
	frozen, stored, err := store.LoadPreparedDeckFinalization(ctx, owner.ID, prep.ID, runID)
	require.NoError(t, err)
	finalDeck, err := cardexport.NewPresentation(nil).Restore(frozen)
	require.NoError(t, err)
	artifact, _, err := cardexport.NewPresentation(nil).Finalize(ctx, finalDeck, stored, preparedDeckRunFacts(finalRun))
	require.NoError(t, err)
	require.Len(t, artifact.Generated, 2)
	note := artifact.Generated[1].Note
	assert.Equal(t, cardexport.IdiomDedupKey("it", "in gamba", owner.ID), note.Key)
	assert.Contains(t, note.Tags, "mouseion::idiom")
	assert.Equal(t, "È davvero un ragazzo <b>in</b> <b>gamba</b>.", note.Text)
	assert.Equal(t, "idiom", note.POS)
	assert.Equal(t, "capable", note.Gloss)
	assert.Empty(t, note.Article)
	assert.Empty(t, note.Gender)

	// A presentation-only rerender reads the same frozen idiom evidence.
	rerenderer := &DurableRerenderer{Store: store, Renderer: cardexport.NewPresentation(nil)}
	_, err = rerenderer.Rerender(ctx, owner.ID, prep.ID, runID, cardexport.PresentationVersion+1)
	require.NoError(t, err)
	rerenderedFrozen, rerenderedStored, err := store.LoadPreparedDeckFinalization(ctx, owner.ID, prep.ID, runID)
	require.NoError(t, err)
	rerenderedDeck, err := cardexport.NewPresentation(nil).Restore(rerenderedFrozen)
	require.NoError(t, err)
	rerendered, _, err := cardexport.NewPresentation(nil).Finalize(ctx, rerenderedDeck, rerenderedStored, preparedDeckRunFacts(finalRun))
	require.NoError(t, err)
	require.Len(t, rerendered.Generated, 2)
	assert.Equal(t, note, rerendered.Generated[1].Note)
}
