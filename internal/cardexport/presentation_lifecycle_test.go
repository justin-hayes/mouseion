package cardexport

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analyzer"
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
	projection := lifecycleProjection()
	presentation := NewPresentation(lifecycleLexicalProvider{})

	deck, diagnostics, err := presentation.Freeze(context.Background(), []CandidateProjection{projection})
	require.NoError(t, err)
	assert.Empty(t, diagnostics.DegradationCodes)
	assert.Equal(t, 1, deck.Summary().Completeness.TotalCards)
	assert.Equal(t, 1, deck.Summary().Accepted)
	assert.Equal(t, ManifestSchemaVersion, deck.StorageProjection().SchemaVersion)

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
	artifact, finalDiagnostics, err := presentation.Finalize(context.Background(), deck, []StoredResult{{CacheKey: work[0].CacheKey, Result: result}}, RunFacts{Consent: true, Configured: true, ExecutionMode: "standard", Provider: "llm", ProviderVersion: "prompt-v1"})
	require.NoError(t, err)
	assert.Empty(t, finalDiagnostics.DegradationCodes)
	assert.Equal(t, "building · house", artifact.Generated[0].Note.Gloss)
	assert.Contains(t, artifact.TSV, "The house is old today.")
}

func TestPresentationLifecycleRestoresWithoutReevaluatingFrozenQuality(t *testing.T) {
	snapshot := ManifestSnapshot{
		SchemaVersion: ManifestSchemaVersionV6,
		Owner:         "owner-1",
		DeckName:      "Book",
		Filename:      DownloadFilename("Book"),
		Items: []ManifestItem{{
			Ordinal: 0, Disposition: ManifestAccepted,
			Entry:   Entry{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", Sentence: "Das Haus steht heute dort.", TargetWord: "Haus", Gloss: "frozen gloss"},
			Quality: SentenceQuality{Accepted: true, Score: 12, Reasons: []string{"target present"}},
		}},
	}

	deck, err := NewPresentation(nil).Restore(snapshot)
	require.NoError(t, err)
	assert.Equal(t, 12, deck.StorageProjection().Items[0].Quality.Score)
	assert.Equal(t, "frozen gloss", deck.StorageProjection().Items[0].Entry.Gloss)
	assert.Empty(t, deck.Diagnostics().DegradationCodes)
}

func TestPresentationLifecycleRestoresEveryManifestSchema(t *testing.T) {
	for schema := LegacyManifestSchemaVersion; schema <= ManifestSchemaVersion; schema++ {
		snapshot := ManifestSnapshot{
			SchemaVersion: schema,
			Owner:         "owner-1",
			DeckName:      "Book",
			Filename:      DownloadFilename("Book"),
			Items: []ManifestItem{{
				Ordinal: 0, Disposition: ManifestAccepted,
				Entry:   Entry{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", Sentence: "Das Haus steht heute dort.", TargetWord: "Haus"},
				Quality: SentenceQuality{Accepted: true, Score: 12, Reasons: []string{"target present"}},
			}},
		}
		deck, err := NewPresentation(nil).Restore(snapshot)
		require.NoError(t, err, "schema %d", schema)
		assert.Equal(t, schema, deck.StorageProjection().SchemaVersion)
	}
}

func TestPresentationLifecycleFinalizesBatchWithMissingOptionalResults(t *testing.T) {
	projection := lifecycleProjection()
	deck, _, err := NewPresentation(lifecycleLexicalProvider{}).Freeze(context.Background(), []CandidateProjection{projection})
	require.NoError(t, err)

	artifact, diagnostics, err := NewPresentation(lifecycleLexicalProvider{}).Finalize(context.Background(), deck, nil, RunFacts{Consent: true, Configured: true, ExecutionMode: "batch", Provider: "llm", ProviderVersion: "prompt-v1"})
	require.NoError(t, err)
	assert.Equal(t, 1, artifact.Count)
	assert.Equal(t, "house · building", artifact.Generated[0].Note.Gloss)
	assert.Empty(t, diagnostics.DegradationCodes)
}

func TestPresentationLifecycleRequiresCompleteStandardResults(t *testing.T) {
	projection := lifecycleProjection()
	deck, _, err := NewPresentation(lifecycleLexicalProvider{}).Freeze(context.Background(), []CandidateProjection{projection})
	require.NoError(t, err)

	_, _, err = NewPresentation(lifecycleLexicalProvider{}).Finalize(context.Background(), deck, nil, RunFacts{Consent: true, Configured: true, ExecutionMode: "standard", Provider: "llm", ProviderVersion: "prompt-v1"})
	assert.ErrorIs(t, err, ErrInvalidInput)
}

func TestPresentationLifecycleReportsFallbackGlossDegradation(t *testing.T) {
	projection := lifecycleProjection()
	deck, _, err := NewPresentation(lifecycleLexicalProvider{}).Freeze(context.Background(), []CandidateProjection{projection})
	require.NoError(t, err)
	work := deck.WorkProjection()
	result := enrichment.Result{
		Candidate:      work[0].RequestCandidate(),
		SenseSelection: enrichment.Field[[]int]{Value: []int{}, Available: true, Provenance: enrichment.Provenance{Provider: "llm", ProviderVersion: "prompt-v1"}},
		FallbackGloss:  enrichment.Field[string]{Value: "a contextual house", Available: true, Provenance: enrichment.Provenance{Provider: "llm", ProviderVersion: "prompt-v1"}},
	}

	artifact, diagnostics, err := NewPresentation(lifecycleLexicalProvider{}).Finalize(context.Background(), deck, []StoredResult{{CacheKey: work[0].CacheKey, Result: result}}, RunFacts{Consent: true, Configured: true, ExecutionMode: "batch", Provider: "llm", ProviderVersion: "prompt-v1"})
	require.NoError(t, err)
	assert.Equal(t, "a contextual house", artifact.Generated[0].Note.Gloss)
	assert.Contains(t, diagnostics.DegradationCodes, "fallback_gloss_applied")
}

func TestPresentationLifecycleProjectionsAreDefensive(t *testing.T) {
	projection := lifecycleProjection()
	deck, _, err := NewPresentation(lifecycleLexicalProvider{}).Freeze(context.Background(), []CandidateProjection{projection})
	require.NoError(t, err)

	first := deck.StorageProjection()
	first.Items[0].Entry.CandidateSenses[0].Examples = []string{"changed"}
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

func lifecycleProjection() CandidateProjection {
	refs, _ := json.Marshal([]struct {
		SentenceIndex int              `json:"sentence_index"`
		Text          string           `json:"text"`
		Location      map[string]int64 `json:"location"`
	}{{SentenceIndex: 0, Text: "stale", Location: map[string]int64{"start_offset": 2}}})
	return CandidateProjection{
		OwnerID: "owner-1", DeckName: "Book",
		Candidate: domain.SelectionCandidate{OwnerID: "owner-1", CorpusID: "corpus-1", Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", ObservedForms: []byte(`["Haus"]`), SentenceReferences: refs},
		Entry:     Entry{OwnerID: "owner-1", Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", SourceDocument: "Book"},
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
