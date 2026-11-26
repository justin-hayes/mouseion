package main

import (
	"fmt"
	"sort"
	"strings"
)

// reportMeta is what the report pins besides the per-word rows.
type reportMeta struct {
	Owner, Language, Date                string
	Model, Reasoning, ProviderVersion    string
	DictionaryPin                        string
	Calls                                int64
	Analyzers, TranslationPins, BookList []string
}

// cell makes text safe inside one Markdown table cell.
func cell(text string) string {
	text = strings.NewReplacer("|", "\\|", "\n", " ", "\r", " ").Replace(strings.TrimSpace(text))
	if text == "" {
		return "–"
	}
	return text
}

// highlight wraps the first occurrence of each excerpt in bold.
func highlight(text string, excerpts []string) string {
	for _, excerpt := range excerpts {
		if excerpt == "" {
			continue
		}
		if i := strings.Index(text, excerpt); i >= 0 {
			text = text[:i] + "**" + excerpt + "**" + text[i+len(excerpt):]
		}
	}
	return text
}

func renderRow(n int, r wordResult) string {
	sentence, gloss, translation, cited, passage := "–", "–", "–", "–", "–"
	if r.Candidate.ExampleSentence != "" {
		sentence = highlight(r.Candidate.ExampleSentence, []string{r.Candidate.TargetWord})
	}
	if r.Ready != nil {
		gloss = r.Ready.Gloss
		translation = highlight(r.Ready.SentenceTranslation, r.Ready.SentenceTranslationTargets)
		var lines []string
		for i, id := range r.Ready.Cited.SenseIDs {
			definition := ""
			for _, sense := range r.Candidates {
				if sense.EvidenceID == id {
					definition = sense.Gloss
					break
				}
			}
			lines = append(lines, fmt.Sprintf("`%s` (entry `%s`): %s", id, r.Ready.Cited.EntryIDs[i], definition))
		}
		cited = strings.Join(lines, "<br>")
		passage = r.Ready.Passage.Citation
		if passage == "" {
			passage = "none"
		}
	} else if m := r.Meaning; m.Gloss != "" || m.SentenceTranslation != "" {
		// An omitted answer's text is shown for the reader's context only.
		gloss = "(omitted) " + m.Gloss
		translation = highlight(m.SentenceTranslation, m.SentenceTranslationTargets)
	}
	outcome := r.Outcome
	if r.SubReason != "" {
		outcome += ": " + r.SubReason
	}
	if r.Failure != "" {
		outcome += ": " + r.Failure
	}
	if len(r.Invented) > 0 {
		outcome += " **[invented: " + strings.Join(r.Invented, ", ") + "]**"
	}
	return fmt.Sprintf("| %d | %s (%s) | %s | %s | %s | %s | %s | %s | |\n",
		n, cell(r.Identity.CanonicalLemma), cell(r.Identity.UPOS), cell(sentence), cell(gloss), cell(cited),
		cell(translation), cell(passage), cell(outcome))
}

func renderReport(meta reportMeta, results []wordResult) string {
	var b strings.Builder
	counts := map[string]int{}
	var invented []string
	for _, r := range results {
		key := r.Outcome
		if r.Outcome == outcomeMeaning {
			key += " (" + strings.SplitN(r.SubReason, ":", 2)[0] + ")"
		}
		counts[key]++
		if len(r.Invented) > 0 {
			invented = append(invented, fmt.Sprintf("%s: %s", r.Identity.CanonicalLemma, strings.Join(r.Invented, ", ")))
		}
	}
	keys := make([]string, 0, len(counts))
	for key := range counts {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	pins := func(values []string) string {
		if len(values) == 0 {
			return "none"
		}
		return strings.Join(values, "; ")
	}

	fmt.Fprintf(&b, "# Greek Study evidence run, %s\n\n", meta.Date)
	b.WriteString("This is the evidence run for turning Greek's Study switch on ([#2767](https://github.com/justin-hayes/mouseion/issues/2767), spec [#2759](https://github.com/justin-hayes/mouseion/issues/2759), [ADR 0138](../adr/0138-study-for-source-evidence-languages.md) §6). It runs the production Study material path once per word and records what Study would have shown. Nothing here was written to a database. Justin judges each row; the last column is his.\n\n")
	b.WriteString("## Threshold\n\nStated before the run (ADR 0138 §6): **at least 90% of Glosses acceptable, and no answer cites an invented sense ID.** Justin's judgement column decides the first half. The second half is counted automatically below.\n\n")
	b.WriteString("## Inputs and pins\n\n")
	fmt.Fprintf(&b, "- **Words:** the first %d in Study's new-word order for Justin's Greek (`%s`) account: Current reading first, then Personal occurrences descending, then lemma. Words already in study or held by an open Wrong-word report are skipped, as Study skips them.\n", len(results), meta.Language)
	fmt.Fprintf(&b, "- **Books drawn on:** %s.\n", pins(meta.BookList))
	fmt.Fprintf(&b, "- **Model:** `%s`, reasoning effort `%s`.\n", meta.Model, meta.Reasoning)
	fmt.Fprintf(&b, "- **Prompt contract (provider version):** `%s`, the Greek evidence-assisted pass, unchanged from its recorded assessment.\n", meta.ProviderVersion)
	fmt.Fprintf(&b, "- **Dictionary pin:** LSJ index `%s`.\n", meta.DictionaryPin)
	fmt.Fprintf(&b, "- **Translation pins:** %s.\n", pins(meta.TranslationPins))
	fmt.Fprintf(&b, "- **Treebank:** OGA `oga_src` v0.2.0 annotations (Zenodo `10.5281/zenodo.15199546`); analysis run(s): %s.\n", pins(meta.Analyzers))
	fmt.Fprintf(&b, "- **Gate:** `ProviderAssessments().MaySend` accepted the compiled codec for `%s` before anything was sent.\n", meta.Language)
	fmt.Fprintf(&b, "- **Provider calls made:** %d HTTP requests (one per word that had a dictionary entry with citable senses, plus any retry of a failed request; the other words cost no call).\n", meta.Calls)
	b.WriteString("- **Tool:** `cmd/studyevidencerun` on the branch `prototype/2767-greek-evidence-run`, never merged. It runs `study.MaterialWorker` unchanged against a restored copy of the database, with a store that records nothing.\n\n")

	b.WriteString("## Automatic summary\n\n| Outcome | Words |\n|---|---|\n")
	for _, key := range keys {
		fmt.Fprintf(&b, "| %s | %d |\n", key, counts[key])
	}
	fmt.Fprintf(&b, "| **Total** | %d |\n\n", len(results))
	fmt.Fprintf(&b, "**Answers that cited a handle outside their candidate set (invented IDs): %d.**", len(invented))
	if len(invented) > 0 {
		b.WriteString("\n\n")
		for _, line := range invented {
			fmt.Fprintf(&b, "- %s\n", line)
		}
	}
	b.WriteString("\n\n## Words\n\nA cited sense shows its native LSJ sense ID, its entry ID and the definition the provider saw. The translation's bold text is the English target highlight; the sentence's bold text is the target word. An omitted answer's Gloss and translation are shown only for context.\n\n")
	b.WriteString("| # | Lemma | Sentence | Gloss | Cited senses | Sentence translation | Passage citation | Outcome | Justin's judgement (acceptable / not, note) |\n|---|---|---|---|---|---|---|---|---|\n")
	for i, r := range results {
		b.WriteString(renderRow(i+1, r))
	}
	b.WriteString("\n## Judgement tally\n\nJustin fills this in after judging: acceptable Glosses ___ of ___ ready words; threshold (90%) met: yes / no.\n")
	return b.String()
}
