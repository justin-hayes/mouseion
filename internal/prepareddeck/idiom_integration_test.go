//go:build integration

package prepareddeck

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
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

// integrationIdiomCatalog is a lexical provider exposing only a frozen set of
// dictionary idiom entries. It lets an integration test run the real matcher
// without deriving the dictionary index.
type integrationIdiomCatalog struct {
	entries []idiom.Entry
}

func (integrationIdiomCatalog) Name() string    { return "fixture" }
func (integrationIdiomCatalog) Version() string { return "fixture-v1" }
func (integrationIdiomCatalog) Lookup(context.Context, enrichment.LexicalLookupRequest) (enrichment.LexicalEntry, bool, error) {
	return enrichment.LexicalEntry{}, false, nil
}
func (c integrationIdiomCatalog) IdiomEntries(_ context.Context, language, word string) ([]idiom.Entry, error) {
	result := make([]idiom.Entry, 0, 1)
	for _, entry := range c.entries {
		if entry.Language != language {
			continue
		}
		if slices.Contains(entry.Words, word) {
			result = append(result, entry)
		}
	}
	return result, nil
}

func integrationTagliareCordaSentence(start uint64) (string, []analyzer.Token) {
	words := []string{"Tagliò", "la", "corda", "del", "pacco", "con", "il", "coltello", "affilato."}
	lemmas := []string{"tagliare", "la", "corda", "del", "pacco", "con", "il", "coltello", "affilato"}
	upos := []string{"VERB", "DET", "NOUN", "ADP", "NOUN", "ADP", "DET", "NOUN", "ADJ"}
	tokens := make([]analyzer.Token, len(words))
	offset := start
	for i := range words {
		tokens[i] = analyzer.Token{
			Surface: words[i], CanonicalLemma: lemmas[i], UPOS: upos[i],
			Location: analyzer.SourceLocation{StartOffset: offset, EndOffset: offset + uint64(len(words[i]))},
		}
		offset += uint64(len(words[i])) + 1
	}
	return "Tagliò la corda del pacco con il coltello affilato.", tokens
}

// integrationTagliareCordaProjections builds the two frozen vocabulary
// identities whose sentence contains the literal use of tagliare la corda.
func integrationTagliareCordaProjections(t *testing.T) []cardexport.CandidateProjection {
	t.Helper()
	start := uint64(100)
	sentence, tokens := integrationTagliareCordaSentence(start)
	source := analyzer.Sentence{Text: sentence, Tokens: tokens, Location: analyzer.SourceLocation{StartOffset: start}}
	projections := make([]cardexport.CandidateProjection, 0, 2)
	for _, identity := range []struct{ lemma, upos, target string }{
		{lemma: "corda", upos: "NOUN", target: "corda"},
		{lemma: "pacco", upos: "NOUN", target: "pacco"},
	} {
		observed, err := json.Marshal([]string{identity.lemma})
		require.NoError(t, err)
		references, err := json.Marshal([]struct {
			SentenceIndex int    `json:"sentence_index"`
			Text          string `json:"text"`
			Location      struct {
				StartOffset uint64 `json:"start_offset"`
			} `json:"location"`
		}{{SentenceIndex: 1, Text: sentence, Location: struct {
			StartOffset uint64 `json:"start_offset"`
		}{StartOffset: start}}})
		require.NoError(t, err)
		projections = append(projections, cardexport.CandidateProjection{
			DeckName: "Libro",
			Candidate: domain.SelectionCandidate{
				Language: "it", CanonicalLemma: identity.lemma, UPOS: identity.upos,
				OccurrenceCount: 3, FirstEncounter: int64(start), ObservedForms: observed,
				SentenceReferences: references,
			},
			Entry: cardexport.Entry{
				Language: "it", CanonicalLemma: identity.lemma, UPOS: identity.upos,
				SentenceOrdinal: 1, Sentence: sentence, TargetWord: identity.target,
				SentenceTokens: tokens, SentenceStartOffset: start, FirstEncounter: int64(start),
				SourceDocument: "Libro",
			},
			Sentences: map[int64]analyzer.Sentence{1: source},
		})
	}
	return projections
}

// newIntegrationIdiomRun freezes the projections with idiom cards enabled,
// starts the durable standard run, and returns the run plus its provider. The
// caller supplies the provider callback and drives or waits for execution.
func newIntegrationIdiomRun(t *testing.T, ctx context.Context, projections []cardexport.CandidateProjection, lexicon enrichment.LexicalProvider, codec *enrichment.TranslationCodec, maxAttempts int) (standardIntegrationRun, *river.Client[pgx.Tx], *barrierTranslationProvider) {
	t.Helper()
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
	corpusID := uuid.NewString()
	for i := range projections {
		projections[i].OwnerID = owner.ID
		projections[i].Candidate.OwnerID = owner.ID
		projections[i].Candidate.CorpusID = corpusID
		projections[i].Entry.CorpusID = corpusID
		projections[i].Provider = codec.ProviderName()
		projections[i].ProviderVersion = codec.ContextualGlossProviderVersion()
		projections[i].TargetLanguage = "en"
		projections[i].RequireContextualGloss = true
	}
	deck, _, err := cardexport.NewPresentation(lexicon).Freeze(ctx, owner.ID, "Libro", projections, cardexport.WithIdiomCards([]string{"it"}, codec.IdiomCardProviderVersion()))
	require.NoError(t, err)
	work := deck.WorkProjection()
	require.Len(t, work, len(projections)+1, "each vocabulary identity and the one idiom freeze together")
	keys := make([]enrichment.CacheKey, len(work))
	for i, item := range work {
		keys[i] = item.CacheKey
	}
	provider := &barrierTranslationProvider{}
	workers := river.NewWorkers()
	client, err := river.NewClient(riverpgxv5.New(store.Pool()), &river.Config{Queues: map[string]river.QueueConfig{Queue: {MaxWorkers: 1}, TranslationQueue: {MaxWorkers: 2}}, Workers: workers})
	require.NoError(t, err)
	AddStandardTranslationWorkerWithDependencies(workers, store, client, provider, PreparedDeckConfig{StandardRetryBaseDelay: time.Millisecond, StandardRetryMaxDelay: time.Millisecond}, time.Second)
	river.AddWorker(workers, &integrationFinalizeWorker{})
	config := persistence.PreparedDeckRunConfig{ExternalTranslationConsent: true, ExternalTranslationConfigured: true, ExecutionMode: string(domain.PreparedDeckExecutionStandard), TargetLanguage: "en", ContextMode: string(enrichment.SentenceContext), Provider: codec.ProviderName(), ProviderVersion: codec.ContextualGlossProviderVersion(), MaxProviderAttempts: maxAttempts}
	config.Endpoint = enrichment.OpenAIChatCompletionsEndpoint
	config.Model = codec.Model()
	runID := uuid.NewString()
	coordinator := NewDurableCoordinator(store, client, fixedStandardPlanner{params: persistence.FreezePreparedDeckRunParams{RunID: runID, Projection: deck.StorageProjection(), Config: config}})
	result, err := coordinator.Freeze(ctx, DurableFreezeRequest{OwnerID: owner.ID, PreparationID: prep.ID})
	require.NoError(t, err)
	return standardIntegrationRun{store: store, owner: owner.ID, prep: prep, run: result.Run, keys: keys}, client, provider
}

func integrationTagliareCordaLexicon() integrationIdiomCatalog {
	return integrationIdiomCatalog{entries: []idiom.Entry{{
		Language: "it", Headword: "tagliare la corda", UPOS: "VERB", Words: []string{"tagliare", "la", "corda"},
		Senses: []enrichment.LexicalSense{{EvidenceID: "wikt:run-away", Gloss: "to run away"}},
		Source: "wiktionary", Version: "fixture-v1",
	}}}
}

func TestIdiomLiteralUseOmitsOnlyTheIdiomCard(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "integration-model"})
	require.NoError(t, err)
	run, client, provider := newIntegrationIdiomRun(t, ctx, integrationTagliareCordaProjections(t), integrationTagliareCordaLexicon(), codec, 1)
	provider.onCall = func(_ int, request enrichment.TranslationRequest) (enrichment.TranslationResponse, error) {
		if request.RequireIdiomCard {
			return enrichment.TranslationResponse{
				Translation: "to cut the rope", ContextOnly: true,
				SentenceTranslation: "He cut the package's rope.",
				UnresolvedKind:      enrichment.ContextualGlossUnresolvedLiteralUse,
				UnresolvedReason:    "The sentence uses the words literally.",
			}, nil
		}
		return enrichment.TranslationResponse{
			Translation: request.CanonicalLemma + "-translated", Gloss: request.CanonicalLemma + "-gloss",
			ContextOnly: true, SentenceTranslation: "He cut the package's rope.",
			SentenceTranslationTargets: []string{"cut"},
		}, nil
	}
	require.NoError(t, client.Start(ctx))
	testutil.StopOnCleanup(t, "River client", client.Stop)
	outcomes := waitStandardOutcomes(t, ctx, run, len(run.keys))
	for _, outcome := range outcomes {
		assert.Equal(t, domain.PreparedDeckOutcomeCompleted, outcome.State)
	}
	currentRun, err := run.store.GetPreparedDeckRun(ctx, run.owner, run.prep.ID, run.run.ID)
	require.NoError(t, err)
	_, err = (&DurableFinalizer{Store: run.store, Renderer: cardexport.NewPresentation(nil)}).Finalize(ctx, run.owner, run.prep.ID, run.run.ID, currentRun.FinalizationDispatchGeneration)
	require.NoError(t, err)

	status, err := run.store.GetDeckPreparationStatus(ctx, run.owner, run.prep.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.DeckPreparationReady, status.State)
	assert.Equal(t, 2, status.TotalCards, "the two lemma cards remain")
	assert.Equal(t, 2, status.VocabularyCards)
	assert.Zero(t, status.IdiomCards)
	assert.Equal(t, 1, status.IdiomOmissions)
	assert.Zero(t, status.VocabularyOmissions)
	assert.Equal(t, []domain.DeckPreparationIdiomOmission{{Headword: "tagliare la corda", Reason: domain.DeckPreparationIdiomLiteralUseLabel}}, status.IdiomOmissionDetails)
	assert.Empty(t, status.MeaningOmissions, "an idiom omission is not a vocabulary omission")
	assert.Zero(t, status.LemmaMismatchOmissions)

	var flags int
	require.NoError(t, run.store.Pool().QueryRow(ctx, `SELECT count(*) FROM occurrence_lemma_review_flags WHERE owner_id=$1`, run.owner).Scan(&flags))
	assert.Zero(t, flags, "an idiom omission raises no lemma review flag")

	frozen, stored, err := run.store.LoadPreparedDeckFinalization(ctx, run.owner, run.prep.ID, run.run.ID)
	require.NoError(t, err)
	deck, err := cardexport.NewPresentation(nil).Restore(frozen)
	require.NoError(t, err)
	artifact, diagnostics, err := cardexport.NewPresentation(nil).Finalize(ctx, deck, stored, preparedDeckRunFacts(currentRun))
	require.NoError(t, err)
	require.Len(t, artifact.Generated, 2, "only the lemma cards are exported")
	for _, generated := range artifact.Generated {
		assert.NotEqual(t, "IDIOM", generated.Note.POS)
	}
	assert.Empty(t, diagnostics.MeaningOmissions, "an idiom omission is not a vocabulary meaning omission")
	assert.Equal(t, []cardexport.IdiomOmission{{Headword: "tagliare la corda", Reason: domain.DeckPreparationIdiomLiteralUseLabel}}, diagnostics.IdiomOmissions)
}

func TestOnlyOmittedIdiomCardsLeaveAVocabularyDeckReady(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "integration-model"})
	require.NoError(t, err)
	run, client, provider := newIntegrationIdiomRun(t, ctx, integrationTagliareCordaProjections(t), integrationTagliareCordaLexicon(), codec, 1)
	provider.onCall = func(_ int, request enrichment.TranslationRequest) (enrichment.TranslationResponse, error) {
		if request.RequireIdiomCard {
			return enrichment.TranslationResponse{
				Translation: "to cut the rope", ContextOnly: true, SentenceTranslation: "He cut the package's rope.",
				UnresolvedKind: enrichment.ContextualGlossUnresolvedAmbiguous, UnresolvedReason: "The sentence does not distinguish the meanings.",
			}, nil
		}
		return validTranslationResponse(request), nil
	}
	require.NoError(t, client.Start(ctx))
	testutil.StopOnCleanup(t, "River client", client.Stop)
	outcomes := waitStandardOutcomes(t, ctx, run, len(run.keys))
	for _, outcome := range outcomes {
		assert.Equal(t, domain.PreparedDeckOutcomeCompleted, outcome.State)
	}
	currentRun, err := run.store.GetPreparedDeckRun(ctx, run.owner, run.prep.ID, run.run.ID)
	require.NoError(t, err)
	_, err = (&DurableFinalizer{Store: run.store, Renderer: cardexport.NewPresentation(nil)}).Finalize(ctx, run.owner, run.prep.ID, run.run.ID, currentRun.FinalizationDispatchGeneration)
	require.NoError(t, err, "an omitted idiom must not fail a deck with vocabulary cards")
	status, err := run.store.GetDeckPreparationStatus(ctx, run.owner, run.prep.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.DeckPreparationReady, status.State)
	assert.Equal(t, 2, status.VocabularyCards)
	assert.Zero(t, status.IdiomCards)
	assert.Equal(t, 1, status.IdiomOmissions)
}

func TestOnlyIdiomCardsSucceedingIsStillAllOmitted(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "integration-model"})
	require.NoError(t, err)
	run, client, provider := newIntegrationIdiomRun(t, ctx, integrationTagliareCordaProjections(t), integrationTagliareCordaLexicon(), codec, 1)
	provider.onCall = func(_ int, request enrichment.TranslationRequest) (enrichment.TranslationResponse, error) {
		if request.RequireIdiomCard {
			return enrichment.TranslationResponse{
				Translation: "to run away", Gloss: "to run away", ContextOnly: true,
				SentenceTranslation: "He ran away.", SentenceTranslationTargets: []string{"ran away"},
			}, nil
		}
		return enrichment.TranslationResponse{
			Translation: "translated", ContextOnly: true, SentenceTranslation: "He cut the package's rope.",
			UnresolvedKind: enrichment.ContextualGlossUnresolvedAmbiguous, UnresolvedReason: "The sentence does not distinguish the meanings.",
		}, nil
	}
	require.NoError(t, client.Start(ctx))
	testutil.StopOnCleanup(t, "River client", client.Stop)
	outcomes := waitStandardOutcomes(t, ctx, run, len(run.keys))
	for _, outcome := range outcomes {
		assert.Equal(t, domain.PreparedDeckOutcomeCompleted, outcome.State)
	}
	currentRun, err := run.store.GetPreparedDeckRun(ctx, run.owner, run.prep.ID, run.run.ID)
	require.NoError(t, err)
	_, err = (&DurableFinalizer{Store: run.store, Renderer: cardexport.NewPresentation(nil)}).Finalize(ctx, run.owner, run.prep.ID, run.run.ID, currentRun.FinalizationDispatchGeneration)
	require.ErrorIs(t, err, cardexport.ErrAllMeaningsUnresolved, "idiom cards never substitute for vocabulary cards")
	status, err := run.store.GetDeckPreparationStatus(ctx, run.owner, run.prep.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.DeckPreparationFailed, status.State)
	assert.Len(t, status.MeaningOmissions, 2, "both vocabulary identities were omitted")
	assert.Empty(t, status.IdiomOmissionDetails)
}

func TestOnlyFailedIdiomItemsLeaveAVocabularyDeckReady(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "integration-model"})
	require.NoError(t, err)
	run, client, provider := newIntegrationIdiomRun(t, ctx, integrationTagliareCordaProjections(t), integrationTagliareCordaLexicon(), codec, 1)
	provider.onCall = func(_ int, request enrichment.TranslationRequest) (enrichment.TranslationResponse, error) {
		if request.RequireIdiomCard {
			return enrichment.TranslationResponse{}, &enrichment.LLMHTTPError{StatusCode: 503}
		}
		return validTranslationResponse(request), nil
	}
	require.NoError(t, client.Start(ctx))
	testutil.StopOnCleanup(t, "River client", client.Stop)
	outcomes := waitStandardOutcomes(t, ctx, run, len(run.keys))
	failed := 0
	for _, outcome := range outcomes {
		if outcome.State == domain.PreparedDeckOutcomeFailed {
			failed++
		}
	}
	assert.Equal(t, 1, failed, "only the exhausted idiom item failed")
	currentRun, err := run.store.GetPreparedDeckRun(ctx, run.owner, run.prep.ID, run.run.ID)
	require.NoError(t, err)
	_, err = (&DurableFinalizer{Store: run.store, Renderer: cardexport.NewPresentation(nil)}).Finalize(ctx, run.owner, run.prep.ID, run.run.ID, currentRun.FinalizationDispatchGeneration)
	require.NoError(t, err, "a failed idiom item must not fail a deck with vocabulary cards")
	status, err := run.store.GetDeckPreparationStatus(ctx, run.owner, run.prep.ID)
	require.NoError(t, err)
	assert.Equal(t, domain.DeckPreparationReady, status.State)
	assert.Equal(t, 2, status.TotalCards)
	assert.Equal(t, 2, status.VocabularyCards)
	assert.Zero(t, status.IdiomCards)
	assert.Zero(t, status.IdiomOmissions)
	assert.Equal(t, 1, status.ProviderOmissions, "the unusable idiom answer stays a provider omission")
}

func TestIdiomProviderFailureRetriesLikeALemmaItem(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "integration-model"})
	require.NoError(t, err)
	run, client, provider := newIntegrationIdiomRun(t, ctx, integrationTagliareCordaProjections(t), integrationTagliareCordaLexicon(), codec, 2)
	provider.onCall = func(attempt int, request enrichment.TranslationRequest) (enrichment.TranslationResponse, error) {
		if request.RequireIdiomCard {
			if attempt == 1 {
				return enrichment.TranslationResponse{}, &enrichment.LLMHTTPError{StatusCode: 503}
			}
			return enrichment.TranslationResponse{
				Translation: "to run away", Gloss: "to run away", ContextOnly: true,
				SentenceTranslation: "He ran away.", SentenceTranslationTargets: []string{"ran away"},
			}, nil
		}
		return validTranslationResponse(request), nil
	}
	require.NoError(t, client.Start(ctx))
	testutil.StopOnCleanup(t, "River client", client.Stop)
	outcomes := waitStandardOutcomes(t, ctx, run, len(run.keys))
	retried := make([]domain.PreparedDeckTranslationOutcome, 0, 1)
	for _, outcome := range outcomes {
		if outcome.ProviderAttemptCount == 2 {
			retried = append(retried, outcome)
		}
	}
	require.Len(t, retried, 1, "only the idiom item needed a retry")
	assert.Equal(t, domain.PreparedDeckOutcomeCompleted, retried[0].State, "the retried idiom item completed")
	currentRun, err := run.store.GetPreparedDeckRun(ctx, run.owner, run.prep.ID, run.run.ID)
	require.NoError(t, err)
	_, err = (&DurableFinalizer{Store: run.store, Renderer: cardexport.NewPresentation(nil)}).Finalize(ctx, run.owner, run.prep.ID, run.run.ID, currentRun.FinalizationDispatchGeneration)
	require.NoError(t, err)
	status, err := run.store.GetDeckPreparationStatus(ctx, run.owner, run.prep.ID)
	require.NoError(t, err)
	assert.Equal(t, 1, status.IdiomCards, "the recovered idiom item produced its card")
	assert.Zero(t, status.IdiomOmissions)
}
func TestIdiomBoundAndOverflowSurviveDurableFreezeAndStatus(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	url, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	owner, err := store.CreateUser(ctx, "idiom-bound-"+uuid.NewString(), false)
	require.NoError(t, err)
	source, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "it", SourceIdentifier: uuid.NewString(), Title: "Libro", MediaType: "text/plain", ContentHash: uuid.NewString(), Content: []byte("testo"), FullText: "testo"})
	require.NoError(t, err)
	prep, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{OwnerID: owner.ID, SourceMaterialID: source.ID, Filename: cardexport.DownloadFilename("Libro"), DeckName: "Libro", ContentHash: source.ContentHash})
	require.NoError(t, err)

	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "integration-model"})
	require.NoError(t, err)
	projections, lexicon := integrationIdiomBoundFixture(t, owner.ID, codec)
	deck, diagnostics, err := cardexport.NewPresentation(lexicon).Freeze(ctx, owner.ID, "Libro", projections, cardexport.WithIdiomCards([]string{"it"}, codec.IdiomCardProviderVersion()))
	require.NoError(t, err)
	const overflow = 5
	snapshot := deck.StorageProjection()

	t.Run("freeze diagnostics report the overflow", func(t *testing.T) {
		require.Equal(t, overflow, diagnostics.IdiomOverflow)
	})
	t.Run("the manifest carries the overflow", func(t *testing.T) {
		require.Equal(t, overflow, snapshot.IdiomOverflow)
	})
	t.Run("the manifest keeps the highest-priority idioms up to the bound", func(t *testing.T) {
		idioms := 0
		for _, item := range snapshot.Items {
			if item.ItemKind.IsIdiom() {
				idioms++
			}
		}
		require.Equal(t, cardexport.IdiomCardBound, idioms)
	})

	workers := river.NewWorkers()
	river.AddWorker(workers, &StandardTranslationWorker{})
	river.AddWorker(workers, &integrationFinalizeWorker{})
	client, err := river.NewClient(riverpgxv5.New(store.Pool()), &river.Config{Queues: map[string]river.QueueConfig{Queue: {MaxWorkers: 1}, TranslationQueue: {MaxWorkers: 1}}, Workers: workers})
	require.NoError(t, err)
	testutil.StopOnCleanup(t, "River client", client.Stop)
	config := persistence.PreparedDeckRunConfig{ExternalTranslationConsent: true, ExternalTranslationConfigured: true, ExecutionMode: string(domain.PreparedDeckExecutionStandard), TargetLanguage: "en", ContextMode: string(enrichment.SentenceContext), Provider: codec.ProviderName(), ProviderVersion: codec.ContextualGlossProviderVersion(), MaxProviderAttempts: 1}
	config.Endpoint = enrichment.OpenAIChatCompletionsEndpoint
	config.Model = codec.Model()
	runID := uuid.NewString()
	coordinator := NewDurableCoordinator(store, client, fixedStandardPlanner{params: persistence.FreezePreparedDeckRunParams{RunID: runID, Projection: snapshot, Config: config}})
	_, err = coordinator.Freeze(ctx, DurableFreezeRequest{OwnerID: owner.ID, PreparationID: prep.ID})
	require.NoError(t, err)

	status, err := store.GetDeckPreparationStatus(ctx, owner.ID, prep.ID)
	require.NoError(t, err)

	t.Run("the durable status carries the overflow", func(t *testing.T) {
		require.Equal(t, overflow, status.IdiomOverflow)
	})
}

type integrationBoundLexicon struct {
	entries map[string][]idiom.Entry
}

func (integrationBoundLexicon) Name() string    { return "fixture" }
func (integrationBoundLexicon) Version() string { return "fixture-v1" }
func (integrationBoundLexicon) Lookup(context.Context, enrichment.LexicalLookupRequest) (enrichment.LexicalEntry, bool, error) {
	return enrichment.LexicalEntry{}, false, nil
}
func (l integrationBoundLexicon) IdiomEntries(_ context.Context, language, word string) ([]idiom.Entry, error) {
	return l.entries[language+"\x00"+word], nil
}

func integrationIdiomBoundFixture(t *testing.T, owner string, codec *enrichment.TranslationCodec) ([]cardexport.CandidateProjection, integrationBoundLexicon) {
	t.Helper()
	const count = cardexport.IdiomCardBound + 5
	entries := make(map[string][]idiom.Entry, count*2)
	projections := make([]cardexport.CandidateProjection, 0, count)
	for i := range count {
		anchor := fmt.Sprintf("lemma%02d", i)
		other := fmt.Sprintf("word%02d", i)
		sentence, tokens := integrationIdiomBoundSentence(uint64(100+i*100), anchor, other)
		entry := idiom.Entry{
			Language: "it", Headword: anchor + " " + other, UPOS: "ADJ",
			Words:  []string{anchor, other},
			Senses: []enrichment.LexicalSense{{EvidenceID: "e-" + anchor, Gloss: "gloss for " + anchor}},
			Source: "wiktionary", Version: "fixture-v1",
		}
		entries["it\x00"+anchor] = []idiom.Entry{entry}
		entries["it\x00"+other] = []idiom.Entry{entry}
		observed, err := json.Marshal([]string{anchor})
		require.NoError(t, err)
		corpusID := uuid.NewString()
		projections = append(projections, cardexport.CandidateProjection{
			OwnerID: owner, DeckName: "Libro",
			Candidate: domain.SelectionCandidate{
				OwnerID: owner, CorpusID: corpusID, Language: "it",
				CanonicalLemma: anchor, UPOS: "NOUN", OccurrenceCount: count - i,
				FirstEncounter: int64(100 + i*100), ObservedForms: observed,
			},
			Entry: cardexport.Entry{
				OwnerID: owner, Language: "it", CanonicalLemma: anchor, UPOS: "NOUN",
				CorpusID: corpusID, SentenceOrdinal: 1, Sentence: sentence, TargetWord: anchor,
				SentenceTokens: tokens, SentenceStartOffset: uint64(100 + i*100), FirstEncounter: int64(100 + i*100),
				SourceDocument: "Libro",
			},
			Sentences: map[int64]analyzer.Sentence{1: {Text: sentence, Tokens: tokens, Location: analyzer.SourceLocation{StartOffset: uint64(100 + i*100)}}},
			Provider:  codec.ProviderName(), ProviderVersion: codec.ContextualGlossProviderVersion(),
			TargetLanguage: "en", RequireContextualGloss: true,
		})
	}
	return projections, integrationBoundLexicon{entries: entries}
}

func integrationIdiomBoundSentence(start uint64, anchor, other string) (string, []analyzer.Token) {
	words := []string{"È", "davvero", "un", anchor, other, "bello."}
	lemmas := []string{"è", "davvero", "un", anchor, other, "bello"}
	upos := []string{"AUX", "ADV", "DET", "NOUN", "NOUN", "ADJ"}
	tokens := make([]analyzer.Token, len(words))
	offset := start
	for i := range words {
		tokens[i] = analyzer.Token{
			Surface: words[i], CanonicalLemma: lemmas[i], UPOS: upos[i],
			Location: analyzer.SourceLocation{StartOffset: offset, EndOffset: offset + uint64(len(words[i]))},
		}
		offset += uint64(len(words[i])) + 1
	}
	return "È davvero un " + anchor + " " + other + " bello.", tokens
}
