package main

import (
	"io"
	"strings"

	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

// writeIdiomReport renders one idiom review as the dated Markdown report the
// operator keeps under doc/reviews/. Every judgement-based verdict stays blank.
func writeIdiomReport(output io.Writer, report idiomReviewReport) error {
	var builder strings.Builder
	writef(&builder, "# Idiom card review — %s\n\n", codeSpan(report.Provider.PromptContract))
	builder.WriteString("Status: **Operator review required**\n\n")
	writef(&builder, "- Date: %s\n", report.Now.Format("2006-01-02"))
	writef(&builder, "- Model: %s\n", codeSpan(report.Provider.Model))
	writef(&builder, "- Reasoning setting: %s\n", codeSpan(report.Provider.ReasoningEffort))
	writef(&builder, "- Prompt contract: %s\n", codeSpan(report.Provider.PromptContract))
	writef(&builder, "- Provider: %s (%s)\n", codeSpan(report.Provider.ProviderName), codeSpan(report.Provider.ProviderVersion))
	writef(&builder, "- Case set: %s (%d cases)\n", codeSpan(report.CaseSetVersion), len(report.Cases))
	writef(&builder, "- Matcher version: %s\n", codeSpan(report.MatcherVersion))
	writef(&builder, "- Matcher gap bound: %d\n", report.GapBound)
	writeIdiomSampleHeader(&builder, report.Sample)
	builder.WriteString("- Rubric: [doc/reviews/README.md](README.md)\n\n")
	writeIdiomSummary(&builder, report)
	writeIdiomGate(&builder)
	for _, result := range report.Cases {
		writeIdiomCaseReport(&builder, result)
	}
	writeIdiomSampleCards(&builder, report.Sample)
	_, err := io.WriteString(output, builder.String())
	return err
}

func writeIdiomSampleHeader(builder *strings.Builder, sample idiomSampleReport) {
	if !sample.Requested {
		builder.WriteString("- Sample: not requested; the rollout gate cannot pass without one\n")
		return
	}
	writef(builder, "- Sample: %d confirmed idiom cards requested from Book %s in %s (seed %s); selected %d\n",
		sample.Limit, codeSpan(sample.BookID), codeSpan(sample.Language), codeSpan(sample.Seed), len(sample.Cards))
}

func writeIdiomSummary(builder *strings.Builder, report idiomReviewReport) {
	summary := report.Summary
	builder.WriteString("## Summary\n\n")
	writef(builder, "- Cases run: %d\n", summary.Total)
	writef(builder, "- Outcomes: confirmed=%d, literal_use=%d, ambiguous=%d, unresolved=%d, invalid response=%d, provider error=%d, lemma-card controls=%d\n",
		summary.Confirmed, summary.LiteralUse, summary.Ambiguous, summary.Unresolved, summary.InvalidResponse, summary.ProviderError, summary.LemmaCardControls)
	writef(builder, "- Automatic checks: %d passed, %d failed\n\n", summary.ChecksPass, summary.ChecksFail)
}

func writeIdiomGate(builder *strings.Builder) {
	builder.WriteString("## Gate\n\n")
	builder.WriteString("The gate passes only when every item below is true:\n\n")
	builder.WriteString("- every positive case is confirmed with a citation-form Gloss;\n")
	builder.WriteString("- no literal-use control is confirmed and every lemma-card control passes its automatic check;\n")
	builder.WriteString("- none of the sampled cards teaches a wrong meaning.\n\n")
	builder.WriteString("- [ ] Gate evaluated and recorded: __________\n\n")
}

func writeIdiomCaseReport(builder *strings.Builder, result idiomCaseResult) {
	testCase := result.Case
	writef(builder, "## %s — %s %s (%s)\n\n", codeSpan(testCase.ID), testCase.Language, codeSpan(testCase.Headword), testCase.Kind.label())
	if len(testCase.MatchedForms) > 0 {
		writef(builder, "- Matched words: %s\n", codeSpan(strings.Join(testCase.MatchedForms, ", ")))
	}
	if strings.TrimSpace(testCase.Sentence) != "" {
		writef(builder, "- Representative sentence: %s\n", codeSpan(testCase.Sentence))
	}
	if strings.TrimSpace(testCase.ExpectedGloss) != "" {
		writef(builder, "- Expected Gloss: %s\n", codeSpan(testCase.ExpectedGloss))
	}
	if strings.TrimSpace(testCase.Notes) != "" {
		writef(builder, "- Notes: %s\n", testCase.Notes)
	}
	if testCase.Kind == idiomCaseLemmaCard {
		writeIdiomLemmaControl(builder, testCase, result)
		return
	}
	writeIdiomRequest(builder, testCase, result)
	writeIdiomProviderResult(builder, result)
	writeIdiomChecks(builder, result)
	builder.WriteString("\n### Reviewer verdicts\n\n")
	builder.WriteString("Leave these blank for the operator; the command cannot judge them.\n\n")
	builder.WriteString("- [ ] Citation form — verdict: __________\n")
	builder.WriteString("- [ ] No definition paraphrase — verdict: __________\n")
	builder.WriteString("- [ ] No sentence inflection or arguments — verdict: __________\n")
	builder.WriteString("- [ ] Meaning correct in context — verdict: __________\n\n")
}

func writeIdiomLemmaControl(builder *strings.Builder, testCase idiomReviewCase, result idiomCaseResult) {
	writef(builder, "- Anchor: %s (%s)\n", codeSpan(testCase.AnchorLemma), codeSpan(testCase.AnchorUPOS))
	writef(builder, "- Idiom sentence: %s\n", codeSpan(testCase.IdiomSentence))
	writef(builder, "- Expected lemma-card sentence: %s\n", codeSpan(testCase.LemmaCardSentence))
	writeIdiomChecks(builder, result)
	builder.WriteString("\n### Reviewer verdicts\n\n")
	builder.WriteString("- [ ] Anchor card moved to an idiom-free sentence — verdict: __________\n\n")
}

func writeIdiomRequest(builder *strings.Builder, testCase idiomReviewCase, result idiomCaseResult) {
	builder.WriteString("\n### Request summary\n\n")
	builder.WriteString("| Field | Value |\n| --- | --- |\n")
	writef(builder, "| item_id | %s |\n", codeSpan(enrichment.IdiomCardItemHandle))
	writef(builder, "| language | %s |\n", codeSpan(result.Request.Language))
	writef(builder, "| target_language | %s |\n", codeSpan(result.Request.TargetLanguage))
	writef(builder, "| headword | %s |\n", codeSpan(result.Request.IdiomHeadword))
	writef(builder, "| matched_forms | %s |\n", codeSpan(strings.Join(result.Request.IdiomMatchedForms, ", ")))
	writef(builder, "| example_sentence | %s |\n", codeSpan(result.Request.ExampleSentence))
	writef(builder, "| candidate_senses | %s |\n", codeSpan(evidenceSummary(testCase.Evidence)))
}

func writeIdiomProviderResult(builder *strings.Builder, result idiomCaseResult) {
	builder.WriteString("\n### Provider result\n\n")
	writef(builder, "- Outcome: %s\n", codeSpan(string(result.Outcome)))
	if result.Err != nil {
		writef(builder, "- Error: %s\n", codeSpan(result.Err.Error()))
	}
	writef(builder, "- Translation: %s\n", codeSpan(result.Response.Translation))
	writef(builder, "- Raw Gloss: %s\n", codeSpan(result.Response.Gloss))
	writef(builder, "- Evidence IDs: %s\n", codeSpan(strings.Join(result.Response.EvidenceIDs, ", ")))
	writef(builder, "- Context only: %t\n", result.Response.ContextOnly)
	writef(builder, "- Unresolved kind: %s\n", codeSpan(string(result.Response.UnresolvedKind)))
	writef(builder, "- Unresolved reason: %s\n", codeSpan(result.Response.UnresolvedReason))
}

func writeIdiomChecks(builder *strings.Builder, result idiomCaseResult) {
	builder.WriteString("\n### Automatic checks\n\n")
	builder.WriteString("| Check | Result |\n| --- | --- |\n")
	for _, check := range result.Checks {
		writef(builder, "| %s | %s |\n", checkDisplayName(check.Name), checkResult(check))
	}
}

// sampleCardCheck records that a sampled card carries the resolved provider
// result the sample query selected.
func sampleCardCheck(card persistence.ConfirmedIdiomCard) automaticCheck {
	check := automaticCheck{Name: checkNameIdiomConfirmed, Status: checkFail}
	if strings.TrimSpace(card.Gloss) != "" && strings.TrimSpace(card.Translation) != "" {
		check.Status = checkPass
		return check
	}
	check.Detail = "the sampled card has no resolved Gloss and translation"
	return check
}

func writeIdiomSampleCards(builder *strings.Builder, sample idiomSampleReport) {
	builder.WriteString("## Sample\n\n")
	if !sample.Requested {
		builder.WriteString("No sample was requested. Run with `-book <id>` before evaluating the gate.\n\n")
		return
	}
	if len(sample.Cards) == 0 {
		builder.WriteString("No confirmed idiom cards were found for this Book and language. The gate cannot pass.\n\n")
		return
	}
	if len(sample.Cards) < sample.Limit {
		writef(builder, "Only %d confirmed idiom cards were found; the gate requires %d.\n\n", len(sample.Cards), sample.Limit)
	}
	for i, card := range sample.Cards {
		writef(builder, "### %d. %s — %s (%s)\n\n", i+1, codeSpan(card.Headword), codeSpan(card.AnchorLemma), codeSpan(card.AnchorUPOS))
		writef(builder, "- Matched words: %s\n", codeSpan(strings.Join(card.MatchedWords, ", ")))
		writef(builder, "- Representative sentence: %s\n", codeSpan(card.Sentence))
		writef(builder, "- Gloss: %s\n", codeSpan(card.Gloss))
		writef(builder, "- Translation: %s\n", codeSpan(card.Translation))
		builder.WriteString("\n### Automatic checks\n\n")
		builder.WriteString("| Check | Result |\n| --- | --- |\n")
		writef(builder, "| %s | %s |\n", checkDisplayName(checkNameIdiomConfirmed), checkResult(sampleCardCheck(card)))
		builder.WriteString("\n### Reviewer verdicts\n\n")
		builder.WriteString("- [ ] Meaning correct in context — verdict: __________\n\n")
	}
}
