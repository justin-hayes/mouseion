package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/justin-hayes/mouseion/internal/enrichment"
)

// translationClient is the real provider seam. The command uses the same
// Translate call normal preparation uses; tests install the real client with a
// stubbed HTTP transport so no network is reached.
type translationClient interface {
	Translate(context.Context, enrichment.TranslationRequest) (enrichment.TranslationResponse, error)
}

// reviewOptions freezes everything one report records besides the provider
// answers, so a run is reproducible and a test can supply a fixed date.
// providerMetadata is the configured provider identity every report header
// records. Bundling it keeps environment, options and report from repeating the
// same field set.
type providerMetadata struct {
	Model           string
	ReasoningEffort string
	ProviderName    string
	ProviderVersion string
	PromptContract  string
}

type reviewOptions struct {
	Now                      time.Time
	Cases                    reviewCaseSet
	Client                   translationClient
	Provider                 providerMetadata
	MismatchOutcomeSupported bool
}

type reviewReport struct {
	Now            time.Time
	Provider       providerMetadata
	CaseSetVersion string
	Cases          []caseResult
	Summary        reviewSummary
}

type reviewSummary struct {
	Total               int
	Glossed             int
	Abstained           int
	Unresolved          int
	InvalidResponse     int
	ProviderError       int
	EqualsPass          int
	EqualsFail          int
	EqualsNotApplicable int
	ContainsGloss       int
	ContainsTotal       int
	MismatchStatus      checkStatus
	MismatchDetail      string
}

type caseResult struct {
	Case     reviewCase
	Request  enrichment.TranslationRequest
	Response enrichment.TranslationResponse
	Err      error
	Outcome  outcomeKind
	Checks   []automaticCheck
}

type outcomeKind string

const (
	outcomeGlossed       outcomeKind = "glossed"
	outcomeAbstained     outcomeKind = "abstained"
	outcomeUnresolved    outcomeKind = "unresolved"
	outcomeInvalid       outcomeKind = "invalid response"
	outcomeProviderError outcomeKind = "provider error"
)

type checkStatus string

const (
	checkPass          checkStatus = "pass"
	checkFail          checkStatus = "fail"
	checkNotApplicable checkStatus = "not applicable"
	checkNotSupported  checkStatus = "not supported"

	checkNameGlossEqualsTranslation = "Gloss equals translation"
	checkNameLemmaMismatch          = "outcome is lemma_mismatch"
)

type automaticCheck struct {
	Name   string
	Status checkStatus
	Detail string
}

// review sends every frozen case through the provider and applies the two
// automatic checks. It never decides the judgement-based rubric items.
func review(ctx context.Context, options reviewOptions) reviewReport {
	report := reviewReport{
		Now:            options.Now.UTC(),
		Provider:       options.Provider,
		CaseSetVersion: options.Cases.Version,
		Cases:          make([]caseResult, 0, len(options.Cases.Cases)),
	}
	for _, testCase := range options.Cases.Cases {
		result := caseResult{Case: testCase, Request: testCase.request()}
		result.Response, result.Err = options.Client.Translate(ctx, result.Request)
		result.Outcome = classifyOutcome(result)
		result.Checks = []automaticCheck{
			glossEqualsTranslation(result),
			lemmaMismatchCheck(testCase, result, options.MismatchOutcomeSupported),
		}
		report.Cases = append(report.Cases, result)
	}
	report.Summary = summarize(report.Cases, options.MismatchOutcomeSupported)
	return report
}

// classifyOutcome names the visible result class of one provider call. The v15
// contract has no enumerated unresolved kind, so an abstention is distinguished
// by context_only rather than by a lemma_mismatch value.
func classifyOutcome(result caseResult) outcomeKind {
	if result.Err != nil {
		if errors.Is(result.Err, enrichment.ErrInvalidTranslationResponse) {
			return outcomeInvalid
		}
		return outcomeProviderError
	}
	if strings.TrimSpace(result.Response.Gloss) != "" {
		return outcomeGlossed
	}
	if result.Response.ContextOnly {
		return outcomeAbstained
	}
	return outcomeUnresolved
}

// glossEqualsTranslation checks the explicit relationship the v16 contract
// will state: unless the sentence uses a narrower sense, the Gloss is the plain
// lemma translation. Comparison ignores surrounding whitespace and case, which
// do not change the equivalent.
func glossEqualsTranslation(result caseResult) automaticCheck {
	check := automaticCheck{Name: checkNameGlossEqualsTranslation}
	if result.Outcome != outcomeGlossed {
		check.Status = checkNotApplicable
		check.Detail = "no resolved Gloss"
		return check
	}
	check.Status = checkFail
	if strings.EqualFold(strings.TrimSpace(result.Response.Gloss), strings.TrimSpace(result.Response.Translation)) {
		check.Status = checkPass
		return check
	}
	check.Detail = fmt.Sprintf("Gloss %q differs from translation %q", result.Response.Gloss, result.Response.Translation)
	return check
}

// lemmaMismatchCheck compares the expected mismatch verdict with the observed
// outcome. The v15 contract cannot carry a lemma_mismatch outcome, so the check
// reports unsupported instead of failing every mismatch case.
func lemmaMismatchCheck(testCase reviewCase, result caseResult, supported bool) automaticCheck {
	check := automaticCheck{Name: checkNameLemmaMismatch}
	if !supported {
		check.Status = checkNotSupported
		check.Detail = "not supported by this contract"
		return check
	}
	expected := testCase.ExpectedVerdictClass == verdictLemmaMismatch
	observed := observedLemmaMismatch(result)
	if expected == observed {
		check.Status = checkPass
		return check
	}
	check.Status = checkFail
	check.Detail = fmt.Sprintf("expected lemma_mismatch=%t, observed=%t", expected, observed)
	return check
}

// observedLemmaMismatch reports the enumerated unresolved kind once the
// mismatch-outcome ticket adds it to the response contract. Flip this together
// with lemmaMismatchOutcomeSupported; until then no response can be a lemma
// mismatch.
func observedLemmaMismatch(caseResult) bool {
	return false
}

// summarize counts outcomes and both automatic checks across the full case set.
func summarize(cases []caseResult, mismatchSupported bool) reviewSummary {
	summary := reviewSummary{Total: len(cases), MismatchStatus: checkPass}
	for _, result := range cases {
		switch result.Outcome {
		case outcomeGlossed:
			summary.Glossed++
		case outcomeAbstained:
			summary.Abstained++
		case outcomeUnresolved:
			summary.Unresolved++
		case outcomeInvalid:
			summary.InvalidResponse++
		case outcomeProviderError:
			summary.ProviderError++
		}
		if result.Outcome == outcomeGlossed && strings.TrimSpace(result.Response.Translation) != "" {
			summary.ContainsTotal++
			if strings.Contains(strings.ToLower(result.Response.Gloss), strings.ToLower(result.Response.Translation)) {
				summary.ContainsGloss++
			}
		}
		switch automaticCheckStatus(result, checkNameGlossEqualsTranslation) {
		case checkPass:
			summary.EqualsPass++
		case checkFail:
			summary.EqualsFail++
		default:
			summary.EqualsNotApplicable++
		}
		if !mismatchSupported {
			summary.MismatchStatus = checkNotSupported
			summary.MismatchDetail = "not supported by this contract"
			continue
		}
		if automaticCheckStatus(result, checkNameLemmaMismatch) == checkFail {
			summary.MismatchStatus = checkFail
		}
	}
	return summary
}

// automaticCheckStatus finds one named automatic check on a case result.
func automaticCheckStatus(result caseResult, name string) checkStatus {
	for _, check := range result.Checks {
		if check.Name == name {
			return check.Status
		}
	}
	return checkNotApplicable
}
