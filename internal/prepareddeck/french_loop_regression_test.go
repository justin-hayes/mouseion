package prepareddeck

import (
	"context"
	"encoding/json"
	"maps"
	"os"
	"testing"

	mouseionv1 "github.com/justin-hayes/mouseion/gen/go/mouseion/v1"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/checked"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/selection"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/encoding/protojson"
)

// frenchLoopStore is an in-memory selection store that records the Known and
// Reserved lookups a French preparation makes, so the regression can assert
// that card generation never writes Known vocabulary.
type frenchLoopStore struct {
	known    map[string]bool
	reserved map[string]bool
	saved    []domain.SelectionCandidate
}

func frenchLoopKey(owner, language, lemma, upos string) string {
	return owner + "\x00" + language + "\x00" + lemma + "\x00" + upos
}

func (s *frenchLoopStore) IsKnownVocabularyIdentity(_ context.Context, owner, language, lemma, upos string) (bool, error) {
	return s.known[frenchLoopKey(owner, language, lemma, upos)], nil
}

func (s *frenchLoopStore) IsReservedVocabulary(_ context.Context, owner, language, lemma, upos string) (bool, error) {
	return s.reserved[frenchLoopKey(owner, language, lemma, upos)], nil
}

func (s *frenchLoopStore) PutSelectionCandidate(_ context.Context, candidate domain.SelectionCandidate) (bool, error) {
	s.saved = append(s.saved, candidate)
	return true, nil
}

// frenchLoopDictionary is a deterministic local lexical provider: every French
// identity resolves to one gloss and no morphology, so the analyzer's own
// morphology is what reaches the card.
type frenchLoopDictionary struct{}

func (frenchLoopDictionary) Name() string    { return "fixture-dictionary" }
func (frenchLoopDictionary) Version() string { return "fixture-fr-v1" }
func (frenchLoopDictionary) Lookup(_ context.Context, request enrichment.LexicalLookupRequest) (enrichment.LexicalEntry, bool, error) {
	return enrichment.LexicalEntry{
		Senses:          []enrichment.LexicalSense{{Gloss: "gloss for " + request.CanonicalLemma}},
		CandidateSenses: []enrichment.LexicalSense{{Gloss: "gloss for " + request.CanonicalLemma}},
	}, true, nil
}

// TestFrenchProductLoopRegressionFixture proves the whole French product loop
// from the committed analyzer fixture — analysis to selection to a prepared
// deck to an APKG — without a live model or database.
func TestFrenchProductLoopRegressionFixture(t *testing.T) {
	corpus := loadFrenchFixtureCorpus(t)
	require.Equal(t, "fr", corpus.Language)
	assert.Equal(t, "french", corpus.NormalizationProfile.Name, "the French profile identifies the analysis")

	// aller is already Known for this learner in French. Selection must honour it
	// and card generation must not add to it.
	known := map[string]bool{frenchLoopKey("alice", "fr", "aller", "VERB"): true}
	store := &frenchLoopStore{known: known, reserved: map[string]bool{}}
	candidates, err := selection.NewService(store).Select(t.Context(), "alice", corpus, selection.DefaultConfig("french-corpus"))
	require.NoError(t, err)
	require.NotEmpty(t, candidates)
	for _, candidate := range candidates {
		assert.NotEqual(t, "aller", candidate.Identity.CanonicalLemma, "Known vocabulary is excluded from selection")
	}

	projections := frenchLoopProjections(t, "alice", "Le Petit Prince", corpus, candidates)
	require.NotEmpty(t, projections)

	presentation := cardexport.NewPresentation(frenchLoopDictionary{})
	deck, _, err := presentation.Freeze(t.Context(), "alice", "Le Petit Prince", projections)
	require.NoError(t, err)

	work := deck.WorkProjection()
	require.NotEmpty(t, work)
	for _, item := range work {
		// The provider request names the language French through its ISO code,
		// which the enrichment codec validates and echoes.
		assert.Equal(t, "fr", item.Request.Language)
	}

	results := make([]cardexport.StoredResult, 0, len(work))
	for _, item := range work {
		results = append(results, cardexport.StoredResult{
			CacheKey: item.CacheKey,
			Record: enrichment.CacheEntry{
				CacheKey: item.CacheKey, Translation: "translation for " + item.Request.CanonicalLemma,
				SentenceTranslation: "A complete English sentence.",
			},
		})
	}

	artifact, _, err := cardexport.NewPresentation(frenchLoopDictionary{}).Finalize(t.Context(), deck, results, cardexport.RunFacts{
		Consent: true, Configured: true, ExecutionMode: "standard", TargetLanguage: "en", Provider: "llm", ProviderVersion: "prompt-fr",
	})
	require.NoError(t, err)
	require.NotEmpty(t, artifact.Generated)

	assert.Equal(t, "Mouseion::fr::Le Petit Prince", artifact.DeckName)
	assert.NotEmpty(t, artifact.APKG)
	assert.Contains(t, artifact.TSV, "lang::fr")
	for _, generated := range artifact.Generated {
		assert.Contains(t, generated.Note.Tags, "lang::fr")
		assert.Equal(t, "fr", generated.Input.Language)
	}

	// The analyzer's own French morphology reaches the card: a consonant-initial
	// masculine noun takes "le", and an unknown-h noun appears bare with its
	// gender rather than risking a wrong article.
	byLemma := make(map[string]cardexport.Note, len(artifact.Generated))
	for _, generated := range artifact.Generated {
		byLemma[generated.Note.Lemma] = generated.Note
	}
	marché, ok := byLemma["marché"]
	require.True(t, ok, "the French loop must export the selected noun marché: %v", byLemma)
	assert.Equal(t, "le", marché.Article)
	assert.Empty(t, marché.Gender)

	homme, ok := byLemma["homme"]
	require.True(t, ok, "the French loop must export the selected noun homme: %v", byLemma)
	assert.Empty(t, homme.Article, "an unknown-h noun must not risk a wrong article")
	assert.Equal(t, "m.", homme.Gender)

	// Generating cards is provenance only: the preparation never writes Known
	// or Reserved vocabulary, so the seeded Known set is byte-for-byte unchanged.
	assert.Equal(t, known, store.known, "card generation must not change Known vocabulary")
	assert.Empty(t, store.reserved)
	require.NotEmpty(t, store.saved)
	for _, candidate := range store.saved {
		assert.Equal(t, "fr", candidate.Language)
	}
}

// loadFrenchFixtureCorpus reads the committed French Stanza expected-corpus
// fixture and converts it to the project-owned analyzer result.
func loadFrenchFixtureCorpus(t *testing.T) analyzer.Result {
	t.Helper()
	payload, err := os.ReadFile("../../nlp/tests/testdata/french_stanza_expected.json")
	require.NoError(t, err)
	corpus := new(mouseionv1.NormalizedCorpus)
	require.NoError(t, protojson.Unmarshal(payload, corpus))
	result, err := analyzer.FromProto(corpus)
	require.NoError(t, err)
	return result
}

// frenchLoopProjections materializes the prepared-deck projections selection
// would persist: each candidate's representative sentence, target and analyzer
// morphology, plus the corpus sentences the renderer anchors against.
func frenchLoopProjections(t *testing.T, owner, deckName string, corpus analyzer.Result, candidates []selection.Candidate) []cardexport.CandidateProjection {
	t.Helper()
	persisted := make(map[int64]analyzer.Sentence, len(corpus.Sentences))
	for index, sentence := range corpus.Sentences {
		persisted[int64(index)] = sentence
	}
	projections := make([]cardexport.CandidateProjection, 0, len(candidates))
	for _, candidate := range candidates {
		forms, err := json.Marshal(candidate.ObservedForms)
		require.NoError(t, err)
		references, err := json.Marshal(candidate.SentenceReferences)
		require.NoError(t, err)
		provenance, err := json.Marshal(candidate.Provenance)
		require.NoError(t, err)
		firstEncounter := int64(-1)
		for _, reference := range candidate.SentenceReferences {
			offset, err := checked.Int64FromUint64(reference.Location.StartOffset)
			require.NoError(t, err, "fixture source offset must fit int64")
			if firstEncounter < 0 || offset < firstEncounter {
				firstEncounter = offset
			}
		}
		entry, ok := frenchLoopEntry(corpus, candidate)
		if !ok {
			t.Fatalf("no analyzer sentence resolved target for %s", candidate.Identity.CanonicalLemma)
		}
		projections = append(projections, cardexport.CandidateProjection{
			OwnerID: owner, DeckName: deckName,
			Candidate: domain.SelectionCandidate{
				OwnerID: owner, CorpusID: "french-corpus",
				Language: candidate.Identity.Language, CanonicalLemma: candidate.Identity.CanonicalLemma, UPOS: candidate.Identity.UPOS,
				OccurrenceCount: candidate.OccurrenceCount, FirstEncounter: firstEncounter,
				ObservedForms: forms, SentenceReferences: references, Provenance: provenance,
			},
			Entry: entry, Sentences: persisted,
			Provider: "llm", ProviderVersion: "prompt-fr", TargetLanguage: "en",
		})
	}
	return projections
}

// frenchLoopEntry builds the minimal frozen entry for one selected candidate
// from the corpus token that realizes it.
func frenchLoopEntry(corpus analyzer.Result, candidate selection.Candidate) (cardexport.Entry, bool) {
	for _, reference := range candidate.SentenceReferences {
		if reference.SentenceIndex < 0 || reference.SentenceIndex >= len(corpus.Sentences) {
			continue
		}
		sentence := corpus.Sentences[reference.SentenceIndex]
		for _, token := range sentence.Tokens {
			if token.CanonicalLemma != candidate.Identity.CanonicalLemma || token.UPOS != candidate.Identity.UPOS {
				continue
			}
			morphology, err := json.Marshal(cloneMorphology(token.Morphology))
			if err != nil {
				return cardexport.Entry{}, false
			}
			return cardexport.Entry{
				Language:       candidate.Identity.Language,
				CanonicalLemma: candidate.Identity.CanonicalLemma, UPOS: candidate.Identity.UPOS,
				Sentence: sentence.Text, TargetWord: token.Surface, Morphology: string(morphology),
				SentenceTokens: sentence.Tokens, SentenceStartOffset: sentence.Location.StartOffset,
				SourceDocument: "Le Petit Prince",
			}, true
		}
	}
	return cardexport.Entry{}, false
}

// cloneMorphology returns a non-nil copy of a token's morphology map so a nil
// analyzer morphology encodes as an empty object rather than JSON null.
func cloneMorphology(morphology map[string]string) map[string]string {
	if cloned := maps.Clone(morphology); cloned != nil {
		return cloned
	}
	return map[string]string{}
}
