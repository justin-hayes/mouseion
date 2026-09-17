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
	deck, diagnostics, err := presentation.Freeze(t.Context(), "owner-1", "Book", []cardexport.CandidateProjection{lifecycleProjection()})
	require.NoError(t, err)
	assert.Empty(t, diagnostics.DegradationCodes)
	assert.Equal(t, 1, deck.Summary().Completeness.TotalCards)
	assert.Equal(t, 1, deck.Summary().Accepted)
	assert.Equal(t, cardexport.ManifestSchemaVersion, deck.StorageProjection().SchemaVersion)

	work := deck.WorkProjection()
	require.Len(t, work, 1)
	assert.Equal(t, "en", work[0].CacheKey.TargetLanguage)
	assert.Equal(t, "dictionary-v1", work[0].DictionaryProviderVersion)
	record := enrichment.CacheEntry{CacheKey: work[0].CacheKey, Translation: "house", SentenceTranslation: "The house is old today.", SenseSelection: []int{1, 0}}
	artifact, finalDiagnostics, err := presentation.Finalize(t.Context(), deck, []cardexport.StoredResult{{CacheKey: work[0].CacheKey, Record: record}}, cardexport.RunFacts{Consent: true, Configured: true, ExecutionMode: "standard", TargetLanguage: "en", Provider: "llm", ProviderVersion: "prompt-v1"})
	require.NoError(t, err)
	assert.Empty(t, finalDiagnostics.DegradationCodes)
	assert.Equal(t, finalDiagnostics, artifact.Diagnostics)
	assert.Equal(t, "building · house", artifact.Generated[0].Note.Gloss)
	assert.Contains(t, artifact.TSV, "The house is old today.")
	assert.Contains(t, artifact.Generated[0].Note.Text, "<b>Haus</b>")
	assert.NotEmpty(t, artifact.APKG)
	assert.Len(t, artifact.Generated, 1)
}

func TestPresentationLifecycleFreezesEmptyLocalDeck(t *testing.T) {
	deck, diagnostics, err := cardexport.NewPresentation(nil).Freeze(t.Context(), "owner-1", "Empty Book", nil)

	require.NoError(t, err)
	assert.Empty(t, diagnostics.QualityOmissions)
	assert.Equal(t, cardexport.DownloadFilename("Empty Book"), deck.StorageProjection().Filename)
	assert.Equal(t, 0, deck.Summary().Accepted)
	assert.Empty(t, deck.WorkProjection())
}

func TestPresentationLifecycleReturnsDictionaryCoverageDiagnostics(t *testing.T) {
	projections := []cardexport.CandidateProjection{
		{OwnerID: "owner-1", DeckName: "Book", Candidate: domain.SelectionCandidate{Language: "de", UPOS: "NOUN"}, Entry: cardexport.Entry{Language: "de", UPOS: "NOUN", Sentence: "Das Haus steht dort.", TargetWord: "Haus", Gloss: "house"}},
		{OwnerID: "owner-1", DeckName: "Book", Candidate: domain.SelectionCandidate{Language: "de", UPOS: "NOUN"}, Entry: cardexport.Entry{Language: "de", UPOS: "NOUN", Sentence: "Das Fragment steht dort.", TargetWord: "Fragment"}},
		{OwnerID: "owner-1", DeckName: "Book", Candidate: domain.SelectionCandidate{Language: "it", UPOS: "VERB"}, Entry: cardexport.Entry{Language: "it", UPOS: "VERB", Sentence: "La casa sta lì.", TargetWord: "sta", Gloss: "stands"}},
	}
	_, diagnostics, err := cardexport.NewPresentation(nil).Freeze(t.Context(), "owner-1", "Book", projections)
	require.NoError(t, err)
	assert.Equal(t, []cardexport.GlossCoverage{
		{Language: "de", POS: "NOUN", Selected: 2, WithGloss: 1, WithoutGloss: 1},
		{Language: "it", POS: "VERB", Selected: 1, WithGloss: 1, WithoutGloss: 0},
	}, diagnostics.GlossCoverage)

	diagnostics.GlossCoverage[0].Selected = 99
	_, diagnostics, err = cardexport.NewPresentation(nil).Freeze(t.Context(), "owner-1", "Book", projections)
	require.NoError(t, err)
	assert.Equal(t, 2, diagnostics.GlossCoverage[0].Selected)
}

func TestPresentationLifecycleRestoresEveryManifestSchemaAndDigest(t *testing.T) {
	wantManifestDigests := map[int]string{
		1: "85cfe6352a1ae28d06519c4d94adfcbb9c01e1277ed7f24c7178e54b5f4edeba",
		2: "cd1d97d3530526b88e3c5dcab590d639b80fe1170b693d57ab3f21c43a9ec20c",
		3: "9d0ee64714e48bfaccda265690a53ce73e3ff73c7a9672a6219ef212c43b7cc7",
		4: "9cbcdf53153d35561e67e6fc0c6047d4998f9c4f54e870e48ac85edeeae909d9",
		5: "5e63d298cf4abd79295cad9e647d54bae78c8f5d9e9541c550d0efcd77b9952d",
		6: "689341753a71cd9c14609d5337668cc7503f5233ca4396f35909d75e293ea6ca",
	}
	wantCandidateDigests := map[int]string{
		1: "43552493d6d8cc96b17112bc9ee667bdd1a0379e38df85c4eb691aa6c788b1cf",
		2: "8e145364594d48f70d70131efc6921655bc3cf7830347eb93ca09c4521d8908a",
		3: "a6daea9cee8b8c3f8be0ef6d1d7fdb0c98b8fec3e04a2cf75fa7614d14aef71f",
		4: "81ac05b57e64637efb78936bfa0352f821e6222d2feda13c6eb18862aa39014f",
		5: "651de8c35431d328a63e6b6cd358dc3299ce0314c1fafb74302bbbbf28ba5698",
		6: "7f94e44ed363cbbcad66e81060a234507d3c2cb1bd2814802ea05c01721f5dc5",
	}
	for schema := cardexport.LegacyManifestSchemaVersion; schema <= cardexport.ManifestSchemaVersion; schema++ {
		snapshot := lifecycleRestoreSnapshot()
		snapshot.SchemaVersion = schema
		if schema == cardexport.LegacyManifestSchemaVersion {
			snapshot.Items[0].CacheKey.TargetLanguage = ""
		}
		manifestDigest, err := snapshot.Digest()
		require.NoError(t, err, "schema %d", schema)
		candidateDigest, err := cardexport.CandidateDigestVersion(snapshot.Items[0], schema)
		require.NoError(t, err, "schema %d", schema)
		assert.Equal(t, wantManifestDigests[schema], manifestDigest, "schema %d manifest golden", schema)
		assert.Equal(t, wantCandidateDigests[schema], candidateDigest, "schema %d candidate golden", schema)
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
		assert.Equal(t, "house", stored.Items[0].Entry.Gloss)
		if schema == cardexport.LegacyManifestSchemaVersion {
			work := deck.WorkProjection()
			_, _, err = cardexport.NewPresentation(nil).Finalize(t.Context(), deck, []cardexport.StoredResult{{CacheKey: work[0].CacheKey, Record: enrichment.CacheEntry{CacheKey: work[0].CacheKey, Translation: "house", SentenceTranslation: "The house stands there today."}}}, cardexport.RunFacts{Consent: true, Configured: true, ExecutionMode: "standard", TargetLanguage: "en", Provider: "llm", ProviderVersion: "prompt-v1"})
			require.NoError(t, err, "schema %d finalization", schema)
		}
	}
}

func TestPresentationLifecycleRestoresPersistedV1TargetLanguage(t *testing.T) {
	key := &enrichment.CacheKey{Language: "de", TargetLanguage: "en", CanonicalLemma: "haus", UPOS: "NOUN", Provider: "llm", ProviderVersion: "prompt-v1", SentenceHash: enrichment.SentenceHash("Das Haus steht heute dort.")}
	snapshot := cardexport.ManifestSnapshot{
		SchemaVersion: cardexport.LegacyManifestSchemaVersion, Owner: "owner-1", DeckName: "Book", Filename: cardexport.DownloadFilename("Book"),
		Items: []cardexport.ManifestItem{{Ordinal: 0, Disposition: cardexport.ManifestAccepted,
			Entry:   cardexport.Entry{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", Sentence: "Das Haus steht heute dort.", TargetWord: "Haus", Gloss: "frozen gloss"},
			Quality: cardexport.SentenceQuality{Accepted: true, Score: 12, Reasons: []string{"target present"}}, CacheKey: key}},
	}
	wantDigest, err := snapshot.Digest()
	require.NoError(t, err)
	deck, err := cardexport.NewPresentation(nil).Restore(snapshot)
	require.NoError(t, err)
	got := deck.StorageProjection()
	gotDigest, err := got.Digest()
	require.NoError(t, err)
	assert.Equal(t, wantDigest, gotDigest)
	work, ok := deck.WorkByOrdinal(0)
	require.True(t, ok)
	assert.Equal(t, "en", work.Request.TargetLanguage)
}

func TestPresentationLifecycleRestoresLegacyRenderInputsWithoutChangingProjection(t *testing.T) {
	sentence := "Ich stehe heute auf."
	key := &enrichment.CacheKey{Language: "de", TargetLanguage: "en", CanonicalLemma: "aufstehen", UPOS: "VERB", Provider: "llm", ProviderVersion: "prompt-v1", SentenceHash: enrichment.SentenceHash(sentence)}
	snapshot := cardexport.ManifestSnapshot{
		SchemaVersion: cardexport.ManifestSchemaVersion,
		Owner:         "owner-1",
		DeckName:      "Book",
		Filename:      cardexport.DownloadFilename("Book"),
		Items: []cardexport.ManifestItem{{
			Ordinal: 0, Disposition: cardexport.ManifestAccepted,
			Entry:   cardexport.Entry{Language: "de", CanonicalLemma: "aufstehen", UPOS: "VERB", Sentence: sentence, TargetWord: "stehe"},
			Quality: cardexport.SentenceQuality{Accepted: true, Score: 12, Reasons: []string{"target present"}}, CacheKey: key,
		}},
	}
	tokens := []analyzer.Token{
		{Surface: "Ich", Dependency: "nsubj", Head: 1},
		{Surface: "stehe", Dependency: "root", Head: 1},
		{Surface: "heute", Dependency: "advmod", Head: 1},
		{Surface: "auf", Dependency: "compound:prt", Head: 1},
	}
	deck, err := cardexport.NewPresentation(nil).RestoreWithRecoveredRenderInputs(snapshot, []cardexport.RecoveredRenderInput{{ManifestOrdinal: 0, Sentence: sentence, SentenceTokens: tokens}})
	require.NoError(t, err)
	assert.Empty(t, snapshot.Items[0].Entry.SentenceTokens)
	assert.Equal(t, tokens, deck.StorageProjection().Items[0].Entry.SentenceTokens)

	artifact, _, err := cardexport.NewPresentation(nil).Finalize(t.Context(), deck, []cardexport.StoredResult{{CacheKey: *key}}, cardexport.RunFacts{Consent: true, Configured: true, ExecutionMode: "batch", TargetLanguage: "en", Provider: "llm", ProviderVersion: "prompt-v1"})
	require.NoError(t, err)
	assert.Equal(t, "Ich <b>stehe</b> heute <b>auf</b>.", artifact.Generated[0].Note.Text)
}

func TestPresentationLifecycleRestoreRejectsMalformedSnapshots(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*cardexport.ManifestSnapshot)
	}{
		{
			name: "non-contiguous ordinals",
			mutate: func(snapshot *cardexport.ManifestSnapshot) {
				snapshot.Items[0].Ordinal = 1
			},
		},
		{
			name: "duplicate candidates",
			mutate: func(snapshot *cardexport.ManifestSnapshot) {
				item := snapshot.Items[0]
				item.Ordinal = 1
				snapshot.Items = append(snapshot.Items, item)
			},
		},
		{
			name: "partial accepted cache identity",
			mutate: func(snapshot *cardexport.ManifestSnapshot) {
				item := snapshot.Items[0]
				item.Ordinal = 1
				item.Entry.CanonicalLemma = "baum"
				item.CacheKey = &enrichment.CacheKey{Language: "de", TargetLanguage: "en", CanonicalLemma: "baum", UPOS: "NOUN", Provider: "llm", ProviderVersion: "prompt-v1", SentenceHash: enrichment.SentenceHash(item.Entry.Sentence)}
				snapshot.Items = append(snapshot.Items, item)
				snapshot.Items[1].CacheKey = nil
			},
		},
		{
			name: "unsupported schema version",
			mutate: func(snapshot *cardexport.ManifestSnapshot) {
				snapshot.SchemaVersion = cardexport.ManifestSchemaVersion + 1
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := lifecycleRestoreSnapshot()
			test.mutate(&snapshot)
			_, err := cardexport.NewPresentation(nil).Restore(snapshot)
			assert.ErrorIs(t, err, cardexport.ErrInvalidInput)
		})
	}
}

func TestPresentationLifecycleFreezeOrdersByFirstEncounter(t *testing.T) {
	late := lifecycleProjection()
	late.Candidate.CanonicalLemma = "apple"
	late.Entry.CanonicalLemma = "apple"
	late.Candidate.FirstEncounter = 200

	early := lifecycleProjection()
	early.Candidate.CanonicalLemma = "zebra"
	early.Entry.CanonicalLemma = "zebra"
	early.Candidate.FirstEncounter = 10

	deck, _, err := cardexport.NewPresentation(nil).Freeze(t.Context(), "owner-1", "Book", []cardexport.CandidateProjection{late, early})
	require.NoError(t, err)
	work := deck.WorkProjection()
	require.Len(t, work, 2)
	assert.Equal(t, 0, work[0].Ordinal)
	assert.Equal(t, 1, work[1].Ordinal)
	assert.Equal(t, "zebra", work[0].Request.CanonicalLemma)
	assert.Equal(t, "apple", work[1].Request.CanonicalLemma)
	assert.Equal(t, enrichment.CacheKey{
		Language: "de", TargetLanguage: "en", CanonicalLemma: "zebra", UPOS: "NOUN",
		Provider: "llm", ProviderVersion: "prompt-v1", SentenceHash: enrichment.SentenceHash(work[0].Request.ExampleSentence),
	}, work[0].CacheKey)
	assert.Equal(t, enrichment.CacheKey{
		Language: "de", TargetLanguage: "en", CanonicalLemma: "apple", UPOS: "NOUN",
		Provider: "llm", ProviderVersion: "prompt-v1", SentenceHash: enrichment.SentenceHash(work[1].Request.ExampleSentence),
	}, work[1].CacheKey)

	items := deck.StorageProjection().Items
	assert.Equal(t, "zebra", items[0].Entry.CanonicalLemma)
	assert.Equal(t, "apple", items[1].Entry.CanonicalLemma)
}

func TestPresentationLifecycleSelectsFrozenWorkByOrdinal(t *testing.T) {
	deck, _, err := cardexport.NewPresentation(lifecycleLexicalProvider{}).Freeze(t.Context(), "owner-1", "Book", []cardexport.CandidateProjection{lifecycleProjection()})
	require.NoError(t, err)

	work, ok := deck.WorkByOrdinal(0)
	require.True(t, ok)
	assert.Equal(t, deck.WorkProjection()[0], work)
	_, ok = deck.WorkByOrdinal(1)
	assert.False(t, ok)
}

func TestPresentationLifecycleFinalizesBatchWithMissingOptionalResults(t *testing.T) {
	deck, _, err := cardexport.NewPresentation(lifecycleLexicalProvider{}).Freeze(t.Context(), "owner-1", "Book", []cardexport.CandidateProjection{lifecycleProjection()})
	require.NoError(t, err)
	artifact, diagnostics, err := cardexport.NewPresentation(lifecycleLexicalProvider{}).Finalize(t.Context(), deck, nil, cardexport.RunFacts{Consent: true, Configured: true, ExecutionMode: "batch", TargetLanguage: "en", Provider: "llm", ProviderVersion: "prompt-v1"})
	require.NoError(t, err)
	assert.Equal(t, 1, artifact.Count)
	assert.Equal(t, "house · building", artifact.Generated[0].Note.Gloss)
	assert.Empty(t, diagnostics.DegradationCodes)
}

func TestPresentationLifecycleTreatsUnconfiguredExternalResultsAsOptional(t *testing.T) {
	deck, _, err := cardexport.NewPresentation(lifecycleLexicalProvider{}).Freeze(t.Context(), "owner-1", "Book", []cardexport.CandidateProjection{lifecycleProjection()})
	require.NoError(t, err)
	artifact, _, err := cardexport.NewPresentation(nil).Finalize(t.Context(), deck, nil, cardexport.RunFacts{ExecutionMode: "batch"})
	require.NoError(t, err)
	assert.Equal(t, "house · building", artifact.Generated[0].Note.Gloss)
}

func TestPresentationLifecycleReportsQualityOmissionAndMalformedOptionalData(t *testing.T) {
	projection := lifecycleProjection()
	projection.Sentences[0] = analyzer.Sentence{Text: "Fragment."}
	deck, diagnostics, err := cardexport.NewPresentation(nil).Freeze(t.Context(), "owner-1", "Book", []cardexport.CandidateProjection{projection})
	require.NoError(t, err)
	assert.Len(t, diagnostics.QualityOmissions, 1)
	assert.Equal(t, 0, deck.Summary().Accepted)

	projection = lifecycleProjection()
	deck, _, err = cardexport.NewPresentation(lifecycleLexicalProvider{}).Freeze(t.Context(), "owner-1", "Book", []cardexport.CandidateProjection{projection})
	require.NoError(t, err)
	work := deck.WorkProjection()
	malformed := enrichment.CacheEntry{CacheKey: work[0].CacheKey, SenseSelection: []int{9}, FallbackGloss: "<unsafe>"}
	artifact, finalDiagnostics, err := cardexport.NewPresentation(nil).Finalize(t.Context(), deck, []cardexport.StoredResult{{CacheKey: work[0].CacheKey, Record: malformed}}, cardexport.RunFacts{Consent: true, Configured: true, ExecutionMode: "batch", TargetLanguage: "en", Provider: "llm", ProviderVersion: "prompt-v1"})
	require.NoError(t, err)
	assert.Equal(t, "house · building", artifact.Generated[0].Note.Gloss)
	assert.Contains(t, finalDiagnostics.DegradationCodes, cardexport.DegradationInvalidSenseSelection)
}

func TestPresentationLifecycleRejectsIdentityAndProvenanceFailures(t *testing.T) {
	deck, _, err := cardexport.NewPresentation(lifecycleLexicalProvider{}).Freeze(t.Context(), "owner-1", "Book", []cardexport.CandidateProjection{lifecycleProjection()})
	require.NoError(t, err)
	work := deck.WorkProjection()
	wrongKey := work[0].CacheKey
	wrongKey.ProviderVersion = "other"
	_, _, err = cardexport.NewPresentation(nil).Finalize(t.Context(), deck, []cardexport.StoredResult{{CacheKey: wrongKey}}, cardexport.RunFacts{Consent: true, Configured: true, ExecutionMode: "batch", TargetLanguage: "en", Provider: "llm", ProviderVersion: "prompt-v1"})
	assert.ErrorIs(t, err, cardexport.ErrInvalidInput)

	wrongRecordKey := work[0].CacheKey
	wrongRecordKey.Provider = "other"
	_, _, err = cardexport.NewPresentation(nil).Finalize(t.Context(), deck, []cardexport.StoredResult{{CacheKey: work[0].CacheKey, Record: enrichment.CacheEntry{CacheKey: wrongRecordKey, Translation: "house"}}}, cardexport.RunFacts{Consent: true, Configured: true, ExecutionMode: "batch", TargetLanguage: "en", Provider: "llm", ProviderVersion: "prompt-v1"})
	assert.ErrorIs(t, err, cardexport.ErrInvalidInput)
}

func TestPresentationLifecycleRequiresCompleteStandardResults(t *testing.T) {
	deck, _, err := cardexport.NewPresentation(lifecycleLexicalProvider{}).Freeze(t.Context(), "owner-1", "Book", []cardexport.CandidateProjection{lifecycleProjection()})
	require.NoError(t, err)
	_, _, err = cardexport.NewPresentation(lifecycleLexicalProvider{}).Finalize(t.Context(), deck, nil, cardexport.RunFacts{Consent: true, Configured: true, ExecutionMode: "standard", TargetLanguage: "en", Provider: "llm", ProviderVersion: "prompt-v1"})
	assert.ErrorIs(t, err, cardexport.ErrInvalidInput)
}

func TestPresentationLifecycleReportsFallbackGlossAndRejectsWrongCandidate(t *testing.T) {
	deck, _, err := cardexport.NewPresentation(lifecycleLexicalProvider{}).Freeze(t.Context(), "owner-1", "Book", []cardexport.CandidateProjection{lifecycleProjection()})
	require.NoError(t, err)
	work := deck.WorkProjection()
	record := enrichment.CacheEntry{CacheKey: work[0].CacheKey, SenseSelection: []int{}, FallbackGloss: "a contextual house"}
	artifact, diagnostics, err := cardexport.NewPresentation(lifecycleLexicalProvider{}).Finalize(t.Context(), deck, []cardexport.StoredResult{{CacheKey: work[0].CacheKey, Record: record}}, cardexport.RunFacts{Consent: true, Configured: true, ExecutionMode: "batch", TargetLanguage: "en", Provider: "llm", ProviderVersion: "prompt-v1"})
	require.NoError(t, err)
	assert.Equal(t, "a contextual house", artifact.Generated[0].Note.Gloss)
	assert.Contains(t, diagnostics.DegradationCodes, cardexport.DegradationFallbackGlossApplied)
	assert.Equal(t, diagnostics, artifact.Diagnostics)

	wrongKey := work[0].CacheKey
	wrongKey.CanonicalLemma = "other"
	_, _, err = cardexport.NewPresentation(lifecycleLexicalProvider{}).Finalize(t.Context(), deck, []cardexport.StoredResult{{CacheKey: wrongKey}}, cardexport.RunFacts{Consent: true, Configured: true, ExecutionMode: "batch", TargetLanguage: "en", Provider: "llm", ProviderVersion: "prompt-v1"})
	assert.ErrorIs(t, err, cardexport.ErrInvalidInput)
}

func TestPresentationLifecycleReportsRejectedFallbackGloss(t *testing.T) {
	deck, _, err := cardexport.NewPresentation(lifecycleLexicalProvider{}).Freeze(t.Context(), "owner-1", "Book", []cardexport.CandidateProjection{lifecycleProjection()})
	require.NoError(t, err)
	work := deck.WorkProjection()
	artifact, diagnostics, err := cardexport.NewPresentation(nil).Finalize(t.Context(), deck, []cardexport.StoredResult{{
		CacheKey: work[0].CacheKey,
		Record:   enrichment.CacheEntry{CacheKey: work[0].CacheKey, SenseSelection: []int{}, FallbackGloss: "<unsafe>"},
	}}, cardexport.RunFacts{Consent: true, Configured: true, ExecutionMode: "batch", TargetLanguage: "en", Provider: "llm", ProviderVersion: "prompt-v1"})
	require.NoError(t, err)
	assert.Equal(t, "house · building", artifact.Generated[0].Note.Gloss)
	assert.Contains(t, diagnostics.DegradationCodes, cardexport.DegradationFallbackGlossRejected)
	assert.Equal(t, diagnostics, artifact.Diagnostics)
}

func TestPresentationLifecycleReportsRejectedFallbackEvenWhenSenseSelectionIsValid(t *testing.T) {
	deck, _, err := cardexport.NewPresentation(lifecycleLexicalProvider{}).Freeze(t.Context(), "owner-1", "Book", []cardexport.CandidateProjection{lifecycleProjection()})
	require.NoError(t, err)
	work := deck.WorkProjection()
	_, diagnostics, err := cardexport.NewPresentation(nil).Finalize(t.Context(), deck, []cardexport.StoredResult{{
		CacheKey: work[0].CacheKey,
		Record:   enrichment.CacheEntry{CacheKey: work[0].CacheKey, SenseSelection: []int{0}, FallbackGloss: "<unsafe>"},
	}}, cardexport.RunFacts{Consent: true, Configured: true, ExecutionMode: "batch", TargetLanguage: "en", Provider: "llm", ProviderVersion: "prompt-v1"})
	require.NoError(t, err)
	assert.Contains(t, diagnostics.DegradationCodes, cardexport.DegradationFallbackGlossRejected)
}

func TestPresentationLifecycleProjectionsAreDefensive(t *testing.T) {
	deck, _, err := cardexport.NewPresentation(lifecycleLexicalProvider{}).Freeze(t.Context(), "owner-1", "Book", []cardexport.CandidateProjection{lifecycleProjection()})
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
	refs, err := json.Marshal([]struct {
		SentenceIndex int              `json:"sentence_index"`
		Text          string           `json:"text"`
		Location      map[string]int64 `json:"location"`
	}{{SentenceIndex: 0, Text: "stale", Location: map[string]int64{"start_offset": 2}}})
	if err != nil {
		panic("lifecycle fixture references are not JSON encodable: " + err.Error())
	}
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

func lifecycleRestoreSnapshot() cardexport.ManifestSnapshot {
	sentence := "Das Haus steht heute dort."
	key := &enrichment.CacheKey{Language: "de", TargetLanguage: "en", CanonicalLemma: "haus", UPOS: "NOUN", Provider: "llm", ProviderVersion: "prompt-v1", DictionaryProviderVersion: "dictionary-v1", SentenceHash: enrichment.SentenceHash(sentence)}
	return cardexport.ManifestSnapshot{
		SchemaVersion: cardexport.ManifestSchemaVersion,
		Owner:         "owner-1",
		DeckName:      "Book",
		Filename:      cardexport.DownloadFilename("Book"),
		Items: []cardexport.ManifestItem{{
			Ordinal: 0, Disposition: cardexport.ManifestAccepted,
			Entry: cardexport.Entry{
				Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", Sentence: sentence,
				TargetWord: "Haus", Gloss: "house", Plural: "Häuser", IPA: "/haʊ̯s/", PrincipalParts: "geht · ging · gegangen",
				DictionaryProviderVersion: "dictionary-v1", CandidateSenses: []enrichment.LexicalSense{{Gloss: "house"}},
			},
			Quality:  cardexport.SentenceQuality{Accepted: true, Score: 12, Reasons: []string{"target present"}},
			CacheKey: key,
		}},
	}
}
