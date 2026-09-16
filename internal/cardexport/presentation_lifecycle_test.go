package cardexport_test

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type lifecycleLexicalProvider struct{}

func (lifecycleLexicalProvider) Name() string    { return "dictionary" }
func (lifecycleLexicalProvider) Version() string { return "dictionary-v1" }
func (lifecycleLexicalProvider) Lookup(context.Context, enrichment.LexicalLookupRequest) (enrichment.LexicalEntry, bool, error) {
	return enrichment.LexicalEntry{
		Senses:          []enrichment.LexicalSense{{Gloss: "house", Examples: []string{"a house"}}, {Gloss: "building"}},
		CandidateSenses: []enrichment.LexicalSense{{Gloss: "house", Examples: []string{"a house"}}, {Gloss: "building"}},
		Plural:          "Häuser",
	}, true, nil
}

func TestPresentationLifecycleFreezesProjectsAndFinalizes(t *testing.T) {
	presentation := cardexport.NewPresentation(lifecycleLexicalProvider{})
	deck, diagnostics, err := presentation.Freeze(context.Background(), []cardexport.CandidateProjection{lifecycleProjection()})
	require.NoError(t, err)
	assert.Empty(t, diagnostics.DegradationCodes)
	assert.Equal(t, 1, deck.Summary().Completeness.TotalCards)
	assert.Equal(t, 1, deck.Summary().Accepted)
	assert.Equal(t, cardexport.ManifestSchemaVersion, deck.StorageProjection().SchemaVersion)

	work := deck.WorkProjection()
	require.Len(t, work, 1)
	assert.Equal(t, "en", work[0].CacheKey.TargetLanguage)
	assert.Equal(t, "dictionary-v1", work[0].DictionaryProviderVersion)
	result := enrichment.Result{
		Candidate:           work[0].RequestCandidate(),
		Translation:         enrichment.Field[string]{Value: "house", Available: true, Provenance: enrichment.Provenance{Provider: "llm", ProviderVersion: "prompt-v1"}},
		SentenceTranslation: enrichment.Field[string]{Value: "The house is old today.", Available: true, Provenance: enrichment.Provenance{Provider: "llm", ProviderVersion: "prompt-v1"}},
		SenseSelection:      enrichment.Field[[]int]{Value: []int{1, 0}, Available: true, Provenance: enrichment.Provenance{Provider: "llm", ProviderVersion: "prompt-v1"}},
	}
	artifact, finalDiagnostics, err := presentation.Finalize(context.Background(), deck, []cardexport.StoredResult{{CacheKey: work[0].CacheKey, Result: result}}, cardexport.RunFacts{Consent: true, Configured: true, ExecutionMode: "standard", TargetLanguage: "en", Provider: "llm", ProviderVersion: "prompt-v1"})
	require.NoError(t, err)
	assert.Empty(t, finalDiagnostics.DegradationCodes)
	assert.Equal(t, "building · house", artifact.Generated[0].Note.Gloss)
	assert.Contains(t, artifact.TSV, "The house is old today.")
}

func TestPresentationLifecycleRestoresEveryManifestSchemaAndDigest(t *testing.T) {
	for schema := cardexport.LegacyManifestSchemaVersion; schema <= cardexport.ManifestSchemaVersion; schema++ {
		key := &enrichment.CacheKey{Language: "de", TargetLanguage: "en", CanonicalLemma: "haus", UPOS: "NOUN", Provider: "llm", ProviderVersion: "prompt-v1", SentenceHash: enrichment.SentenceHash("Das Haus steht heute dort.")}
		if schema == cardexport.LegacyManifestSchemaVersion {
			key.TargetLanguage = ""
		}
		snapshot := cardexport.ManifestSnapshot{
			SchemaVersion: schema, Owner: "owner-1", DeckName: "Book", Filename: cardexport.DownloadFilename("Book"),
			Items: []cardexport.ManifestItem{{Ordinal: 0, Disposition: cardexport.ManifestAccepted,
				Entry:   cardexport.Entry{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", Sentence: "Das Haus steht heute dort.", TargetWord: "Haus", Gloss: "frozen gloss"},
				Quality: cardexport.SentenceQuality{Accepted: true, Score: 12, Reasons: []string{"target present"}}, CacheKey: key}},
		}
		manifestDigest, err := snapshot.Digest()
		require.NoError(t, err, "schema %d", schema)
		candidateDigest, err := cardexport.CandidateDigestVersion(snapshot.Items[0], schema)
		require.NoError(t, err, "schema %d", schema)
		deck, err := cardexport.NewPresentation(nil).Restore(snapshot)
		require.NoError(t, err, "schema %d", schema)
		stored := deck.StorageProjection()
		gotManifestDigest, err := stored.Digest()
		require.NoError(t, err, "schema %d", schema)
		gotCandidateDigest, err := cardexport.CandidateDigestVersion(stored.Items[0], schema)
		require.NoError(t, err, "schema %d", schema)
		assert.Equal(t, schema, stored.SchemaVersion)
		assert.Equal(t, manifestDigest, gotManifestDigest, "schema %d", schema)
		assert.Equal(t, candidateDigest, gotCandidateDigest, "schema %d", schema)
		assert.Equal(t, "frozen gloss", stored.Items[0].Entry.Gloss)
	}
}

func TestPresentationLifecycleFinalizesBatchWithMissingOptionalResults(t *testing.T) {
	deck, _, err := cardexport.NewPresentation(lifecycleLexicalProvider{}).Freeze(context.Background(), []cardexport.CandidateProjection{lifecycleProjection()})
	require.NoError(t, err)
	artifact, diagnostics, err := cardexport.NewPresentation(lifecycleLexicalProvider{}).Finalize(context.Background(), deck, nil, cardexport.RunFacts{Consent: true, Configured: true, ExecutionMode: "batch", TargetLanguage: "en", Provider: "llm", ProviderVersion: "prompt-v1"})
	require.NoError(t, err)
	assert.Equal(t, 1, artifact.Count)
	assert.Equal(t, "house · building", artifact.Generated[0].Note.Gloss)
	assert.Empty(t, diagnostics.DegradationCodes)
}

func TestPresentationLifecycleRequiresCompleteStandardResults(t *testing.T) {
	deck, _, err := cardexport.NewPresentation(lifecycleLexicalProvider{}).Freeze(context.Background(), []cardexport.CandidateProjection{lifecycleProjection()})
	require.NoError(t, err)
	_, _, err = cardexport.NewPresentation(lifecycleLexicalProvider{}).Finalize(context.Background(), deck, nil, cardexport.RunFacts{Consent: true, Configured: true, ExecutionMode: "standard", TargetLanguage: "en", Provider: "llm", ProviderVersion: "prompt-v1"})
	assert.ErrorIs(t, err, cardexport.ErrInvalidInput)
}

func TestPresentationLifecycleReportsFallbackGlossAndRejectsWrongCandidate(t *testing.T) {
	deck, _, err := cardexport.NewPresentation(lifecycleLexicalProvider{}).Freeze(context.Background(), []cardexport.CandidateProjection{lifecycleProjection()})
	require.NoError(t, err)
	work := deck.WorkProjection()
	result := enrichment.Result{
		Candidate:      work[0].RequestCandidate(),
		SenseSelection: enrichment.Field[[]int]{Value: []int{}, Available: true, Provenance: enrichment.Provenance{Provider: "llm", ProviderVersion: "prompt-v1"}},
		FallbackGloss:  enrichment.Field[string]{Value: "a contextual house", Available: true, Provenance: enrichment.Provenance{Provider: "llm", ProviderVersion: "prompt-v1"}},
	}
	artifact, diagnostics, err := cardexport.NewPresentation(lifecycleLexicalProvider{}).Finalize(context.Background(), deck, []cardexport.StoredResult{{CacheKey: work[0].CacheKey, Result: result}}, cardexport.RunFacts{Consent: true, Configured: true, ExecutionMode: "batch", TargetLanguage: "en", Provider: "llm", ProviderVersion: "prompt-v1"})
	require.NoError(t, err)
	assert.Equal(t, "a contextual house", artifact.Generated[0].Note.Gloss)
	assert.Contains(t, diagnostics.DegradationCodes, cardexport.DegradationFallbackGlossApplied)

	wrong := work[0].RequestCandidate()
	wrong.CanonicalLemma = "other"
	_, _, err = cardexport.NewPresentation(lifecycleLexicalProvider{}).Finalize(context.Background(), deck, []cardexport.StoredResult{{CacheKey: work[0].CacheKey, Result: enrichment.Result{Candidate: wrong}}}, cardexport.RunFacts{Consent: true, Configured: true, ExecutionMode: "batch", TargetLanguage: "en", Provider: "llm", ProviderVersion: "prompt-v1"})
	assert.ErrorIs(t, err, cardexport.ErrInvalidInput)
}

func TestPresentationLifecycleProjectionsAreDefensive(t *testing.T) {
	deck, _, err := cardexport.NewPresentation(lifecycleLexicalProvider{}).Freeze(context.Background(), []cardexport.CandidateProjection{lifecycleProjection()})
	require.NoError(t, err)
	first := deck.StorageProjection()
	first.Items[0].Entry.CandidateSenses[0].Examples[0] = "changed"
	first.Items[0].Entry.SentenceTokens[0].Morphology["Changed"] = "true"
	first.Items[0].Quality.Reasons[0] = "changed"
	second := deck.StorageProjection()
	assert.NotEqual(t, "changed", second.Items[0].Quality.Reasons[0])
	assert.NotEqual(t, "changed", second.Items[0].Entry.CandidateSenses[0].Examples[0])
	assert.NotEqual(t, "true", second.Items[0].Entry.SentenceTokens[0].Morphology["Changed"])
	work := deck.WorkProjection()
	work[0].Request.CandidateSenses[0].Gloss = "changed"
	assert.NotEqual(t, "changed", deck.WorkProjection()[0].Request.CandidateSenses[0].Gloss)
}

func lifecycleProjection() cardexport.CandidateProjection {
	refs, _ := json.Marshal([]struct {
		SentenceIndex int              `json:"sentence_index"`
		Text          string           `json:"text"`
		Location      map[string]int64 `json:"location"`
	}{{SentenceIndex: 0, Text: "stale", Location: map[string]int64{"start_offset": 2}}})
	return cardexport.CandidateProjection{
		OwnerID: "owner-1", DeckName: "Book",
		Candidate: domain.SelectionCandidate{OwnerID: "owner-1", CorpusID: "corpus-1", Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", ObservedForms: []byte(`["Haus"]`), SentenceReferences: refs},
		Entry:     cardexport.Entry{OwnerID: "owner-1", Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", SourceDocument: "Book"},
		Sentences: map[int64]analyzer.Sentence{0: {Text: "Das alte Haus steht heute ganz ruhig dort.", Tokens: []analyzer.Token{
			{Surface: "Das", UPOS: "DET", Dependency: "det", Head: 2, Morphology: map[string]string{}},
			{Surface: "alte", UPOS: "ADJ", Dependency: "amod", Head: 2},
			{Surface: "Haus", UPOS: "NOUN", Dependency: "nsubj", Head: 3},
			{Surface: "steht", UPOS: "VERB", Dependency: "root", Head: 3, Morphology: map[string]string{"VerbForm": "Fin"}},
			{Surface: "heute", UPOS: "ADV", Dependency: "advmod", Head: 3},
			{Surface: "ganz", UPOS: "ADV", Dependency: "advmod", Head: 3},
			{Surface: "ruhig", UPOS: "ADJ", Dependency: "xcomp", Head: 3},
			{Surface: "dort", UPOS: "ADV", Dependency: "advmod", Head: 3},
		}}},
		Provider: "llm", ProviderVersion: "prompt-v1", TargetLanguage: "en",
	}
}
