package enrichment_test

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type contextualOutcomeFixture struct {
	Name                string   `json:"name"`
	Language            string   `json:"language"`
	Lemma               string   `json:"lemma"`
	UPOS                string   `json:"upos"`
	TargetWord          string   `json:"target_word"`
	SourceSentence      string   `json:"source_sentence"`
	Translation         string   `json:"translation"`
	SentenceTranslation string   `json:"sentence_translation"`
	SentenceTargets     []string `json:"sentence_translation_targets"`
	Gloss               string   `json:"gloss"`
	EvidenceIDs         []string `json:"evidence_ids"`
	ContextOnly         bool     `json:"context_only"`
	UnresolvedKind      string   `json:"unresolved_kind"`
	UnresolvedReason    string   `json:"unresolved_reason"`
	ExpectedOutcome     string   `json:"expected_outcome"`
	ExpectedReason      string   `json:"expected_reason"`
	ExpectedGloss       string   `json:"expected_gloss"`
}

func loadContextualOutcomeFixtures(t *testing.T) []contextualOutcomeFixture {
	t.Helper()
	fixtureBytes, err := os.ReadFile("testdata/contextual_gloss_unresolved_contract.json")
	require.NoError(t, err)
	var fixtures []contextualOutcomeFixture
	require.NoError(t, json.Unmarshal(fixtureBytes, &fixtures))
	require.NotEmpty(t, fixtures)
	return fixtures
}

func contextualOutcomeRequest(fixture contextualOutcomeFixture) enrichment.TranslationRequest {
	return enrichment.TranslationRequest{
		Language: fixture.Language, TargetLanguage: "en", CanonicalLemma: fixture.Lemma, UPOS: fixture.UPOS,
		TargetWord: fixture.TargetWord, ExampleSentence: fixture.SourceSentence, RequireContextualGloss: true,
	}
}

func contextualOutcomeResponse(t *testing.T, fixture contextualOutcomeFixture) []byte {
	t.Helper()
	content, err := json.Marshal(struct {
		ItemID         string   `json:"item_id"`
		SourceLanguage string   `json:"source_language"`
		TargetLanguage string   `json:"target_language"`
		Translation    string   `json:"translation"`
		Sentence       string   `json:"sentence_translation"`
		Targets        []string `json:"sentence_translation_targets"`
		Gloss          string   `json:"gloss"`
		EvidenceIDs    []string `json:"evidence_ids"`
		ContextOnly    bool     `json:"context_only"`
		UnresolvedKind string   `json:"unresolved_kind"`
		Unresolved     string   `json:"unresolved_reason"`
	}{
		ItemID: enrichment.ContextualGlossItemHandle, SourceLanguage: fixture.Language, TargetLanguage: "en",
		Translation: fixture.Translation, Sentence: fixture.SentenceTranslation, Targets: fixture.SentenceTargets,
		Gloss: fixture.Gloss, EvidenceIDs: fixture.EvidenceIDs, ContextOnly: fixture.ContextOnly,
		UnresolvedKind: fixture.UnresolvedKind, Unresolved: fixture.UnresolvedReason,
	})
	require.NoError(t, err)
	body, err := json.Marshal(struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}{Choices: []struct {
		Message struct {
			Content string `json:"content"`
		} `json:"message"`
	}{{Message: struct {
		Content string `json:"content"`
	}{Content: string(content)}}}})
	require.NoError(t, err)
	return body
}

func contextualOutcomeDeck(t *testing.T, codec *enrichment.TranslationCodec, fixture contextualOutcomeFixture) (cardexport.FrozenDeck, cardexport.WorkItem, cardexport.WorkItem) {
	t.Helper()
	providerVersion := codec.ContextualGlossProviderVersion()
	projection := func(fixture contextualOutcomeFixture) cardexport.CandidateProjection {
		observedForms, err := json.Marshal([]string{fixture.TargetWord})
		require.NoError(t, err)
		return cardexport.CandidateProjection{
			OwnerID: "owner-1", DeckName: "Contract",
			Candidate: domain.SelectionCandidate{Language: fixture.Language, CanonicalLemma: fixture.Lemma, UPOS: fixture.UPOS, ObservedForms: observedForms},
			Entry: cardexport.Entry{
				Language: fixture.Language, CanonicalLemma: fixture.Lemma, UPOS: fixture.UPOS,
				Sentence: fixture.SourceSentence, TargetWord: fixture.TargetWord, Gloss: "frozen dictionary evidence",
			},
			RequireContextualGloss: true,
			Provider:               "fixture", ProviderVersion: providerVersion, TargetLanguage: "en",
		}
	}
	control := contextualOutcomeFixture{
		Language: "de", Lemma: "Haus", UPOS: "NOUN", TargetWord: "Haus",
		SourceSentence: "Das alte Haus steht heute ganz ruhig dort.",
	}
	deck, _, err := cardexport.NewPresentation(nil).Freeze(t.Context(), "owner-1", "Contract", []cardexport.CandidateProjection{projection(fixture), projection(control)})
	require.NoError(t, err)
	work := deck.WorkProjection()
	require.Len(t, work, 2)
	var target, other cardexport.WorkItem
	for _, item := range work {
		if item.Request.TargetWord == fixture.TargetWord {
			target = item
		} else {
			other = item
		}
	}
	require.NotEmpty(t, target.CacheKey.CanonicalLemma)
	require.NotEmpty(t, other.CacheKey.CanonicalLemma)
	return deck, target, other
}

func controlRecord(item cardexport.WorkItem) cardexport.StoredResult {
	return cardexport.StoredResult{CacheKey: item.CacheKey, Record: enrichment.CacheEntry{
		CacheKey: item.CacheKey, Translation: "house", SentenceTranslation: "The house is old.", FallbackGloss: "house",
	}}
}

func TestContextualGlossUnresolvedContractFixtures(t *testing.T) {
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "fixture-model"})
	require.NoError(t, err)
	fixtures := loadContextualOutcomeFixtures(t)
	seenOutcomes := map[string]bool{}

	for _, fixture := range fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			seenOutcomes[fixture.ExpectedOutcome] = true
			input := contextualOutcomeRequest(fixture)
			body := contextualOutcomeResponse(t, fixture)
			response, decodeErr := codec.DecodeResponse(input, body)
			if fixture.ExpectedOutcome == "invalid" {
				require.ErrorIs(t, decodeErr, enrichment.ErrInvalidTranslationResponse)
				return
			}
			require.NoError(t, decodeErr)

			switch fixture.ExpectedOutcome {
			case "resolved":
				assert.Equal(t, fixture.ExpectedGloss, response.Gloss)
				assert.Empty(t, response.UnresolvedKind, "a resolved gloss discards a stray kind")
				assert.Empty(t, response.UnresolvedReason, "a resolved gloss discards a stray reason")
			case "lemma_mismatch":
				assert.Equal(t, enrichment.ContextualGlossUnresolvedLemmaMismatch, response.UnresolvedKind)
				assert.Equal(t, domain.DeckPreparationLemmaMismatchOmissionReason(response.UnresolvedReason), fixture.ExpectedReason)
			case "ambiguous":
				assert.Equal(t, enrichment.ContextualGlossUnresolvedAmbiguous, response.UnresolvedKind)
				assert.Equal(t, response.UnresolvedReason, fixture.ExpectedReason)
			}

			deck, target, control := contextualOutcomeDeck(t, codec, fixture)
			results := []cardexport.StoredResult{controlRecord(control)}
			switch fixture.ExpectedOutcome {
			case "resolved":
				results = append(results, cardexport.StoredResult{CacheKey: target.CacheKey, Record: enrichment.CacheEntry{
					CacheKey: target.CacheKey, Translation: response.Translation, SentenceTranslation: response.SentenceTranslation,
					SentenceTranslationTargets: response.SentenceTranslationTargets, FallbackGloss: response.Gloss,
				}})
			case "lemma_mismatch", "ambiguous":
				results = append(results, cardexport.StoredResult{CacheKey: target.CacheKey, OmissionReason: fixture.ExpectedReason})
			}
			facts := cardexport.RunFacts{
				Consent: true, Configured: true, ExecutionMode: "standard", TargetLanguage: "en",
				Provider: "fixture", ProviderVersion: codec.ContextualGlossProviderVersion(),
			}
			for _, mode := range []string{"standard", "batch"} {
				modeFacts := facts
				modeFacts.ExecutionMode = mode
				artifact, diagnostics, finalizeErr := cardexport.NewPresentation(nil).Finalize(t.Context(), deck, results, modeFacts)
				require.NoError(t, finalizeErr)
				if fixture.ExpectedOutcome == "resolved" {
					assert.Len(t, artifact.Generated, 2, "%s mode renders the resolved card", mode)
					assert.Contains(t, artifact.TSV, fixture.ExpectedGloss)
					continue
				}
				assert.Len(t, artifact.Generated, 1, "%s mode omits only the unresolved card", mode)
				assert.Equal(t, []cardexport.MeaningOmission{{Target: fixture.TargetWord, Reason: fixture.ExpectedReason}}, diagnostics.MeaningOmissions)
				assert.NotContains(t, artifact.TSV, fixture.TargetWord)
			}
		})
	}
	assert.Equal(t, map[string]bool{"ambiguous": true, "invalid": true, "lemma_mismatch": true, "resolved": true}, seenOutcomes)
}

func TestAllLemmaMismatchOmissionsStillFailFinalize(t *testing.T) {
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "fixture-model"})
	require.NoError(t, err)
	fixture := loadContextualOutcomeFixtures(t)[0]
	input := contextualOutcomeRequest(fixture)
	response, err := codec.DecodeResponse(input, contextualOutcomeResponse(t, fixture))
	require.NoError(t, err)
	require.Equal(t, enrichment.ContextualGlossUnresolvedLemmaMismatch, response.UnresolvedKind)

	providerVersion := codec.ContextualGlossProviderVersion()
	observedForms, err := json.Marshal([]string{fixture.TargetWord})
	require.NoError(t, err)
	deck, _, err := cardexport.NewPresentation(nil).Freeze(t.Context(), "owner-1", "Contract", []cardexport.CandidateProjection{{
		OwnerID: "owner-1", DeckName: "Contract",
		Candidate: domain.SelectionCandidate{Language: fixture.Language, CanonicalLemma: fixture.Lemma, UPOS: fixture.UPOS, ObservedForms: observedForms},
		Entry: cardexport.Entry{
			Language: fixture.Language, CanonicalLemma: fixture.Lemma, UPOS: fixture.UPOS,
			Sentence: fixture.SourceSentence, TargetWord: fixture.TargetWord, Gloss: "frozen dictionary evidence",
		},
		RequireContextualGloss: true, Provider: "fixture", ProviderVersion: providerVersion, TargetLanguage: "en",
	}})
	require.NoError(t, err)
	work := deck.WorkProjection()
	require.Len(t, work, 1)
	target := work[0]
	_, _, err = cardexport.NewPresentation(nil).Finalize(t.Context(), deck, []cardexport.StoredResult{{
		CacheKey: target.CacheKey, OmissionReason: domain.DeckPreparationLemmaMismatchOmissionReason(response.UnresolvedReason),
	}}, cardexport.RunFacts{
		Consent: true, Configured: true, ExecutionMode: "standard", TargetLanguage: "en", Provider: "fixture", ProviderVersion: providerVersion,
	})
	require.ErrorIs(t, err, cardexport.ErrAllMeaningsUnresolved)
}
