package main

import (
	"fmt"
	"io"
	"strings"

	"github.com/justin-hayes/mouseion/internal/enrichment"
)

// writeReport renders one run as the dated Markdown review the operator keeps
// under doc/reviews/. Every judgement-based rubric item stays blank.
func writeReport(output io.Writer, report reviewReport) error {
	var builder strings.Builder
	writef(&builder, "# Contextual Gloss review — %s\n\n", codeSpan(report.Provider.PromptContract))
	builder.WriteString("Status: **Operator review required**\n\n")
	writef(&builder, "- Date: %s\n", report.Now.Format("2006-01-02"))
	writef(&builder, "- Model: %s\n", codeSpan(report.Provider.Model))
	writef(&builder, "- Reasoning setting: %s\n", codeSpan(report.Provider.ReasoningEffort))
	writef(&builder, "- Prompt contract: %s\n", codeSpan(report.Provider.PromptContract))
	writef(&builder, "- Provider: %s (%s)\n", codeSpan(report.Provider.ProviderName), codeSpan(report.Provider.ProviderVersion))
	writef(&builder, "- Case set: %s (%d cases)\n", codeSpan(report.CaseSetVersion), len(report.Cases))
	builder.WriteString("- Rubric: [doc/reviews/README.md](README.md)\n\n")
	writeReportSummary(&builder, report)
	for _, result := range report.Cases {
		writeCaseReport(&builder, result)
	}
	_, err := io.WriteString(output, builder.String())
	return err
}

func writeReportSummary(builder *strings.Builder, report reviewReport) {
	summary := report.Summary
	builder.WriteString("## Summary\n\n")
	writef(builder, "- Cases run: %d\n", summary.Total)
	writef(builder, "- Outcomes: glossed=%d, abstained=%d, unresolved=%d, invalid response=%d, provider error=%d\n", summary.Glossed, summary.Abstained, summary.Unresolved, summary.InvalidResponse, summary.ProviderError)
	writef(builder, "- Automatic check %s: %d passed, %d failed, %d not applicable\n", checkDisplayName(checkNameGlossEqualsTranslation), summary.EqualsPass, summary.EqualsFail, summary.EqualsNotApplicable)
	writef(builder, "- Automatic check %s: %s\n", checkDisplayName(checkNameLemmaMismatch), checkResult(automaticCheck{Status: summary.MismatchStatus, Detail: summary.MismatchDetail}))
	if summary.ContainsTotal == 0 {
		builder.WriteString("- Full review-corpus \"Gloss contains translation\": no resolved Gloss and translation pairs\n\n")
		return
	}
	writef(builder, "- Full review-corpus \"Gloss contains translation\" rate: %d/%d (%.1f%%), for information only and not a rollout gate\n\n",
		summary.ContainsGloss, summary.ContainsTotal, 100*float64(summary.ContainsGloss)/float64(summary.ContainsTotal))
}

func writeCaseReport(builder *strings.Builder, result caseResult) {
	testCase := result.Case
	writef(builder, "## %s — %s %s (%s)\n\n", codeSpan(testCase.ID), testCase.Language, codeSpan(testCase.Lemma), codeSpan(testCase.UPOS))
	writef(builder, "- Expected verdict class: %s\n", testCase.ExpectedVerdictClass.label())
	if strings.TrimSpace(testCase.ExpectedGloss) != "" {
		writef(builder, "- Expected Gloss: %s\n", codeSpan(testCase.ExpectedGloss))
	}
	writef(builder, "- Representative sentence: %s\n", codeSpan(testCase.Sentence))
	if strings.TrimSpace(testCase.Notes) != "" {
		writef(builder, "- Notes: %s\n", testCase.Notes)
	}
	builder.WriteString("\n### Request summary\n\n")
	builder.WriteString("| Field | Value |\n| --- | --- |\n")
	writef(builder, "| item_id | %s |\n", codeSpan(enrichment.ContextualGlossItemHandle))
	writef(builder, "| language | %s |\n", codeSpan(result.Request.Language))
	writef(builder, "| target_language | %s |\n", codeSpan(result.Request.TargetLanguage))
	writef(builder, "| canonical_lemma | %s |\n", codeSpan(result.Request.CanonicalLemma))
	writef(builder, "| upos | %s |\n", codeSpan(result.Request.UPOS))
	writef(builder, "| target_word | %s |\n", codeSpan(result.Request.TargetWord))
	writef(builder, "| example_sentence | %s |\n", codeSpan(result.Request.ExampleSentence))
	writef(builder, "| candidate_senses | %s |\n", codeSpan(evidenceSummary(testCase.Evidence)))
	builder.WriteString("\n### Provider result\n\n")
	writef(builder, "- Outcome: %s\n", codeSpan(string(result.Outcome)))
	if result.Err != nil {
		writef(builder, "- Error: %s\n", codeSpan(result.Err.Error()))
	}
	writef(builder, "- Translation: %s\n", codeSpan(result.Response.Translation))
	writef(builder, "- Raw Gloss: %s\n", codeSpan(result.Response.Gloss))
	writef(builder, "- Evidence IDs: %s\n", codeSpan(strings.Join(result.Response.EvidenceIDs, ", ")))
	writef(builder, "- Context only: %t\n", result.Response.ContextOnly)
	writef(builder, "- Unresolved reason: %s\n", codeSpan(result.Response.UnresolvedReason))
	builder.WriteString("\n### Automatic checks\n\n")
	builder.WriteString("| Check | Result |\n| --- | --- |\n")
	for _, check := range result.Checks {
		writef(builder, "| %s | %s |\n", checkDisplayName(check.Name), checkResult(check))
	}
	builder.WriteString("\n### Reviewer verdicts\n\n")
	builder.WriteString("Leave these blank for the operator; the command cannot judge them.\n\n")
	builder.WriteString("- [ ] Citation form — verdict: __________\n")
	builder.WriteString("- [ ] No definition paraphrase — verdict: __________\n")
	builder.WriteString("- [ ] No sentence inflection or arguments — verdict: __________\n")
	builder.WriteString("- [ ] Protected win kept — verdict: __________\n\n")
}

// evidenceSummary names the short handles the request actually transmits while
// showing the frozen evidence text the reviewer needs.
func evidenceSummary(evidence []enrichment.LexicalSense) string {
	parts := make([]string, 0, len(evidence))
	for i, sense := range evidence {
		parts = append(parts, fmt.Sprintf("e%d %s: %s", i+1, sense.EvidenceID, sense.Gloss))
	}
	return strings.Join(parts, "; ")
}

// checkDisplayName renders the automatic-check name for the report without
// putting Markdown into the check's domain identity.
func checkDisplayName(name string) string {
	if name == checkNameLemmaMismatch {
		return "outcome is `lemma_mismatch`"
	}
	return name
}

func checkResult(check automaticCheck) string {
	label := string(check.Status)
	switch check.Status {
	case checkPass:
		label = "PASS"
	case checkFail:
		label = "FAIL"
	case checkNotSupported:
		label = "not supported by this contract"
	case checkNotApplicable:
		label = "not applicable"
	}
	if strings.TrimSpace(check.Detail) == "" || check.Status == checkNotSupported {
		return "**" + label + "**"
	}
	return "**" + label + "** — " + check.Detail
}

// codeSpan renders one provider or case value inline without letting Markdown
// metacharacters or a backtick break the report.
func codeSpan(value string) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), "\n", " ")
	if value == "" {
		return "—"
	}
	fence := "`"
	for strings.Contains(value, fence) {
		fence += "`"
	}
	padding := ""
	if strings.HasPrefix(value, "`") || strings.HasSuffix(value, "`") {
		padding = " "
	}
	return fence + padding + value + padding + fence
}

func writef(builder *strings.Builder, format string, args ...any) {
	_, _ = fmt.Fprintf(builder, format, args...)
}
