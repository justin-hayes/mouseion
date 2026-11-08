package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/stretchr/testify/require"
)

func TestEmbeddedIdiomCaseSetCoversEveryKindAndLanguage(t *testing.T) {
	cases, err := loadIdiomCaseSet(bytes.NewReader(idiomReviewCases))
	require.NoError(t, err)

	t.Run("version is pinned", func(t *testing.T) {
		require.Equal(t, "idiom-review-v1", cases.Version)
	})
	t.Run("every supported language is present", func(t *testing.T) {
		seen := map[string]bool{}
		for _, reviewCase := range cases.Cases {
			seen[reviewCase.Language] = true
		}
		for _, language := range reviewLanguages {
			require.True(t, seen[language], "case set has no %s case", language)
		}
	})
	t.Run("every case kind is present", func(t *testing.T) {
		seen := map[idiomCaseKind]bool{}
		for _, reviewCase := range cases.Cases {
			seen[reviewCase.Kind] = true
		}
		for _, kind := range idiomCaseKinds {
			require.True(t, seen[kind], "case set has no %s case", kind)
		}
	})
	t.Run("the reviewed cases are present", func(t *testing.T) {
		ids := idiomCaseIDs(cases)
		for _, want := range []string{
			"it-in-gamba", "it-in-bocca-al-lupo", "it-macchina-da-scrivere",
			"el-kano-ta-strava-matia", "it-tagliare-la-corda-literal",
			"de-jemanden-auf-den-arm-nehmen-literal", "it-gamba-lemma-card", "de-arm-lemma-card",
		} {
			require.Contains(t, ids, want)
		}
	})
	t.Run("positive cases expect a Gloss", func(t *testing.T) {
		for _, reviewCase := range cases.Cases {
			if reviewCase.Kind != idiomCasePositive {
				continue
			}
			require.NotEmpty(t, reviewCase.ExpectedGloss, "%s has no expected Gloss", reviewCase.ID)
			require.NotEmpty(t, reviewCase.Evidence, "%s has no frozen evidence", reviewCase.ID)
		}
	})
	t.Run("literal-use controls expect no Gloss", func(t *testing.T) {
		for _, reviewCase := range cases.Cases {
			if reviewCase.Kind != idiomCaseLiteralUse {
				continue
			}
			require.Empty(t, reviewCase.ExpectedGloss, "%s must not expect a Gloss", reviewCase.ID)
		}
	})
}

func TestLoadIdiomCaseSetRejectsUnknownFieldsAndIncompleteCases(t *testing.T) {
	valid := `{"version":"v1","cases":[` +
		`{"id":"it-positive","language":"it","kind":"positive","headword":"in gamba","matched_forms":["in"],"sentence":"In gamba.","evidence":[{"evidence_id":"e1","gloss":"capable"}],"expected_gloss":"capable"},` +
		`{"id":"de-literal","language":"de","kind":"literal_use_control","headword":"auf den Arm nehmen","matched_forms":["Arm"],"sentence":"Auf den Arm.","evidence":[{"evidence_id":"e1","gloss":"leg"}]},` +
		`{"id":"el-lemma","language":"el","kind":"lemma_card_control","headword":"κάνω τα στραβά μάτια","anchor_lemma":"μάτι","anchor_upos":"NOUN","anchor_form":"μάτια","idiom_sentence":"Τα στραβά μάτια.","lemma_card_sentence":"Τα μάτια μου."}]}`
	for name, testCase := range map[string]struct {
		input string
		want  string
	}{
		"unknown field":          {input: strings.Replace(valid, `"headword":"in gamba"`, `"headword":"in gamba","unexpected":true`, 1), want: "unknown field"},
		"missing language":       {input: strings.Replace(valid, `"language":"el"`, `"language":"it"`, 1), want: "no el case"},
		"no cases":               {input: `{"version":"v1","cases":[]}`, want: "no cases"},
		"missing kind":           {input: strings.Replace(valid, `"kind":"positive"`, `"kind":"unexpected"`, 1), want: "unknown kind"},
		"positive without Gloss": {input: strings.Replace(valid, `,"expected_gloss":"capable"`, ``, 1), want: "needs an expected Gloss"},
		"duplicate case id":      {input: strings.Replace(valid, `"id":"de-literal"`, `"id":"it-positive"`, 1), want: "duplicate case id"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := loadIdiomCaseSet(strings.NewReader(testCase.input))
			require.ErrorContains(t, err, testCase.want)
		})
	}
}

func TestIdiomCaseValidationRejectsIncompleteControls(t *testing.T) {
	base := idiomReviewCase{
		ID: "it-positive", Language: "it", Kind: idiomCasePositive, Headword: "in gamba",
		MatchedForms: []string{"in", "gamba"}, Sentence: "È in gamba.", ExpectedGloss: "capable",
		Evidence: []enrichment.LexicalSense{{EvidenceID: "e1", Gloss: "capable"}},
	}
	for name, mutate := range map[string]func(*idiomReviewCase){
		"empty id":               func(c *idiomReviewCase) { c.ID = "" },
		"unsupported language":   func(c *idiomReviewCase) { c.Language = "fr" },
		"empty headword":         func(c *idiomReviewCase) { c.Headword = "" },
		"no matched forms":       func(c *idiomReviewCase) { c.MatchedForms = nil },
		"blank matched form":     func(c *idiomReviewCase) { c.MatchedForms = []string{" "} },
		"empty sentence":         func(c *idiomReviewCase) { c.Sentence = "" },
		"no evidence":            func(c *idiomReviewCase) { c.Evidence = nil },
		"missing evidence id":    func(c *idiomReviewCase) { c.Evidence[0].EvidenceID = "" },
		"missing evidence gloss": func(c *idiomReviewCase) { c.Evidence[0].Gloss = "" },
		"duplicate evidence":     func(c *idiomReviewCase) { c.Evidence = append(c.Evidence, c.Evidence[0]) },
		"missing expected Gloss": func(c *idiomReviewCase) { c.ExpectedGloss = "" },
	} {
		t.Run(name, func(t *testing.T) {
			reviewCase := base
			reviewCase.Evidence = append([]enrichment.LexicalSense(nil), base.Evidence...)
			mutate(&reviewCase)
			require.Error(t, reviewCase.validate())
		})
	}

	lemma := idiomReviewCase{
		ID: "it-lemma", Language: "it", Kind: idiomCaseLemmaCard, Headword: "in gamba",
		AnchorLemma: "gamba", AnchorUPOS: "NOUN", AnchorForm: "gamba",
		IdiomSentence: "È un ragazzo in gamba.", LemmaCardSentence: "Mi fa male la gamba.",
	}
	literal := idiomReviewCase{
		ID: "it-literal", Language: "it", Kind: idiomCaseLiteralUse, Headword: "tagliare la corda",
		MatchedForms: []string{"Tagliò", "la", "corda"}, Sentence: "Tagliò la corda del pacco.", ExpectedGloss: "to run away",
		Evidence: []enrichment.LexicalSense{{EvidenceID: "e1", Gloss: "to run away"}},
	}
	require.ErrorContains(t, literal.validate(), "literal-use control has no expected Gloss")

	for name, mutate := range map[string]func(*idiomReviewCase){
		"no anchor lemma":        func(c *idiomReviewCase) { c.AnchorLemma = "" },
		"no anchor upos":         func(c *idiomReviewCase) { c.AnchorUPOS = "" },
		"no anchor form":         func(c *idiomReviewCase) { c.AnchorForm = "" },
		"no idiom sentence":      func(c *idiomReviewCase) { c.IdiomSentence = "" },
		"no lemma-card sentence": func(c *idiomReviewCase) { c.LemmaCardSentence = "" },
		"same sentence":          func(c *idiomReviewCase) { c.LemmaCardSentence = c.IdiomSentence },
		"missing anchor":         func(c *idiomReviewCase) { c.LemmaCardSentence = "Mi fa male la testa." },
	} {
		t.Run(name, func(t *testing.T) {
			reviewCase := lemma
			mutate(&reviewCase)
			require.Error(t, reviewCase.validate())
		})
	}
}

func TestReviewIdiomScoresCasesThroughTheStubbedIdiomContract(t *testing.T) {
	var calls int
	client := stubIdiomClient(t, func(payload idiomProviderRequestPayload) idiomStubAnswer {
		calls++
		if payload.Headword == "tagliare la corda" {
			return idiomStubAnswer{translation: "to run away", unresolvedKind: string(enrichment.ContextualGlossUnresolvedLiteralUse), reason: "The cord belongs to the parcel."}
		}
		return idiomStubAnswer{translation: "capable", gloss: "capable"}
	})
	report := reviewIdiom(context.Background(), idiomReviewOptions{
		Now:            time.Date(2026, 11, 8, 12, 0, 0, 0, time.UTC),
		Cases:          idiomReviewTestCases(),
		Client:         client,
		Provider:       idiomTestProviderMetadata(),
		MatcherVersion: "idiom-matcher/1",
		GapBound:       2,
	})
	require.Len(t, report.Cases, 4)
	byID := idiomResultsByCaseID(report.Cases)

	t.Run("one provider request per provider case", func(t *testing.T) {
		require.Equal(t, 3, calls, "the lemma-card control needs no provider call")
	})
	t.Run("positive case is confirmed", func(t *testing.T) {
		require.Equal(t, idiomOutcomeConfirmed, byID["it-in-gamba"].Outcome)
		require.Equal(t, checkPass, idiomCheckStatus(byID["it-in-gamba"], checkNameIdiomConfirmed))
	})
	t.Run("literal-use control is rejected", func(t *testing.T) {
		require.Equal(t, idiomOutcomeLiteralUse, byID["it-tagliare-la-corda"].Outcome)
		require.Equal(t, checkPass, idiomCheckStatus(byID["it-tagliare-la-corda"], checkNameIdiomLiteralUse))
	})
	t.Run("lemma-card control passes without a provider", func(t *testing.T) {
		require.Equal(t, idiomOutcomeLemmaCardControl, byID["it-gamba-lemma-card"].Outcome)
		require.Equal(t, checkPass, idiomCheckStatus(byID["it-gamba-lemma-card"], checkNameIdiomLemmaCard))
	})
	t.Run("summary", func(t *testing.T) {
		require.Equal(t, idiomReviewSummary{Total: 4, Confirmed: 2, LiteralUse: 1, LemmaCardControls: 1, ChecksPass: 4}, report.Summary)
	})

	var output bytes.Buffer
	require.NoError(t, writeIdiomReport(&output, report))
	rendered := output.String()
	for name, fragment := range map[string]string{
		"title":            "# Idiom card review — `idiom-card-v1`",
		"date":             "- Date: 2026-11-08",
		"model":            "- Model: `gpt-review`",
		"reasoning":        "- Reasoning setting: `low`",
		"prompt contract":  "- Prompt contract: `idiom-card-v1`",
		"matcher version":  "- Matcher version: `idiom-matcher/1`",
		"gap bound":        "- Matcher gap bound: 2",
		"case set":         "- Case set: `idiom-review-test` (4 cases)",
		"sample absent":    "- Sample: not requested",
		"gate":             "- [ ] Gate evaluated and recorded: __________",
		"case heading":     "## `it-in-gamba` — it `in gamba` (positive)",
		"matched words":    "- Matched words: `in, gamba`",
		"request headword": "| headword | `in gamba` |",
		"outcome":          "- Outcome: `confirmed`",
		"gloss":            "- Raw Gloss: `capable`",
		"confirmed check":  "| outcome is `confirmed` | **PASS** |",
		"literal check":    "| outcome is `literal_use` | **PASS** |",
		"lemma check":      "| lemma card moves off the idiom sentence | **PASS** |",
		"citation verdict": "- [ ] Citation form — verdict: __________",
		"meaning verdict":  "- [ ] Meaning correct in context — verdict: __________",
	} {
		t.Run(name, func(t *testing.T) {
			require.Contains(t, rendered, fragment)
		})
	}
	t.Run("no API key", func(t *testing.T) {
		require.NotContains(t, rendered, "test-key")
	})
}

func TestReviewIdiomFailsAControlThatIsWronglyConfirmedOrRejected(t *testing.T) {
	t.Run("a confirmed literal-use control fails", func(t *testing.T) {
		client := stubIdiomClient(t, func(idiomProviderRequestPayload) idiomStubAnswer {
			return idiomStubAnswer{translation: "to run away", gloss: "to run away"}
		})
		report := reviewIdiom(context.Background(), idiomReviewOptions{
			Now: time.Now(), Cases: idiomReviewTestCases(), Client: client, Provider: idiomTestProviderMetadata(),
		})
		result := idiomResultsByCaseID(report.Cases)["it-tagliare-la-corda"]
		require.Equal(t, idiomOutcomeConfirmed, result.Outcome)
		require.Equal(t, checkFail, idiomCheckStatus(result, checkNameIdiomLiteralUse))
	})
	t.Run("a rejected positive case fails", func(t *testing.T) {
		client := stubIdiomClient(t, func(idiomProviderRequestPayload) idiomStubAnswer {
			return idiomStubAnswer{translation: "capable", unresolvedKind: string(enrichment.ContextualGlossUnresolvedAmbiguous), reason: "Not enough evidence."}
		})
		report := reviewIdiom(context.Background(), idiomReviewOptions{
			Now: time.Now(), Cases: idiomReviewTestCases(), Client: client, Provider: idiomTestProviderMetadata(),
		})
		result := idiomResultsByCaseID(report.Cases)["it-in-gamba"]
		require.Equal(t, idiomOutcomeAmbiguous, result.Outcome)
		require.Equal(t, checkFail, idiomCheckStatus(result, checkNameIdiomConfirmed))
	})
}

func TestSelectIdiomSampleIsDeterministicForBookAndSeed(t *testing.T) {
	cards := make([]persistence.ConfirmedIdiomCard, 0, 40)
	for i := range 40 {
		cards = append(cards, persistence.ConfirmedIdiomCard{
			Language: "it", Headword: "idiom-" + string(rune('a'+i%26)) + "-" + string(rune('a'+i/26)),
			Sentence: "Frase " + string(rune('a'+i%26)) + " " + string(rune('a'+i/26)) + ".",
		})
	}

	first := selectIdiomSample(cards, "book-1", "seed-1", idiomSampleSize)
	second := selectIdiomSample(cards, "book-1", "seed-1", idiomSampleSize)
	otherSeed := selectIdiomSample(cards, "book-1", "seed-2", idiomSampleSize)
	otherBook := selectIdiomSample(cards, "book-2", "seed-1", idiomSampleSize)

	t.Run("selects the reviewed sample size", func(t *testing.T) {
		require.Len(t, first, idiomSampleSize)
	})
	t.Run("same Book and seed select the same cards", func(t *testing.T) {
		require.Equal(t, first, second)
	})
	t.Run("a different seed changes the order", func(t *testing.T) {
		require.NotEqual(t, first, otherSeed)
	})
	t.Run("a different Book changes the order", func(t *testing.T) {
		require.NotEqual(t, first, otherBook)
	})
	t.Run("a short deck is not padded", func(t *testing.T) {
		require.Len(t, selectIdiomSample(cards[:7], "book-1", "seed-1", idiomSampleSize), 7)
	})
	t.Run("duplicates are one card", func(t *testing.T) {
		duplicated := append([]persistence.ConfirmedIdiomCard(nil), cards...)
		duplicated = append(duplicated, cards[0])
		require.Len(t, selectIdiomSample(duplicated, "book-1", "seed-1", 100), 40)
	})
	t.Run("zero limit selects nothing", func(t *testing.T) {
		require.Empty(t, selectIdiomSample(cards, "book-1", "seed-1", 0))
	})
}

func TestExecuteIdiomCommandWritesTheDatedReportWithASample(t *testing.T) {
	client := stubIdiomClient(t, func(payload idiomProviderRequestPayload) idiomStubAnswer {
		if strings.Contains(payload.Headword, "tagliare") || strings.Contains(payload.Headword, "Arm nehmen") {
			return idiomStubAnswer{translation: "literal", unresolvedKind: string(enrichment.ContextualGlossUnresolvedLiteralUse), reason: "The words are literal."}
		}
		return idiomStubAnswer{translation: "review", gloss: "review"}
	})
	source := &fakeIdiomSampleSource{cards: sampleCards(35)}
	path := filepath.Join(t.TempDir(), "idiom-review.md")
	options, err := parseReviewCommand([]string{
		"-review", "idiom", "-out", path, "-date", "2026-11-08",
		"-book", "book-1", "-language", "it", "-seed", "seed-1",
	})
	require.NoError(t, err)
	var output bytes.Buffer
	require.NoError(t, executeIdiomCommand(context.Background(), options, &output,
		func() (reviewEnvironment, error) {
			return reviewEnvironment{Client: client, Provider: idiomTestProviderMetadata()}, nil
		},
		func(context.Context) (idiomSampleSource, error) { return source, nil }))

	rendered, err := os.ReadFile(path) //nolint:gosec // path is inside t.TempDir().
	require.NoError(t, err)
	for name, fragment := range map[string]string{
		"report title":   "# Idiom card review — `idiom-card-v1`",
		"report date":    "- Date: 2026-11-08",
		"full case set":  "- Case set: `idiom-review-v1` (8 cases)",
		"matcher":        "- Matcher version: `idiom-matcher/1`",
		"sample header":  "- Sample: 30 confirmed idiom cards requested from Book `book-1` in `it` (seed `seed-1`); selected 30",
		"sample heading": "### 1. `idiom",
		"gate":           "- every positive case is confirmed with a citation-form Gloss;",
	} {
		t.Run(name, func(t *testing.T) {
			require.Contains(t, string(rendered), fragment)
		})
	}
	t.Run("stdout", func(t *testing.T) {
		require.Equal(t, "idiom review report written: path="+path+" cases=8 sample=30\n", output.String())
	})
	t.Run("sample source is closed", func(t *testing.T) {
		require.True(t, source.closed)
	})
	t.Run("sample language is passed through", func(t *testing.T) {
		require.Equal(t, "it", source.language)
		require.Equal(t, "book-1", source.bookID)
	})
}

func TestExecuteIdiomCommandReportsAMissingSampleWithoutError(t *testing.T) {
	client := stubIdiomClient(t, func(idiomProviderRequestPayload) idiomStubAnswer {
		return idiomStubAnswer{translation: "review", gloss: "review"}
	})
	path := filepath.Join(t.TempDir(), "idiom-review.md")
	options, err := parseReviewCommand([]string{"-review", "idiom", "-out", path, "-date", "2026-11-08"})
	require.NoError(t, err)
	require.NoError(t, executeIdiomCommand(context.Background(), options, io.Discard,
		func() (reviewEnvironment, error) {
			return reviewEnvironment{Client: client, Provider: idiomTestProviderMetadata()}, nil
		},
		func(context.Context) (idiomSampleSource, error) {
			t.Fatal("a sample source must not be opened without -book")
			return nil, errors.New("unexpected sample source open")
		}))
	rendered, err := os.ReadFile(path) //nolint:gosec // path is inside t.TempDir().
	require.NoError(t, err)
	require.Contains(t, string(rendered), "- Sample: not requested; the rollout gate cannot pass without one")
}

func TestParseReviewCommandSelectsTheIdiomReviewAndDefaultsTheSeed(t *testing.T) {
	options, err := parseReviewCommand([]string{"-review", "idiom", "-book", "book-1", "-date", "2026-11-08"})
	require.NoError(t, err)
	require.Equal(t, reviewKindIdiom, options.Kind)
	require.Equal(t, "it", options.Language)
	require.Equal(t, "2026-11-08", options.Seed)

	_, err = parseReviewCommand([]string{"-review", "unexpected"})
	require.ErrorContains(t, err, "unknown review")
	_, err = parseReviewCommand([]string{"-review", "idiom", "-book", "book-1", "-language", " "})
	require.ErrorContains(t, err, "-language is required with -book")
}

func TestOpenIdiomSampleSourceRequiresADatabaseURL(t *testing.T) {
	t.Setenv("MOUSEION_DATABASE_URL", "")
	_, err := openIdiomSampleSource(context.Background())
	require.ErrorContains(t, err, "MOUSEION_DATABASE_URL is required")
}

func TestRunRejectsAnUnconfiguredIdiomProvider(t *testing.T) {
	t.Setenv("MOUSEION_LLM_ENABLED", "false")
	t.Setenv("MOUSEION_LLM_API_KEY", "")
	t.Setenv("MOUSEION_LLM_MODEL", "")
	require.ErrorContains(t, run(context.Background(), []string{"-review", "idiom"}, io.Discard), "no provider is configured")
}

func idiomTestProviderMetadata() providerMetadata {
	return providerMetadata{
		Model:           "gpt-review",
		ReasoningEffort: "low",
		ProviderName:    "openai-compatible",
		ProviderVersion: "gpt-review/idiom-card-v1-reasoning-low",
		PromptContract:  "idiom-card-v1",
	}
}

func idiomReviewTestCases() idiomReviewCaseSet {
	return idiomReviewCaseSet{
		Version: "idiom-review-test",
		Cases: []idiomReviewCase{
			{
				ID: "it-in-gamba", Language: "it", Kind: idiomCasePositive, Headword: "in gamba",
				MatchedForms: []string{"in", "gamba"}, Sentence: "È un ragazzo in gamba.", ExpectedGloss: "capable",
				Evidence: []enrichment.LexicalSense{{EvidenceID: "wikt:in gamba:1", Gloss: "capable, on the ball"}},
			},
			{
				ID: "el-kano", Language: "el", Kind: idiomCasePositive, Headword: "κάνω τα στραβά μάτια",
				MatchedForms: []string{"κάνω", "τα", "στραβά", "μάτια"}, Sentence: "Κάνω τα στραβά μάτια.", ExpectedGloss: "to turn a blind eye",
				Evidence: []enrichment.LexicalSense{{EvidenceID: "reviewed:kano:1", Gloss: "to turn a blind eye"}},
			},
			{
				ID: "it-tagliare-la-corda", Language: "it", Kind: idiomCaseLiteralUse, Headword: "tagliare la corda",
				MatchedForms: []string{"Tagliò", "la", "corda"}, Sentence: "Tagliò la corda del pacco.",
				Evidence: []enrichment.LexicalSense{{EvidenceID: "wikt:tagliare la corda:1", Gloss: "to run away"}},
			},
			{
				ID: "it-gamba-lemma-card", Language: "it", Kind: idiomCaseLemmaCard, Headword: "in gamba",
				AnchorLemma: "gamba", AnchorUPOS: "NOUN", AnchorForm: "gamba",
				IdiomSentence: "È un ragazzo in gamba.", LemmaCardSentence: "Mi fa male la gamba.",
			},
		},
	}
}

func idiomCaseIDs(cases idiomReviewCaseSet) []string {
	ids := make([]string, 0, len(cases.Cases))
	for _, reviewCase := range cases.Cases {
		ids = append(ids, reviewCase.ID)
	}
	return ids
}

func idiomResultsByCaseID(results []idiomCaseResult) map[string]idiomCaseResult {
	byID := make(map[string]idiomCaseResult, len(results))
	for _, result := range results {
		byID[result.Case.ID] = result
	}
	return byID
}

type idiomProviderRequestPayload struct {
	Language        string   `json:"language"`
	Headword        string   `json:"headword"`
	MatchedForms    []string `json:"matched_forms"`
	ExampleSentence string   `json:"example_sentence"`
}

type idiomStubAnswer struct {
	translation    string
	gloss          string
	unresolvedKind string
	reason         string
}

func stubIdiomClient(t *testing.T, answer func(idiomProviderRequestPayload) idiomStubAnswer) *enrichment.OpenAITranslationClient {
	t.Helper()
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		payload := decodeIdiomProviderRequest(t, request)
		reply := answer(payload)
		evidenceIDs, contextOnly := []string{"e1"}, false
		if reply.gloss == "" {
			evidenceIDs, contextOnly = []string{}, true
		}
		content, err := json.Marshal(map[string]any{
			"item_id":                      enrichment.IdiomCardItemHandle,
			"source_language":              payload.Language,
			"target_language":              "en",
			"translation":                  reply.translation,
			"sentence_translation":         "A sentence translation.",
			"sentence_translation_targets": []string{},
			"gloss":                        reply.gloss,
			"evidence_ids":                 evidenceIDs,
			"context_only":                 contextOnly,
			"unresolved_kind":              reply.unresolvedKind,
			"unresolved_reason":            reply.reason,
		})
		require.NoError(t, err)
		envelope, err := json.Marshal(map[string]any{
			"choices": []any{map[string]any{"message": map[string]string{"content": string(content)}}},
			"usage":   map[string]int{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
		})
		require.NoError(t, err)
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(envelope))}, nil
	})
	client, err := enrichment.NewOpenAITranslationClient(enrichment.LLMConfig{
		Enabled: true, APIKey: "test-key", Model: "gpt-review", BaseURL: "https://stub.example/v1",
		Timeout: time.Second, ReasoningEffort: "low", SupportsReasoningEffort: true,
	}, &http.Client{Transport: transport})
	require.NoError(t, err)
	return client
}

func decodeIdiomProviderRequest(t *testing.T, request *http.Request) idiomProviderRequestPayload {
	t.Helper()
	body, err := io.ReadAll(request.Body)
	require.NoError(t, err)
	var envelope struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	require.NoError(t, json.Unmarshal(body, &envelope))
	require.Len(t, envelope.Messages, 2)
	require.Equal(t, "system", envelope.Messages[0].Role)
	require.Contains(t, envelope.Messages[0].Content, "Confirm and translate the supplied idiom")
	var payload idiomProviderRequestPayload
	require.NoError(t, json.Unmarshal([]byte(envelope.Messages[1].Content), &payload))
	require.NotEmpty(t, payload.ExampleSentence, "the request carries the representative sentence")
	require.NotEmpty(t, payload.MatchedForms, "the request carries the matched forms")
	return payload
}

type fakeIdiomSampleSource struct {
	cards    []persistence.ConfirmedIdiomCard
	err      error
	closed   bool
	bookID   string
	language string
}

func (f *fakeIdiomSampleSource) ListConfirmedIdiomCardsForBook(_ context.Context, bookID, language string) ([]persistence.ConfirmedIdiomCard, error) {
	f.bookID, f.language = bookID, language
	return f.cards, f.err
}

func (f *fakeIdiomSampleSource) Close() error {
	f.closed = true
	return nil
}

func sampleCards(count int) []persistence.ConfirmedIdiomCard {
	cards := make([]persistence.ConfirmedIdiomCard, 0, count)
	for i := range count {
		cards = append(cards, persistence.ConfirmedIdiomCard{
			Language: "it", Headword: "idiom-" + strings.Repeat("x", i%5) + "-" + string(rune('a'+i)),
			MatchedWords: []string{"in", "gamba"}, Sentence: "Frase numero " + string(rune('a'+i)) + ".",
			Gloss: "capable", Translation: "capable", AnchorLemma: "gamba", AnchorUPOS: "NOUN",
			MatcherVersion: "idiom-matcher/1", GapBound: 2,
		})
	}
	return cards
}
