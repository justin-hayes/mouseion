package enrichment_test

import (
	"encoding/json"
	"html"
	"os"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type alignmentQualityFixture struct {
	Name                    string   `json:"name"`
	Language                string   `json:"language"`
	Lemma                   string   `json:"lemma"`
	TargetWord              string   `json:"target_word"`
	SourceSentence          string   `json:"source_sentence"`
	Translation             string   `json:"translation"`
	SentenceTranslation     string   `json:"sentence_translation"`
	SentenceTargets         []string `json:"sentence_translation_targets"`
	Gloss                   string   `json:"gloss"`
	ExpectedEnglishSentence string   `json:"expected_english_sentence"`
}

func TestMultilingualAlignmentProviderFixturesPreserveTheEnglishCardContract(t *testing.T) {
	fixtureBytes, err := os.ReadFile("testdata/multilingual_alignment_quality.json")
	require.NoError(t, err)
	var fixtures []alignmentQualityFixture
	require.NoError(t, json.Unmarshal(fixtureBytes, &fixtures))
	require.NotEmpty(t, fixtures)

	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "fixture-model"})
	require.NoError(t, err)
	seenLanguages := map[string]bool{}
	for _, fixture := range fixtures {
		t.Run(fixture.Name, func(t *testing.T) {
			seenLanguages[fixture.Language] = true
			request := enrichment.TranslationRequest{
				Language: fixture.Language, TargetLanguage: "en", CanonicalLemma: fixture.Lemma,
				TargetWord: fixture.TargetWord, ExampleSentence: fixture.SourceSentence,
				RequireContextualGloss: true,
			}
			content, marshalErr := json.Marshal(struct {
				ItemID         string   `json:"item_id"`
				SourceLanguage string   `json:"source_language"`
				TargetLanguage string   `json:"target_language"`
				Translation    string   `json:"translation"`
				Sentence       string   `json:"sentence_translation"`
				Targets        []string `json:"sentence_translation_targets"`
				Gloss          string   `json:"gloss"`
				EvidenceIDs    []string `json:"evidence_ids"`
				ContextOnly    bool     `json:"context_only"`
				Unresolved     string   `json:"unresolved_reason"`
			}{
				ItemID: enrichment.TranslationItemID(request), SourceLanguage: fixture.Language, TargetLanguage: "en",
				Translation: fixture.Translation, Sentence: fixture.SentenceTranslation, Targets: fixture.SentenceTargets,
				Gloss: fixture.Gloss, EvidenceIDs: []string{}, ContextOnly: true, Unresolved: "",
			})
			require.NoError(t, marshalErr)
			providerResponse, marshalErr := json.Marshal(struct {
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
			require.NoError(t, marshalErr)
			response, decodeErr := codec.DecodeResponse(request, providerResponse)
			require.NoError(t, decodeErr)
			require.Equal(t, fixture.SentenceTranslation, response.SentenceTranslation)
			require.Equal(t, fixture.Gloss, response.Gloss)

			key := enrichment.CacheKey{
				Language: fixture.Language, TargetLanguage: "en", CanonicalLemma: fixture.Lemma, UPOS: "NOUN",
				Provider: "fixture", ProviderVersion: codec.ContextualGlossProviderVersion(),
				SentenceHash: enrichment.SentenceHash(fixture.SourceSentence),
			}
			deck, restoreErr := cardexport.NewPresentation(nil).Restore(cardexport.ManifestSnapshot{
				SchemaVersion: cardexport.ManifestSchemaVersion, Owner: "owner", DeckName: "Fixture", Filename: cardexport.DownloadFilename("Fixture"),
				Items: []cardexport.ManifestItem{{Ordinal: 0, Disposition: cardexport.ManifestAccepted,
					Entry:   cardexport.Entry{Language: fixture.Language, CanonicalLemma: fixture.Lemma, UPOS: "NOUN", Sentence: fixture.SourceSentence, TargetWord: fixture.TargetWord, Gloss: "frozen dictionary evidence"},
					Quality: cardexport.SentenceQuality{Accepted: true, Score: 1, Reasons: []string{"target present"}}, CacheKey: &key,
				}},
			})
			require.NoError(t, restoreErr)
			work := deck.WorkProjection()
			require.Len(t, work, 1)
			record := enrichment.CacheEntry{
				CacheKey: work[0].CacheKey, Translation: response.Translation, SentenceTranslation: response.SentenceTranslation,
				SentenceTranslationTargets: response.SentenceTranslationTargets, FallbackGloss: response.Gloss,
			}
			for _, mode := range []string{"standard", "batch"} {
				artifact, _, finalizeErr := cardexport.NewPresentation(nil).Finalize(t.Context(), deck, []cardexport.StoredResult{{CacheKey: work[0].CacheKey, Record: record}}, cardexport.RunFacts{
					Consent: true, Configured: true, ExecutionMode: mode, TargetLanguage: "en", Provider: "fixture", ProviderVersion: codec.ContextualGlossProviderVersion(),
				})
				require.NoError(t, finalizeErr)
				require.Len(t, artifact.Generated, 1)
				assert.Equal(t, fixture.ExpectedEnglishSentence, artifact.Generated[0].Note.EnglishSentence)
				assert.Equal(t, fixture.SentenceTranslation, html.UnescapeString(plainEnglishFromExpected(artifact.Generated[0].Note.EnglishSentence)), "complete translation remains present in %s mode", mode)
				assert.Equal(t, fixture.Gloss, artifact.Generated[0].Note.Gloss, "contextual Gloss remains available in %s mode", mode)
			}
		})
	}
	assert.Equal(t, map[string]bool{"de": true, "it": true, "el": true}, seenLanguages)
}

func plainEnglishFromExpected(value string) string {
	return strings.ReplaceAll(strings.ReplaceAll(value, "<b>", ""), "</b>", "")
}
