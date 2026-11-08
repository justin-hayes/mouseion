package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/prepareddeck"
	"github.com/stretchr/testify/require"
)

func TestEmbeddedIdiomCaseSetCoversEveryKindAndLanguage(t *testing.T) {
	cases, err := loadIdiomCaseSet(bytes.NewReader(idiomReviewCases))
	require.NoError(t, err)

	t.Run("version is pinned", func(t *testing.T) {
		require.Equal(t, "idiom-review-v3", cases.Version)
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
			"it-di-fronte", "it-rispetto-a-comparative", "it-fare-bene-literal",
			"it-in-giro", "it-casa-isolata-remote",
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
	t.Run("the 2026-11-08 review failures carry their expected outcomes", func(t *testing.T) {
		byID := map[string]idiomReviewCase{}
		for _, reviewCase := range cases.Cases {
			byID[reviewCase.ID] = reviewCase
		}
		t.Run("di fronte", func(t *testing.T) {
			require.Equal(t, idiomCasePositive, byID["it-di-fronte"].Kind)
			require.Len(t, byID["it-di-fronte"].Evidence, 4, "di fronte carries the pooled adjective and adverb senses")
		})
		t.Run("rispetto a", func(t *testing.T) {
			require.Equal(t, idiomCaseAmbiguous, byID["it-rispetto-a-comparative"].Kind)
		})
		t.Run("fare bene", func(t *testing.T) {
			require.Equal(t, idiomCaseLiteralUse, byID["it-fare-bene-literal"].Kind)
			require.Len(t, byID["it-fare-bene-literal"].Evidence, 3, "fare bene keeps its literal-use marker sense")
		})
	})
	t.Run("the 2026-11-08 v2 re-run failures carry their expected outcomes", func(t *testing.T) {
		byID := map[string]idiomReviewCase{}
		for _, reviewCase := range cases.Cases {
			byID[reviewCase.ID] = reviewCase
		}
		t.Run("in giro leaves out the sentence verb", func(t *testing.T) {
			require.Equal(t, idiomCasePositive, byID["it-in-giro"].Kind)
			require.Subset(t, byID["it-in-giro"].GlossExcludes, []string{"go", "andare"})
		})
		t.Run("casa isolata has no fitting sense", func(t *testing.T) {
			require.Equal(t, idiomCaseAmbiguous, byID["it-casa-isolata-remote"].Kind)
			require.Len(t, byID["it-casa-isolata-remote"].Evidence, 2, "only the detached-house and homestead senses")
		})
	})
	t.Run("literal-use and ambiguous controls expect no Gloss", func(t *testing.T) {
		for _, reviewCase := range cases.Cases {
			if reviewCase.Kind != idiomCaseLiteralUse && reviewCase.Kind != idiomCaseAmbiguous {
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
		"evidence without an ID": {input: strings.Replace(valid, `{"evidence_id":"e1","gloss":"leg"}`, `{"gloss":"leg"}`, 1), want: "evidence id is required"},
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
		require.Equal(t, checkPass, idiomCheckStatus(byID["it-tagliare-la-corda"], checkNameIdiomOmitted))
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
		"omitted check":    "| card is omitted (`literal_use` or `ambiguous`) | **PASS** |",
		"lemma check":      "| lemma card moves off the idiom sentence | **PASS** |",
		"citation verdict": "- [ ] Citation form — verdict: __________",
		"meaning verdict":  "- [ ] Meaning in context (correct, near-miss or misleading) — verdict: __________",
		"gate misleading":  "- no card, in a control or the sample, teaches a misleading meaning;",
		"gate near-miss":   "- at most 3 cards, across the controls and the sample, are near-misses.",
		"gate grades":      "A control whose card is omitted, as `literal_use` or `ambiguous`, passes.",
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
		require.Equal(t, checkFail, idiomCheckStatus(result, checkNameIdiomOmitted))
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

func TestOrderIdiomSampleIsDeterministicForBookAndSeed(t *testing.T) {
	items := sampleItems(40)

	first := orderIdiomSample(items, "book-1", "seed-1")
	second := orderIdiomSample(items, "book-1", "seed-1")
	otherSeed := orderIdiomSample(items, "book-1", "seed-2")
	otherBook := orderIdiomSample(items, "book-2", "seed-1")

	t.Run("every item is kept", func(t *testing.T) {
		require.Len(t, first, 40)
	})
	t.Run("same Book and seed give the same order", func(t *testing.T) {
		require.Equal(t, first, second)
	})
	t.Run("a different seed changes the order", func(t *testing.T) {
		require.NotEqual(t, first, otherSeed)
	})
	t.Run("a different Book changes the order", func(t *testing.T) {
		require.NotEqual(t, first, otherBook)
	})
	t.Run("input order does not matter", func(t *testing.T) {
		reversed := slices.Clone(items)
		slices.Reverse(reversed)
		require.Equal(t, first, orderIdiomSample(reversed, "book-1", "seed-1"))
	})
	t.Run("a repeated headword in the same sentence is one item", func(t *testing.T) {
		duplicated := append(slices.Clone(items), items[0])
		require.Len(t, orderIdiomSample(duplicated, "book-1", "seed-1"), 40)
	})
}

func TestRunIdiomSampleSendsItemsUntilEnoughAreConfirmed(t *testing.T) {
	var sent int
	client := stubIdiomClient(t, func(payload idiomProviderRequestPayload) idiomStubAnswer {
		sent++
		if strings.HasSuffix(payload.Headword, "-literal") {
			return idiomStubAnswer{translation: "literal", unresolvedKind: string(enrichment.ContextualGlossUnresolvedLiteralUse), reason: "The words are literal."}
		}
		return idiomStubAnswer{translation: "capable", gloss: "capable"}
	})
	items := sampleItems(40)
	for i := range 6 {
		items[i].Headword += "-literal"
		items[i].Work.Request.IdiomHeadword = items[i].Headword
	}
	options := idiomSampleOptions{Requested: true, BookID: "book-1", Language: "it", Seed: "seed-1", Items: items}

	report := runIdiomSample(context.Background(), client, options)

	t.Run("stops at the reviewed sample size", func(t *testing.T) {
		require.Len(t, report.Cards, idiomSampleSize)
		require.Equal(t, report.Sent(), sent)
		require.Less(t, report.Sent(), 40)
	})
	t.Run("counts every item the freeze found", func(t *testing.T) {
		require.Equal(t, 40, report.Found)
	})
	t.Run("lists the unconfirmed items it sent", func(t *testing.T) {
		require.NotEmpty(t, report.Unconfirmed)
		for _, result := range report.Unconfirmed {
			require.True(t, strings.HasSuffix(result.Item.Headword, "-literal"))
			require.Equal(t, idiomOutcomeLiteralUse, result.Outcome)
		}
	})
	t.Run("confirmed cards keep the seeded order", func(t *testing.T) {
		ordered := orderIdiomSample(items, "book-1", "seed-1")
		var confirmed []cardexport.IdiomWorkItem
		for _, item := range ordered {
			if !strings.HasSuffix(item.Headword, "-literal") {
				confirmed = append(confirmed, item)
			}
		}
		for i, card := range report.Cards {
			require.Equal(t, confirmed[i].Headword, card.Item.Headword)
		}
	})
	t.Run("a short Book is not padded", func(t *testing.T) {
		short := options
		short.Items = items[6:13]
		require.Len(t, runIdiomSample(context.Background(), client, short).Cards, 7)
	})
}

func TestExecuteIdiomCommandWritesTheDatedReportWithASample(t *testing.T) {
	client := stubIdiomClient(t, func(payload idiomProviderRequestPayload) idiomStubAnswer {
		if strings.Contains(payload.Headword, "tagliare") || strings.Contains(payload.Headword, "Arm nehmen") {
			return idiomStubAnswer{translation: "literal", unresolvedKind: string(enrichment.ContextualGlossUnresolvedLiteralUse), reason: "The words are literal."}
		}
		return idiomStubAnswer{translation: "review", gloss: "review"}
	})
	source := &fakeIdiomSampleSource{items: sampleItems(35)}
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
		func(_ context.Context, codec *enrichment.TranslationCodec) (idiomSampleSource, error) {
			require.Nil(t, codec, "the test environment supplies no codec")
			return source, nil
		}))

	rendered, err := os.ReadFile(path) //nolint:gosec // path is inside t.TempDir().
	require.NoError(t, err)
	for name, fragment := range map[string]string{
		"report title":   "# Idiom card review — `idiom-card-v1`",
		"report date":    "- Date: 2026-11-08",
		"full case set":  "- Case set: `idiom-review-v3` (13 cases)",
		"matcher":        "- Matcher version: `idiom-matcher/2`",
		"sample header":  "- Sample: 30 confirmed idiom cards requested from Book `book-1` in `it` (seed `seed-1`); 35 idiom items found, 30 sent, 30 confirmed",
		"in-memory note": "re-freezes the Book's current deck preparation in memory",
		"confirmed":      "### Confirmed",
		"sample heading": "#### 1. `idiom",
		"gate":           "- every positive case is confirmed with a citation-form Gloss;",
	} {
		t.Run(name, func(t *testing.T) {
			require.Contains(t, string(rendered), fragment)
		})
	}
	t.Run("stdout", func(t *testing.T) {
		require.Equal(t, "idiom review report written: path="+path+" cases=13 sample=30\n", output.String())
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
		func(context.Context, *enrichment.TranslationCodec) (idiomSampleSource, error) {
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

// A sample item whose candidate senses carry no evidence IDs cannot be
// confirmed under the idiom contract, so the review refuses it before any
// provider call rather than reporting it as unconfirmed (#2385).
func TestExecuteIdiomCommandRefusesASampleWithoutEvidenceIDs(t *testing.T) {
	calls := 0
	client := stubIdiomClient(t, func(idiomProviderRequestPayload) idiomStubAnswer {
		calls++
		return idiomStubAnswer{translation: "review", gloss: "review"}
	})
	items := sampleItems(3)
	items[1].Work.Request.CandidateSenses = []enrichment.LexicalSense{{EvidenceID: "wikt:in gamba:1", Gloss: "capable"}, {Gloss: "in good health"}}
	source := &fakeIdiomSampleSource{items: items}
	path := filepath.Join(t.TempDir(), "idiom-review.md")
	options, err := parseReviewCommand([]string{"-review", "idiom", "-out", path, "-book", "book-1"})
	require.NoError(t, err)

	err = executeIdiomCommand(context.Background(), options, io.Discard,
		func() (reviewEnvironment, error) {
			return reviewEnvironment{Client: client, Provider: idiomTestProviderMetadata()}, nil
		},
		func(context.Context, *enrichment.TranslationCodec) (idiomSampleSource, error) { return source, nil })

	require.ErrorContains(t, err, `idiom "idiom-01"`)
	require.ErrorContains(t, err, "candidate sense 2 has no evidence ID")
	require.Zero(t, calls, "no provider call is made")
	require.NoFileExists(t, path)
	require.True(t, source.closed)
}

func TestExecuteIdiomCommandReportsAFailedSampleFreeze(t *testing.T) {
	client := stubIdiomClient(t, func(idiomProviderRequestPayload) idiomStubAnswer {
		return idiomStubAnswer{translation: "review", gloss: "review"}
	})
	source := &fakeIdiomSampleSource{err: errors.New("freeze failed")}
	options, err := parseReviewCommand([]string{"-review", "idiom", "-out", filepath.Join(t.TempDir(), "r.md"), "-book", "book-1"})
	require.NoError(t, err)
	err = executeIdiomCommand(context.Background(), options, io.Discard,
		func() (reviewEnvironment, error) {
			return reviewEnvironment{Client: client, Provider: idiomTestProviderMetadata()}, nil
		},
		func(context.Context, *enrichment.TranslationCodec) (idiomSampleSource, error) { return source, nil })
	require.ErrorContains(t, err, "freeze failed")
	require.True(t, source.closed)
}

func TestAnInterruptedIdiomSampleWritesNoReport(t *testing.T) {
	client := stubIdiomClient(t, func(idiomProviderRequestPayload) idiomStubAnswer {
		return idiomStubAnswer{translation: "review", gloss: "review"}
	})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	t.Run("no item is sent after cancellation", func(t *testing.T) {
		options := idiomSampleOptions{Requested: true, BookID: "book-1", Language: "it", Seed: "seed-1", Items: sampleItems(5)}
		report := runIdiomSample(ctx, client, options)
		require.Zero(t, report.Sent())
		require.Equal(t, 5, report.Found)
	})
	t.Run("the command returns the cancellation and writes nothing", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "idiom-review.md")
		options, err := parseReviewCommand([]string{"-review", "idiom", "-out", path, "-book", "book-1"})
		require.NoError(t, err)
		err = executeIdiomCommand(ctx, options, io.Discard,
			func() (reviewEnvironment, error) {
				return reviewEnvironment{Client: client, Provider: idiomTestProviderMetadata()}, nil
			},
			func(context.Context, *enrichment.TranslationCodec) (idiomSampleSource, error) {
				return &fakeIdiomSampleSource{items: sampleItems(5)}, nil
			})
		require.ErrorIs(t, err, context.Canceled)
		require.NoFileExists(t, path)
	})
}

func TestWriteIdiomSampleReportsUnconfirmedAndShortSamples(t *testing.T) {
	items := sampleItems(2)
	sample := idiomSampleReport{
		Requested: true, BookID: "book-1", Language: "it", Seed: "seed-1", Limit: idiomSampleSize, Found: 2,
		Cards: []idiomSampleResult{{Item: items[0], Outcome: idiomOutcomeConfirmed, Response: enrichment.TranslationResponse{Gloss: "capable"}}},
		Unconfirmed: []idiomSampleResult{{
			Item: items[1], Outcome: idiomOutcomeLiteralUse, Err: errors.New("provider said no"),
			Response: enrichment.TranslationResponse{UnresolvedReason: "The words are literal."},
		}},
	}
	var builder strings.Builder
	writeIdiomSampleCards(&builder, sample)
	for name, fragment := range map[string]string{
		"short":       "Only 1 idiom items were confirmed; the gate requires 30.",
		"heading":     "### Not confirmed",
		"count":       "The provider did not confirm 1 sampled items.",
		"outcome":     "- Outcome: `literal_use`",
		"error":       "- Error: `provider said no`",
		"reason":      "- Unresolved reason: `The words are literal.`",
		"anchor":      "— `gamba` (`NOUN`)",
		"matched":     "- Matched words: `in, gamba`",
		"translation": "- Gloss: `capable`",
	} {
		t.Run(name, func(t *testing.T) {
			require.Contains(t, builder.String(), fragment)
		})
	}
	t.Run("nothing found", func(t *testing.T) {
		var empty strings.Builder
		writeIdiomSampleCards(&empty, idiomSampleReport{Requested: true, Limit: idiomSampleSize})
		require.Contains(t, empty.String(), "The freeze matched no idiom items for this Book and language.")
	})
}

func TestOpenIdiomSampleSourceRequiresItsConfiguration(t *testing.T) {
	codec, err := enrichment.NewTranslationCodec(enrichment.LLMConfig{Model: "review-model"})
	require.NoError(t, err)
	t.Run("database URL", func(t *testing.T) {
		t.Setenv("MOUSEION_DATABASE_URL", "")
		_, openErr := openIdiomSampleSource(context.Background(), codec)
		require.ErrorContains(t, openErr, "MOUSEION_DATABASE_URL is required")
	})
	t.Run("dictionary index", func(t *testing.T) {
		t.Setenv("MOUSEION_DATABASE_URL", "postgres://unused")
		t.Setenv("MOUSEION_DICTIONARY_INDEX", " ")
		_, openErr := openIdiomSampleSource(context.Background(), codec)
		require.ErrorContains(t, openErr, "MOUSEION_DICTIONARY_INDEX is required")
	})
	t.Run("codec", func(t *testing.T) {
		t.Setenv("MOUSEION_DATABASE_URL", "postgres://unused")
		t.Setenv("MOUSEION_DICTIONARY_INDEX", "/dictionary.sqlite")
		_, openErr := openIdiomSampleSource(context.Background(), nil)
		require.ErrorContains(t, openErr, "provider codec")
	})
	t.Run("missing index", func(t *testing.T) {
		t.Setenv("MOUSEION_DATABASE_URL", "postgres://unused")
		t.Setenv("MOUSEION_DICTIONARY_INDEX", filepath.Join(t.TempDir(), "absent.sqlite"))
		_, openErr := openIdiomSampleSource(context.Background(), codec)
		require.ErrorContains(t, openErr, "open dictionary index")
	})
	t.Run("index without idiom tables", func(t *testing.T) {
		t.Setenv("MOUSEION_DATABASE_URL", "postgres://unused")
		t.Setenv("MOUSEION_DICTIONARY_INDEX", writeIndexWithoutIdioms(t))
		_, openErr := openIdiomSampleSource(context.Background(), codec)
		require.ErrorContains(t, openErr, "has no idiom lookup tables")
	})
}

// writeIndexWithoutIdioms writes a dictionary index from before the idiom
// lookup tables existed.
func writeIndexWithoutIdioms(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "dictionary.sqlite")
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	_, err = db.ExecContext(t.Context(), `CREATE TABLE metadata (key TEXT PRIMARY KEY, value TEXT NOT NULL); CREATE TABLE entries (language TEXT NOT NULL, lemma TEXT NOT NULL, upos TEXT NOT NULL, senses_json TEXT NOT NULL, gender TEXT NOT NULL, article TEXT NOT NULL, plural TEXT NOT NULL, ipa TEXT NOT NULL, principal_parts TEXT NOT NULL, PRIMARY KEY(language, lemma, upos)); INSERT INTO metadata VALUES ('provider_version', 'fixture-v1')`)
	require.NoError(t, err)
	require.NoError(t, db.Close())
	return path
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
	items    []cardexport.IdiomWorkItem
	ranking  []cardexport.IdiomRank
	err      error
	closed   bool
	bookID   string
	language string
}

func (f *fakeIdiomSampleSource) FreezeIdiomReviewItems(_ context.Context, bookID, language string) (prepareddeck.IdiomReviewFreeze, error) {
	f.bookID, f.language = bookID, language
	return prepareddeck.IdiomReviewFreeze{Items: f.items, Ranking: f.ranking}, f.err
}

func (f *fakeIdiomSampleSource) Close() error {
	f.closed = true
	return nil
}

// sampleItems builds distinct Italian idiom items as an in-memory freeze would
// return them.
func sampleItems(count int) []cardexport.IdiomWorkItem {
	items := make([]cardexport.IdiomWorkItem, 0, count)
	for i := range count {
		headword := fmt.Sprintf("idiom-%02d", i)
		items = append(items, cardexport.IdiomWorkItem{
			Headword: headword,
			Evidence: cardexport.IdiomEvidence{AnchorCanonicalLemma: "gamba", AnchorUPOS: "NOUN"},
			Work: cardexport.WorkItem{Ordinal: i, Request: enrichment.TranslationRequest{
				Language: "it", TargetLanguage: "en", CanonicalLemma: "gamba",
				ExampleSentence:  fmt.Sprintf("Frase numero %d in gamba.", i),
				CandidateSenses:  []enrichment.LexicalSense{{EvidenceID: "wikt:in gamba:1", Gloss: "capable"}},
				RequireIdiomCard: true, IdiomHeadword: headword, IdiomMatchedForms: []string{"in", "gamba"},
			}},
		})
	}
	return items
}

func TestIdiomCaseSetKeepsEveryV1CaseUnchanged(t *testing.T) {
	v1Bytes, err := os.ReadFile(filepath.Join("testdata", "idiom_review_cases_v1.json"))
	require.NoError(t, err)
	// v1 predates the ambiguous-control class, so it is decoded without the
	// current coverage validation.
	var v1 idiomReviewCaseSet
	require.NoError(t, json.Unmarshal(v1Bytes, &v1))
	require.Equal(t, "idiom-review-v1", v1.Version)
	require.Len(t, v1.Cases, 8)
	current, err := loadIdiomCaseSet(bytes.NewReader(idiomReviewCases))
	require.NoError(t, err)

	byID := map[string]idiomReviewCase{}
	for _, reviewCase := range current.Cases {
		byID[reviewCase.ID] = reviewCase
	}
	for _, v1Case := range v1.Cases {
		t.Run(v1Case.ID, func(t *testing.T) {
			currentCase, ok := byID[v1Case.ID]
			require.True(t, ok, "the current set dropped the v1 case")
			require.Equal(t, v1Case, currentCase)
		})
	}
}

func TestIdiomCaseValidationRejectsAnAmbiguousControlWithAnExpectedGloss(t *testing.T) {
	ambiguous := idiomReviewCase{
		ID: "it-rispetto-a", Language: "it", Kind: idiomCaseAmbiguous, Headword: "rispetto a",
		MatchedForms: []string{"rispetto", "a"}, Sentence: "È più alto rispetto a me.",
		Evidence: []enrichment.LexicalSense{{EvidenceID: "e1", Gloss: "with respect to"}},
	}
	require.NoError(t, ambiguous.validate())
	ambiguous.ExpectedGloss = "compared with"
	require.ErrorContains(t, ambiguous.validate(), "ambiguous control has no expected Gloss")
	require.Equal(t, "ambiguous control", idiomCaseAmbiguous.label())
}

// The embedded set's v2 cases come from the failed 2026-11-08 Italian review
// and its v3 cases from the failed v2 re-run. A provider that confirms every
// idiom must fail the rispetto a, fare bene and casa isolata controls and the
// in giro exclusion; one that answers as v3 asks passes them.
func TestReviewIdiomScoresTheFailedReviewCards(t *testing.T) {
	cases, err := loadIdiomCaseSet(bytes.NewReader(idiomReviewCases))
	require.NoError(t, err)

	t.Run("a provider that confirms every idiom fails the controls", func(t *testing.T) {
		confirmAll := stubIdiomClient(t, func(payload idiomProviderRequestPayload) idiomStubAnswer {
			if payload.Headword == "in giro" {
				return idiomStubAnswer{translation: "to go around", gloss: "to go around"}
			}
			return idiomStubAnswer{translation: "with respect to", gloss: "with respect to"}
		})
		report := reviewIdiom(context.Background(), idiomReviewOptions{Now: time.Now(), Cases: cases, Client: confirmAll, Provider: idiomTestProviderMetadata()})
		byID := idiomResultsByCaseID(report.Cases)
		t.Run("rispetto a", func(t *testing.T) {
			require.Equal(t, checkFail, idiomCheckStatus(byID["it-rispetto-a-comparative"], checkNameIdiomOmitted))
		})
		t.Run("fare bene", func(t *testing.T) {
			require.Equal(t, checkFail, idiomCheckStatus(byID["it-fare-bene-literal"], checkNameIdiomOmitted))
		})
		t.Run("di fronte", func(t *testing.T) {
			require.Equal(t, checkPass, idiomCheckStatus(byID["it-di-fronte"], checkNameIdiomConfirmed))
		})
		t.Run("casa isolata", func(t *testing.T) {
			require.Equal(t, checkFail, idiomCheckStatus(byID["it-casa-isolata-remote"], checkNameIdiomOmitted))
		})
		t.Run("in giro", func(t *testing.T) {
			require.Equal(t, checkFail, idiomCheckStatus(byID["it-in-giro"], checkNameIdiomGlossExcludes))
		})
	})

	t.Run("a provider that answers as v3 asks passes every case", func(t *testing.T) {
		answerAsV3 := stubIdiomClient(t, func(payload idiomProviderRequestPayload) idiomStubAnswer {
			switch payload.Headword {
			case "in giro":
				return idiomStubAnswer{translation: "around", gloss: "around, about"}
			case "casa isolata":
				return idiomStubAnswer{translation: "remote house", unresolvedKind: string(enrichment.ContextualGlossUnresolvedAmbiguous), reason: "No candidate sense means a remote house."}
			case "rispetto a":
				return idiomStubAnswer{translation: "compared with", unresolvedKind: string(enrichment.ContextualGlossUnresolvedAmbiguous), reason: "No candidate sense is comparative."}
			case "fare bene", "tagliare la corda", "jemanden auf den Arm nehmen":
				return idiomStubAnswer{translation: "to do properly", unresolvedKind: string(enrichment.ContextualGlossUnresolvedLiteralUse), reason: "The words are literal here."}
			default:
				return idiomStubAnswer{translation: "in front", gloss: "in front"}
			}
		})
		report := reviewIdiom(context.Background(), idiomReviewOptions{Now: time.Now(), Cases: cases, Client: answerAsV3, Provider: idiomTestProviderMetadata()})
		byID := idiomResultsByCaseID(report.Cases)
		t.Run("rispetto a is ambiguous", func(t *testing.T) {
			require.Equal(t, idiomOutcomeAmbiguous, byID["it-rispetto-a-comparative"].Outcome)
			require.Equal(t, checkPass, idiomCheckStatus(byID["it-rispetto-a-comparative"], checkNameIdiomOmitted))
		})
		t.Run("fare bene is literal use", func(t *testing.T) {
			require.Equal(t, checkPass, idiomCheckStatus(byID["it-fare-bene-literal"], checkNameIdiomOmitted))
		})
		t.Run("every automatic check passes", func(t *testing.T) {
			require.Zero(t, report.Summary.ChecksFail)
			require.Equal(t, report.Summary.Total+1, report.Summary.ChecksPass, "in giro adds its exclusion check")
		})

		var output bytes.Buffer
		require.NoError(t, writeIdiomReport(&output, report))
		for name, fragment := range map[string]string{
			"case heading": "## `it-rispetto-a-comparative` — it `rispetto a` (ambiguous control)",
			"check row":    "| card is omitted (`literal_use` or `ambiguous`) | **PASS** |",
		} {
			t.Run("report "+name, func(t *testing.T) {
				require.Contains(t, output.String(), fragment)
			})
		}
	})
}

// The idiom review runs and records the idiom-card reasoning effort, so its
// report names the setting the requests used (#2389).
func TestOpenIdiomReviewEnvironmentRecordsTheIdiomCardReasoningEffort(t *testing.T) {
	t.Setenv("MOUSEION_LLM_ENABLED", "true")
	t.Setenv("MOUSEION_LLM_API_KEY", "test-key")
	t.Setenv("MOUSEION_LLM_MODEL", "gpt-review")
	t.Setenv("MOUSEION_LLM_BASE_URL", "https://stub.example/v1")
	t.Setenv("MOUSEION_LLM_SUPPORTS_REASONING_EFFORT", "true")
	t.Setenv("MOUSEION_LLM_REASONING_EFFORT", "low")
	t.Setenv("MOUSEION_LLM_IDIOM_REASONING_EFFORT", "medium")

	idiom, err := openIdiomReviewEnvironment()
	require.NoError(t, err)
	require.Equal(t, "medium", idiom.Provider.ReasoningEffort)
	require.Equal(t, "gpt-review/idiom-card-v3-reasoning-medium", idiom.Provider.ProviderVersion)

	gloss, err := openReviewEnvironment()
	require.NoError(t, err)
	require.Equal(t, "low", gloss.Provider.ReasoningEffort, "the Gloss review keeps the global effort")
}

// A positive case may list sentence words its Gloss must leave out, so a
// sentence verb carried into the Gloss (andare in giro as "to go around")
// fails automatically (#2389).
func TestReviewIdiomFailsAGlossThatAddsAnExcludedSentenceWord(t *testing.T) {
	cases := idiomReviewCaseSet{Version: "idiom-review-test", Cases: []idiomReviewCase{{
		ID: "it-in-giro", Language: "it", Kind: idiomCasePositive, Headword: "in giro",
		MatchedForms: []string{"in", "giro"}, Sentence: "Andiamo in giro per la città.", ExpectedGloss: "around, about",
		GlossExcludes: []string{"go", "andare"},
		Evidence:      []enrichment.LexicalSense{{EvidenceID: "wikt:in giro:1", Gloss: "here and there, about, around"}},
	}}}
	for name, test := range map[string]struct {
		gloss string
		want  checkStatus
	}{
		"the sentence verb":        {gloss: "to go around", want: checkFail},
		"the sentence verb, cased": {gloss: "Go around", want: checkFail},
		"the idiom alone":          {gloss: "around, about", want: checkPass},
		"a word containing go":     {gloss: "ago, gone by", want: checkPass},
	} {
		t.Run(name, func(t *testing.T) {
			client := stubIdiomClient(t, func(idiomProviderRequestPayload) idiomStubAnswer {
				return idiomStubAnswer{translation: test.gloss, gloss: test.gloss}
			})
			report := reviewIdiom(context.Background(), idiomReviewOptions{Now: time.Now(), Cases: cases, Client: client, Provider: idiomTestProviderMetadata()})
			result := report.Cases[0]
			require.Equal(t, checkPass, idiomCheckStatus(result, checkNameIdiomConfirmed))
			require.Equal(t, test.want, idiomCheckStatus(result, checkNameIdiomGlossExcludes))
		})
	}

	t.Run("an unconfirmed case is not applicable", func(t *testing.T) {
		client := stubIdiomClient(t, func(idiomProviderRequestPayload) idiomStubAnswer {
			return idiomStubAnswer{unresolvedKind: string(enrichment.ContextualGlossUnresolvedAmbiguous), reason: "unclear"}
		})
		report := reviewIdiom(context.Background(), idiomReviewOptions{Now: time.Now(), Cases: cases, Client: client, Provider: idiomTestProviderMetadata()})
		require.Equal(t, checkNotApplicable, idiomCheckStatus(report.Cases[0], checkNameIdiomGlossExcludes))
		require.Equal(t, 1, report.Summary.ChecksFail, "only the confirmation check fails")
	})
	t.Run("the report names the check", func(t *testing.T) {
		client := stubIdiomClient(t, func(idiomProviderRequestPayload) idiomStubAnswer {
			return idiomStubAnswer{translation: "to go around", gloss: "to go around"}
		})
		report := reviewIdiom(context.Background(), idiomReviewOptions{Now: time.Now(), Cases: cases, Client: client, Provider: idiomTestProviderMetadata()})
		var output bytes.Buffer
		require.NoError(t, writeIdiomReport(&output, report))
		require.Contains(t, output.String(), "- Gloss must leave out: `go, andare`")
		require.Contains(t, output.String(), "| Gloss leaves out the excluded sentence words | **FAIL** — Gloss `to go around` contains `go` |")
	})
}

func TestIdiomCaseValidationAllowsGlossExclusionsOnPositiveCasesOnly(t *testing.T) {
	positive := idiomReviewCase{
		ID: "it-in-giro", Language: "it", Kind: idiomCasePositive, Headword: "in giro",
		MatchedForms: []string{"in", "giro"}, Sentence: "Andiamo in giro.", ExpectedGloss: "around",
		GlossExcludes: []string{"go"},
		Evidence:      []enrichment.LexicalSense{{EvidenceID: "e1", Gloss: "around"}},
	}
	require.NoError(t, positive.validate())

	blank := positive
	blank.GlossExcludes = []string{" "}
	require.ErrorContains(t, blank.validate(), "Gloss exclusions cannot be blank")

	control := positive
	control.Kind, control.ExpectedGloss = idiomCaseAmbiguous, ""
	require.ErrorContains(t, control.validate(), "only a positive case lists Gloss exclusions")
}

func TestIdiomCaseSetKeepsEveryV2CaseUnchanged(t *testing.T) {
	v2Bytes, err := os.ReadFile(filepath.Join("testdata", "idiom_review_cases_v2.json"))
	require.NoError(t, err)
	v2, err := loadIdiomCaseSet(bytes.NewReader(v2Bytes))
	require.NoError(t, err)
	require.Equal(t, "idiom-review-v2", v2.Version)
	require.Len(t, v2.Cases, 11)
	current, err := loadIdiomCaseSet(bytes.NewReader(idiomReviewCases))
	require.NoError(t, err)

	byID := map[string]idiomReviewCase{}
	for _, reviewCase := range current.Cases {
		byID[reviewCase.ID] = reviewCase
	}
	for _, v2Case := range v2.Cases {
		t.Run(v2Case.ID, func(t *testing.T) {
			currentCase, ok := byID[v2Case.ID]
			require.True(t, ok, "the current set dropped the v2 case")
			require.Equal(t, v2Case, currentCase)
		})
	}
}

// A control passes whenever its card is omitted: literal_use and ambiguous both
// keep the card out of the deck, so the label the provider picks does not
// matter to the learner (#2389).
func TestReviewIdiomControlPassesWithEitherOmissionKind(t *testing.T) {
	cases := idiomReviewCaseSet{Version: "idiom-review-test", Cases: []idiomReviewCase{
		{
			ID: "it-tagliare-la-corda", Language: "it", Kind: idiomCaseLiteralUse, Headword: "tagliare la corda",
			MatchedForms: []string{"tagliò", "la", "corda"}, Sentence: "Tagliò la corda del pacco.",
			Evidence: []enrichment.LexicalSense{{EvidenceID: "e1", Gloss: "to run away"}},
		},
		{
			ID: "it-rispetto-a", Language: "it", Kind: idiomCaseAmbiguous, Headword: "rispetto a",
			MatchedForms: []string{"rispetto", "a"}, Sentence: "È più alto rispetto a lui.",
			Evidence: []enrichment.LexicalSense{{EvidenceID: "e1", Gloss: "with respect to"}},
		},
	}}
	for name, kind := range map[string]enrichment.ContextualGlossUnresolvedKind{
		"literal_use": enrichment.ContextualGlossUnresolvedLiteralUse,
		"ambiguous":   enrichment.ContextualGlossUnresolvedAmbiguous,
	} {
		t.Run(name, func(t *testing.T) {
			client := stubIdiomClient(t, func(idiomProviderRequestPayload) idiomStubAnswer {
				return idiomStubAnswer{unresolvedKind: string(kind), reason: "no card"}
			})
			report := reviewIdiom(context.Background(), idiomReviewOptions{Now: time.Now(), Cases: cases, Client: client, Provider: idiomTestProviderMetadata()})
			byID := idiomResultsByCaseID(report.Cases)
			t.Run("literal-use control", func(t *testing.T) {
				require.Equal(t, checkPass, idiomCheckStatus(byID["it-tagliare-la-corda"], checkNameIdiomOmitted))
			})
			t.Run("ambiguous control", func(t *testing.T) {
				require.Equal(t, checkPass, idiomCheckStatus(byID["it-rispetto-a"], checkNameIdiomOmitted))
			})
		})
	}
	t.Run("an unresolved answer without a kind fails", func(t *testing.T) {
		client := stubIdiomClient(t, func(idiomProviderRequestPayload) idiomStubAnswer {
			return idiomStubAnswer{unresolvedKind: "", reason: ""}
		})
		report := reviewIdiom(context.Background(), idiomReviewOptions{Now: time.Now(), Cases: cases, Client: client, Provider: idiomTestProviderMetadata()})
		require.Equal(t, checkFail, idiomCheckStatus(idiomResultsByCaseID(report.Cases)["it-rispetto-a"], checkNameIdiomOmitted))
	})
}
