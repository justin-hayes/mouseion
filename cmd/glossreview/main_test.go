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
	"github.com/stretchr/testify/require"
)

func TestEmbeddedCaseSetCoversEveryReviewClassAndLanguage(t *testing.T) {
	cases, err := loadCaseSet(bytes.NewReader(glossReviewCases))
	require.NoError(t, err)

	t.Run("version is pinned", func(t *testing.T) {
		require.Equal(t, "gloss-review-v1", cases.Version)
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
	t.Run("every verdict class is present", func(t *testing.T) {
		seen := map[verdictClass]bool{}
		for _, reviewCase := range cases.Cases {
			seen[reviewCase.ExpectedVerdictClass] = true
		}
		for _, class := range reviewVerdictClasses {
			require.True(t, seen[class], "case set has no %s case", class)
		}
	})
	t.Run("seed cases are present", func(t *testing.T) {
		ids := caseIDs(cases)
		for _, want := range []string{"de-sehen-fern", "it-palazzo", "el-tha"} {
			require.Contains(t, ids, want)
		}
	})
	t.Run("cases carry evidence and an expected Gloss", func(t *testing.T) {
		for _, reviewCase := range cases.Cases {
			t.Run(reviewCase.ID, func(t *testing.T) {
				require.NotEmpty(t, reviewCase.Evidence, "case has no frozen evidence")
				if reviewCase.ExpectedVerdictClass != verdictLemmaMismatch {
					require.NotEmpty(t, reviewCase.ExpectedGloss, "case has no expected Gloss")
				}
			})
		}
	})
}

func TestLoadCaseSetRejectsUnknownFieldsAndMissingCoverage(t *testing.T) {
	valid := `{"version":"v1","cases":[{"id":"de-kalt","language":"de","lemma":"kalt","upos":"ADJ","target_form":"kalt","sentence":"Es ist kalt.","evidence":[{"evidence_id":"e1","gloss":"cold"}],"expected_verdict_class":"plain_equivalent"}]}`
	for name, testCase := range map[string]struct {
		input string
		want  string
	}{
		"unknown field":    {input: strings.Replace(valid, `"lemma":"kalt"`, `"lemma":"kalt","unexpected":true`, 1), want: "unknown field"},
		"missing language": {input: valid, want: "no it case"},
		"no cases":         {input: `{"version":"v1","cases":[]}`, want: "no cases"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := loadCaseSet(strings.NewReader(testCase.input))
			require.ErrorContains(t, err, testCase.want)
		})
	}
}

func TestReviewWritesReportThroughStubbedProviderTransport(t *testing.T) {
	var calls int
	client := stubClientForCases(t, func(payload providerRequestPayload) (string, string) {
		calls++
		switch payload.CanonicalLemma {
		case "kalt":
			return "cold", "having a low temperature"
		case "palazzo":
			return "apartment building", "apartment building"
		default:
			return "to see", "to watch television"
		}
	})
	report := review(context.Background(), reviewOptions{
		Now:                      time.Date(2026, 10, 29, 12, 0, 0, 0, time.UTC),
		Cases:                    reviewTestCases(),
		Client:                   client,
		Provider:                 reviewTestProviderMetadata(),
		MismatchOutcomeSupported: false,
	})
	require.Len(t, report.Cases, 3)
	var providerErrors []error
	for _, result := range report.Cases {
		providerErrors = append(providerErrors, result.Err)
	}
	require.NoError(t, errors.Join(providerErrors...))

	t.Run("one provider request per case", func(t *testing.T) {
		require.Equal(t, 3, calls)
	})
	t.Run("resolved outcome", func(t *testing.T) {
		require.Equal(t, outcomeGlossed, report.Cases[0].Outcome)
	})
	t.Run("definition paraphrase fails equality", func(t *testing.T) {
		require.Equal(t, checkFail, automaticCheckStatus(report.Cases[0], checkNameGlossEqualsTranslation))
	})
	t.Run("protected win passes equality", func(t *testing.T) {
		require.Equal(t, checkPass, automaticCheckStatus(report.Cases[1], checkNameGlossEqualsTranslation))
	})
	t.Run("v15 cannot report a mismatch", func(t *testing.T) {
		require.Equal(t, checkNotSupported, automaticCheckStatus(report.Cases[2], checkNameLemmaMismatch))
	})
	t.Run("summary", func(t *testing.T) {
		require.Equal(t, reviewSummary{Total: 3, Glossed: 3, EqualsPass: 1, EqualsFail: 2, ContainsTotal: 3, ContainsGloss: 1, MismatchStatus: checkNotSupported, MismatchDetail: "not supported by this contract"}, report.Summary)
	})

	var output bytes.Buffer
	require.NoError(t, writeReport(&output, report))
	rendered := output.String()
	t.Run("report shape", func(t *testing.T) {
		for name, fragment := range map[string]string{
			"title":                 "# Contextual Gloss review — `translation-v15-short-handles`",
			"date":                  "- Date: 2026-10-29",
			"model":                 "- Model: `gpt-review`",
			"reasoning setting":     "- Reasoning setting: `low`",
			"prompt contract":       "- Prompt contract: `translation-v15-short-handles`",
			"case set":              "- Case set: `gloss-review-test` (3 cases)",
			"case heading":          "## `de-kalt` — de `kalt` (`ADJ`)",
			"sentence":              "- Representative sentence: `Das Wasser war kalt.`",
			"candidate senses":      "| candidate_senses | `e1 wikt:kalt:1: cold, chilly; having a low temperature` |",
			"raw gloss":             "- Raw Gloss: `having a low temperature`",
			"translation":           "- Translation: `cold`",
			"equality failure":      "| Gloss equals translation | **FAIL**",
			"unsupported mismatch":  "| outcome is `lemma_mismatch` | **not supported by this contract** |",
			"citation verdict":      "- [ ] Citation form — verdict: __________",
			"paraphrase verdict":    "- [ ] No definition paraphrase — verdict: __________",
			"inflection verdict":    "- [ ] No sentence inflection or arguments — verdict: __________",
			"protected win verdict": "- [ ] Protected win kept — verdict: __________",
			"contains rate":         "Full review-corpus \"Gloss contains translation\" rate: 1/3 (33.3%)",
		} {
			t.Run(name, func(t *testing.T) {
				require.Contains(t, rendered, fragment)
			})
		}
	})
	t.Run("no API key", func(t *testing.T) {
		require.NotContains(t, rendered, "test-key")
	})
}

func TestClassifyOutcomeSeparatesValidationAndTransportFailures(t *testing.T) {
	for name, testCase := range map[string]struct {
		result caseResult
		want   outcomeKind
	}{
		"invalid response": {result: caseResult{Err: enrichment.ErrInvalidTranslationResponse}, want: outcomeInvalid},
		"transport error":  {result: caseResult{Err: io.ErrUnexpectedEOF}, want: outcomeProviderError},
		"abstained":        {result: caseResult{Response: enrichment.TranslationResponse{ContextOnly: true}}, want: outcomeAbstained},
		"unresolved":       {result: caseResult{}, want: outcomeUnresolved},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, testCase.want, classifyOutcome(testCase.result))
		})
	}
}

func TestRunRefusesAnUnconfiguredProviderAndInvalidDate(t *testing.T) {
	t.Setenv("MOUSEION_LLM_ENABLED", "false")
	t.Setenv("MOUSEION_LLM_API_KEY", "")
	t.Setenv("MOUSEION_LLM_MODEL", "")
	for name, testCase := range map[string]struct {
		args []string
		want string
	}{
		"invalid date":          {args: []string{"-date", "29-10-2026"}, want: "must be YYYY-MM-DD"},
		"unconfigured provider": {args: nil, want: "no provider is configured"},
		"extra argument":        {args: []string{"unexpected"}, want: usage},
	} {
		t.Run(name, func(t *testing.T) {
			require.ErrorContains(t, run(context.Background(), testCase.args, io.Discard), testCase.want)
		})
	}
}

func TestRunWithEnvironmentWritesTheDatedReportAndSummary(t *testing.T) {
	client := stubClientForCases(t, func(providerRequestPayload) (string, string) {
		return "review", "review"
	})
	path := filepath.Join(t.TempDir(), "report.md")
	var output bytes.Buffer
	environment := reviewEnvironment{Client: client, Provider: reviewTestProviderMetadata()}
	require.NoError(t, runWithEnvironment(context.Background(), []string{"-out", path, "-date", "2026-10-29"}, &output, func() (reviewEnvironment, error) {
		return environment, nil
	}))
	rendered, err := os.ReadFile(path) //nolint:gosec // path is inside t.TempDir().
	require.NoError(t, err)

	for name, fragment := range map[string]string{
		"report title":  "# Contextual Gloss review — `translation-v15-short-handles`",
		"report date":   "- Date: 2026-10-29",
		"full case set": "- Case set: `gloss-review-v1` (20 cases)",
		"contains rate": "Full review-corpus \"Gloss contains translation\" rate: 20/20 (100.0%)",
	} {
		t.Run(name, func(t *testing.T) {
			require.Contains(t, string(rendered), fragment)
		})
	}
	t.Run("stdout", func(t *testing.T) {
		require.Contains(t, output.String(), "gloss review report written: path="+path+" cases=20")
	})
}

func TestRunWithEnvironmentRejectsAnEnvironmentOpenFailure(t *testing.T) {
	openErr := errors.New("provider unavailable")
	err := runWithEnvironment(context.Background(), nil, io.Discard, func() (reviewEnvironment, error) {
		return reviewEnvironment{}, openErr
	})
	require.ErrorIs(t, err, openErr)
}

func TestLemmaMismatchCheckOnceTheContractSupportsIt(t *testing.T) {
	mismatchCase := reviewCase{ID: "sehen", ExpectedVerdictClass: verdictLemmaMismatch}
	ordinaryCase := reviewCase{ID: "kalt", ExpectedVerdictClass: verdictPlainEquivalent}
	for name, testCase := range map[string]struct {
		reviewCase reviewCase
		result     caseResult
		supported  bool
		want       checkStatus
	}{
		"expected mismatch fails until observed":  {reviewCase: mismatchCase, result: caseResult{Outcome: outcomeGlossed}, supported: true, want: checkFail},
		"ordinary case passes without a mismatch": {reviewCase: ordinaryCase, result: caseResult{Outcome: outcomeGlossed}, supported: true, want: checkPass},
		"v15 reports unsupported":                 {reviewCase: mismatchCase, supported: false, want: checkNotSupported},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, testCase.want, lemmaMismatchCheck(testCase.reviewCase, testCase.result, testCase.supported).Status)
		})
	}
}

func TestSummarizeCountsEveryOutcomeAndReportsAFailedMismatchCheck(t *testing.T) {
	cases := []caseResult{
		{Outcome: outcomeGlossed, Response: enrichment.TranslationResponse{Translation: "cold", Gloss: "cold"}, Checks: []automaticCheck{{Name: checkNameGlossEqualsTranslation, Status: checkPass}, {Name: checkNameLemmaMismatch, Status: checkPass}}},
		{Outcome: outcomeGlossed, Response: enrichment.TranslationResponse{Translation: "car", Gloss: "vehicles"}, Checks: []automaticCheck{{Name: checkNameGlossEqualsTranslation, Status: checkFail}, {Name: checkNameLemmaMismatch, Status: checkFail}}},
		{Outcome: outcomeAbstained, Checks: []automaticCheck{{Name: checkNameGlossEqualsTranslation, Status: checkNotApplicable}, {Name: checkNameLemmaMismatch, Status: checkPass}}},
		{Outcome: outcomeUnresolved, Checks: []automaticCheck{{Name: checkNameGlossEqualsTranslation, Status: checkNotApplicable}, {Name: checkNameLemmaMismatch, Status: checkPass}}},
		{Outcome: outcomeInvalid, Checks: []automaticCheck{{Name: checkNameGlossEqualsTranslation, Status: checkNotApplicable}, {Name: checkNameLemmaMismatch, Status: checkPass}}},
		{Outcome: outcomeProviderError, Checks: []automaticCheck{{Name: checkNameGlossEqualsTranslation, Status: checkNotApplicable}, {Name: checkNameLemmaMismatch, Status: checkPass}}},
	}
	require.Equal(t, reviewSummary{
		Total: 6, Glossed: 2, Abstained: 1, Unresolved: 1, InvalidResponse: 1, ProviderError: 1,
		EqualsPass: 1, EqualsFail: 1, EqualsNotApplicable: 4,
		ContainsGloss: 1, ContainsTotal: 2,
		MismatchStatus: checkFail,
	}, summarize(cases, true))
}

func TestCaseValidationRejectsIncompleteCases(t *testing.T) {
	base := reviewCase{
		ID: "de-kalt", Language: "de", Lemma: "kalt", UPOS: "ADJ", TargetForm: "kalt",
		Sentence: "Es ist kalt.", ExpectedVerdictClass: verdictPlainEquivalent,
		Evidence: []enrichment.LexicalSense{{EvidenceID: "e1", Gloss: "cold"}},
	}
	for name, mutate := range map[string]func(*reviewCase){
		"empty id":               func(c *reviewCase) { c.ID = "" },
		"unsupported language":   func(c *reviewCase) { c.Language = "fr" },
		"empty lemma":            func(c *reviewCase) { c.Lemma = "" },
		"empty upos":             func(c *reviewCase) { c.UPOS = "" },
		"empty target form":      func(c *reviewCase) { c.TargetForm = "" },
		"empty sentence":         func(c *reviewCase) { c.Sentence = "" },
		"no evidence":            func(c *reviewCase) { c.Evidence = nil },
		"unknown verdict":        func(c *reviewCase) { c.ExpectedVerdictClass = "unexpected" },
		"missing evidence id":    func(c *reviewCase) { c.Evidence[0].EvidenceID = "" },
		"missing evidence gloss": func(c *reviewCase) { c.Evidence[0].Gloss = "" },
		"duplicate evidence":     func(c *reviewCase) { c.Evidence = append(c.Evidence, c.Evidence[0]) },
	} {
		t.Run(name, func(t *testing.T) {
			reviewCase := base
			reviewCase.Evidence = append([]enrichment.LexicalSense(nil), base.Evidence...)
			mutate(&reviewCase)
			require.Error(t, reviewCase.validate())
		})
	}
}

func TestVerdictClassLabels(t *testing.T) {
	for name, testCase := range map[string]struct {
		class verdictClass
		want  string
	}{
		"plain equivalent":       {class: verdictPlainEquivalent, want: "plain equivalent"},
		"drop sentence material": {class: verdictDropSentenceMaterial, want: "drop sentence material"},
		"protected win":          {class: verdictProtectedWin, want: "protected contextual win"},
		"lemma mismatch":         {class: verdictLemmaMismatch, want: "lemma mismatch"},
		"spot check":             {class: verdictSpotCheck, want: "spot check"},
		"unknown":                {class: "unknown", want: "unknown"},
	} {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, testCase.want, testCase.class.label())
		})
	}
}

func reviewTestProviderMetadata() providerMetadata {
	return providerMetadata{
		Model:           "gpt-review",
		ReasoningEffort: "low",
		ProviderName:    "openai-compatible",
		ProviderVersion: "gpt-review/translation-v15-short-handles-reasoning-low",
		PromptContract:  "translation-v15-short-handles",
	}
}

func reviewTestCases() reviewCaseSet {
	return reviewCaseSet{
		Version: "gloss-review-test",
		Cases: []reviewCase{
			{
				ID: "de-kalt", Language: "de", Lemma: "kalt", UPOS: "ADJ", TargetForm: "kalt",
				Sentence: "Das Wasser war kalt.", ExpectedVerdictClass: verdictPlainEquivalent, ExpectedGloss: "cold",
				Evidence: []enrichment.LexicalSense{{EvidenceID: "wikt:kalt:1", Gloss: "cold, chilly; having a low temperature"}},
			},
			{
				ID: "it-palazzo", Language: "it", Lemma: "palazzo", UPOS: "NOUN", TargetForm: "palazzo",
				Sentence: "Abitano in un palazzo del centro.", ExpectedVerdictClass: verdictProtectedWin, ExpectedGloss: "apartment building",
				Evidence: []enrichment.LexicalSense{{EvidenceID: "wikt:palazzo:1", Gloss: "apartment building, block of flats"}},
			},
			{
				ID: "de-sehen-fern", Language: "de", Lemma: "sehen", UPOS: "VERB", TargetForm: "sieht",
				Sentence: "Großmutter sieht im Wohnzimmer fern ohne Bild und Ton.", ExpectedVerdictClass: verdictLemmaMismatch,
				Evidence: []enrichment.LexicalSense{{EvidenceID: "wikt:sehen:1", Gloss: "to see, to look"}},
			},
		},
	}
}

func caseIDs(cases reviewCaseSet) []string {
	ids := make([]string, 0, len(cases.Cases))
	for _, reviewCase := range cases.Cases {
		ids = append(ids, reviewCase.ID)
	}
	return ids
}

type providerRequestPayload struct {
	Language        string `json:"language"`
	CanonicalLemma  string `json:"canonical_lemma"`
	TargetWord      string `json:"target_word"`
	ExampleSentence string `json:"example_sentence"`
}

func decodeProviderRequest(t *testing.T, request *http.Request) providerRequestPayload {
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
	require.Contains(t, envelope.Messages[0].Content, "complete representative sentence")
	var payload providerRequestPayload
	require.NoError(t, json.Unmarshal([]byte(envelope.Messages[1].Content), &payload))
	require.NotEmpty(t, payload.ExampleSentence, "the request carries the representative sentence")
	return payload
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func stubClientForCases(t *testing.T, answer func(providerRequestPayload) (string, string)) *enrichment.OpenAITranslationClient {
	t.Helper()
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		payload := decodeProviderRequest(t, request)
		translation, gloss := answer(payload)
		content, err := json.Marshal(map[string]any{
			"item_id":                      enrichment.ContextualGlossItemHandle,
			"source_language":              payload.Language,
			"target_language":              "en",
			"translation":                  translation,
			"sentence_translation":         "A sentence translation.",
			"sentence_translation_targets": []string{},
			"gloss":                        gloss,
			"evidence_ids":                 []string{"e1"},
			"context_only":                 false,
			"unresolved_reason":            "",
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
