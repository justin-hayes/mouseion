package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/enrichment"
)

// idiomReviewOptions freezes everything one idiom report records besides the
// provider answers. Matcher metadata is recorded because a review approves the
// exact matching rules as well as the prompt.
type idiomReviewOptions struct {
	Now            time.Time
	Cases          idiomReviewCaseSet
	Client         translationClient
	Provider       providerMetadata
	MatcherVersion string
	GapBound       int
	Sample         idiomSampleOptions
}

// idiomSampleOptions names the Book sample and carries the idiom items an
// in-memory freeze of the Book's current deck preparation matched.
type idiomSampleOptions struct {
	Requested bool
	BookID    string
	Language  string
	Seed      string
	Items     []cardexport.IdiomWorkItem
}

type idiomReviewReport struct {
	Now            time.Time
	Provider       providerMetadata
	CaseSetVersion string
	MatcherVersion string
	GapBound       int
	Cases          []idiomCaseResult
	Summary        idiomReviewSummary
	Sample         idiomSampleReport
}

type idiomReviewSummary struct {
	Total             int
	Confirmed         int
	LiteralUse        int
	Ambiguous         int
	Unresolved        int
	InvalidResponse   int
	ProviderError     int
	LemmaCardControls int
	ChecksPass        int
	ChecksFail        int
}

type idiomCaseResult struct {
	Case     idiomReviewCase
	Request  enrichment.TranslationRequest
	Response enrichment.TranslationResponse
	Err      error
	Outcome  idiomOutcomeKind
	Checks   []automaticCheck
}

type idiomOutcomeKind string

const (
	idiomOutcomeConfirmed        idiomOutcomeKind = "confirmed"
	idiomOutcomeLiteralUse       idiomOutcomeKind = "literal_use"
	idiomOutcomeAmbiguous        idiomOutcomeKind = "ambiguous"
	idiomOutcomeUnresolved       idiomOutcomeKind = "unresolved"
	idiomOutcomeInvalid          idiomOutcomeKind = "invalid response"
	idiomOutcomeProviderError    idiomOutcomeKind = "provider error"
	idiomOutcomeLemmaCardControl idiomOutcomeKind = "lemma-card control"
)

const (
	checkNameIdiomConfirmed  = "outcome is confirmed"
	checkNameIdiomLiteralUse = "outcome is literal_use"
	checkNameIdiomAmbiguous  = "outcome is ambiguous"
	checkNameIdiomLemmaCard  = "lemma card moves off the idiom sentence"
	checkNameIdiomKnownKind  = "case kind is known"
)

// reviewIdiom sends every provider case through the real idiom codec and
// applies the automatic checks, then sends the Book sample. Lemma-card controls
// are repository data and are checked without a provider call. The command
// never decides a judgement-based verdict.
func reviewIdiom(ctx context.Context, options idiomReviewOptions) idiomReviewReport {
	report := idiomReviewReport{
		Now:            options.Now.UTC(),
		Provider:       options.Provider,
		CaseSetVersion: options.Cases.Version,
		MatcherVersion: options.MatcherVersion,
		GapBound:       options.GapBound,
		Cases:          make([]idiomCaseResult, 0, len(options.Cases.Cases)),
	}
	for _, testCase := range options.Cases.Cases {
		result := idiomCaseResult{Case: testCase}
		if testCase.Kind == idiomCaseLemmaCard {
			result.Outcome = idiomOutcomeLemmaCardControl
		} else {
			result.Request = testCase.request()
			result.Response, result.Err = options.Client.Translate(ctx, result.Request)
			result.Outcome = classifyIdiomOutcome(result)
		}
		result.Checks = []automaticCheck{idiomCaseCheck(testCase, result)}
		report.Cases = append(report.Cases, result)
	}
	report.Summary = summarizeIdiom(report.Cases)
	report.Sample = runIdiomSample(ctx, options.Client, options.Sample)
	return report
}

// classifyIdiomOutcome names the visible result class of one idiom provider
// call.
func classifyIdiomOutcome(result idiomCaseResult) idiomOutcomeKind {
	if result.Err != nil {
		if errors.Is(result.Err, enrichment.ErrInvalidTranslationResponse) {
			return idiomOutcomeInvalid
		}
		return idiomOutcomeProviderError
	}
	if strings.TrimSpace(result.Response.Gloss) != "" {
		return idiomOutcomeConfirmed
	}
	switch result.Response.UnresolvedKind {
	case enrichment.ContextualGlossUnresolvedLiteralUse:
		return idiomOutcomeLiteralUse
	case enrichment.ContextualGlossUnresolvedAmbiguous:
		return idiomOutcomeAmbiguous
	default:
		return idiomOutcomeUnresolved
	}
}

// idiomCaseCheck compares the expected review class with the observed outcome.
// A positive case must be confirmed; a literal-use control must be rejected as
// literal_use; an ambiguous control must be rejected as ambiguous; a
// lemma-card control must point at a different sentence that still carries the
// anchor.
func idiomCaseCheck(testCase idiomReviewCase, result idiomCaseResult) automaticCheck {
	switch testCase.Kind {
	case idiomCasePositive:
		return expectedIdiomOutcomeCheck(checkNameIdiomConfirmed, idiomOutcomeConfirmed, result)
	case idiomCaseLiteralUse:
		return expectedIdiomOutcomeCheck(checkNameIdiomLiteralUse, idiomOutcomeLiteralUse, result)
	case idiomCaseAmbiguous:
		return expectedIdiomOutcomeCheck(checkNameIdiomAmbiguous, idiomOutcomeAmbiguous, result)
	case idiomCaseLemmaCard:
		return lemmaCardControlCheck(testCase)
	default:
		return automaticCheck{Name: checkNameIdiomKnownKind, Status: checkFail, Detail: fmt.Sprintf("unknown case kind %q", testCase.Kind)}
	}
}

func expectedIdiomOutcomeCheck(name string, want idiomOutcomeKind, result idiomCaseResult) automaticCheck {
	check := automaticCheck{Name: name, Status: checkFail}
	if result.Outcome == want {
		check.Status = checkPass
		return check
	}
	check.Detail = fmt.Sprintf("expected %q, observed %q", want, result.Outcome)
	return check
}

func lemmaCardControlCheck(testCase idiomReviewCase) automaticCheck {
	check := automaticCheck{Name: checkNameIdiomLemmaCard, Status: checkFail}
	if problem := lemmaCardControlProblem(testCase); problem != "" {
		check.Detail = problem
	} else {
		check.Status = checkPass
	}
	return check
}

// idiomCheckStatus finds one named automatic check on an idiom case result.
func idiomCheckStatus(result idiomCaseResult, name string) checkStatus {
	for _, check := range result.Checks {
		if check.Name == name {
			return check.Status
		}
	}
	return checkNotApplicable
}

// summarizeIdiom counts outcomes and automatic checks across the full case set.
func summarizeIdiom(cases []idiomCaseResult) idiomReviewSummary {
	summary := idiomReviewSummary{Total: len(cases)}
	for _, result := range cases {
		switch result.Outcome {
		case idiomOutcomeConfirmed:
			summary.Confirmed++
		case idiomOutcomeLiteralUse:
			summary.LiteralUse++
		case idiomOutcomeAmbiguous:
			summary.Ambiguous++
		case idiomOutcomeUnresolved:
			summary.Unresolved++
		case idiomOutcomeInvalid:
			summary.InvalidResponse++
		case idiomOutcomeProviderError:
			summary.ProviderError++
		case idiomOutcomeLemmaCardControl:
			summary.LemmaCardControls++
		}
		for _, check := range result.Checks {
			switch check.Status {
			case checkPass:
				summary.ChecksPass++
			case checkFail:
				summary.ChecksFail++
			case checkNotApplicable:
			}
		}
	}
	return summary
}
