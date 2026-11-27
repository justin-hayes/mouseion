package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/lexical"
)

// notCitationForm is the automatic citation-form check on a ready Gloss. It is
// a heuristic that flags rows for the learner's eye: a verb Gloss that does not
// start "to ", or a non-verb Gloss that starts "to ", opens with an article,
// carries more than two words in one synonym, or ends like an inflected form.
func notCitationForm(upos, gloss string) string {
	gloss = strings.TrimSpace(gloss)
	lower := strings.ToLower(gloss)
	if gloss == "" {
		return ""
	}
	if upos == "VERB" {
		if !strings.HasPrefix(lower, "to ") {
			return "verb Gloss does not start \"to \""
		}
		return ""
	}
	if strings.HasPrefix(lower, "to ") {
		return "non-verb Gloss starts \"to \""
	}
	for _, article := range []string{"the ", "a ", "an "} {
		if strings.HasPrefix(lower, article) {
			return "Gloss opens with an article"
		}
	}
	for _, part := range strings.Split(lower, ",") {
		words := strings.Fields(part)
		if len(words) > 2 {
			return "Gloss carries extra words"
		}
		if upos == "ADV" || len(words) == 0 {
			continue
		}
		last := words[len(words)-1]
		switch {
		case strings.HasSuffix(last, "ing") && len(last) > 5, strings.HasSuffix(last, "ed") && len(last) > 4,
			strings.HasSuffix(last, "ies"), strings.HasSuffix(last, "s") && !strings.HasSuffix(last, "ss") &&
				!strings.HasSuffix(last, "us") && !strings.HasSuffix(last, "is") && len(last) > 3:
			return "Gloss looks inflected"
		}
	}
	return ""
}

// wideHighlights lists translation highlight excerpts longer than three words.
func wideHighlights(targets []string) []string {
	var wide []string
	for _, target := range targets {
		if len(strings.Fields(target)) > 3 {
			wide = append(wide, target)
		}
	}
	return wide
}

// reportMeta is what the report pins besides the per-word rows.
type reportMeta struct {
	Owner, Language, Date                string
	Model, Reasoning, ProviderVersion    string
	DictionaryPin                        string
	Calls                                int64
	MaxRequest                           int64
	PayloadKind                          string
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
			entryID := r.Ready.Cited.EntryIDs[i]
			definition := ""
			for _, sense := range r.Candidates {
				if sense.EvidenceID == enrichment.EntryEvidenceID(entryID, id) {
					definition = sense.Gloss
					break
				}
			}
			label := ""
			if n := entryCount(r.Candidates); n > 1 {
				label = fmt.Sprintf(" [entry %d of %d]", entryPosition(r.Candidates, entryID), n)
			}
			lines = append(lines, fmt.Sprintf("`%s` (entry `%s`%s): %s", id, entryID, label, definition))
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
	if r.Ready != nil {
		if why := notCitationForm(r.Identity.UPOS, r.Ready.Gloss); why != "" {
			outcome += " **[not citation form: " + why + "]**"
		}
		if wide := wideHighlights(r.Ready.SentenceTranslationTargets); len(wide) > 0 {
			outcome += " **[wide highlight: " + strings.Join(wide, "; ") + "]**"
		}
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
	var notForm, wide, functionWords []string
	for _, r := range results {
		if lexical.IsVocabularyStopword(meta.Language, r.Identity.CanonicalLemma, r.Identity.UPOS) {
			functionWords = append(functionWords, r.Identity.CanonicalLemma)
		}
		if r.Ready == nil {
			continue
		}
		if why := notCitationForm(r.Identity.UPOS, r.Ready.Gloss); why != "" {
			notForm = append(notForm, fmt.Sprintf("%s (%s): \"%s\", %s", r.Identity.CanonicalLemma, r.Identity.UPOS, r.Ready.Gloss, why))
		}
		if w := wideHighlights(r.Ready.SentenceTranslationTargets); len(w) > 0 {
			wide = append(wide, fmt.Sprintf("%s: \"%s\"", r.Identity.CanonicalLemma, strings.Join(w, "; ")))
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
	fmt.Fprintf(&b, "- **Prompt contract (provider version):** `%s`, payload kind `%s`: Study's Greek stacked-homograph pass, which looks up every competing LSJ entry (ADR 0139) and carries each sense's entry handle.\n", meta.ProviderVersion, meta.PayloadKind)
	fmt.Fprintf(&b, "- **Dictionary pin:** LSJ index `%s`.\n", meta.DictionaryPin)
	fmt.Fprintf(&b, "- **Translation pins:** %s.\n", pins(meta.TranslationPins))
	fmt.Fprintf(&b, "- **Treebank:** OGA `oga_src` v0.2.0 annotations (Zenodo `10.5281/zenodo.15199546`); analysis run(s): %s.\n", pins(meta.Analyzers))
	fmt.Fprintf(&b, "- **Gate:** `ProviderAssessments().MaySend` accepted the compiled codec for `%s` before anything was sent.\n", meta.Language)
	fmt.Fprintf(&b, "- **Provider calls made:** %d HTTP requests; the largest request body was %d bytes (one call per word that had a dictionary entry with citable senses, plus any retry of a failed request; the other words cost no call).\n", meta.Calls, meta.MaxRequest)
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
	b.WriteString("\n\n")
	flagList := func(title string, items []string) {
		fmt.Fprintf(&b, "**%s: %d.**", title, len(items))
		if len(items) > 0 {
			b.WriteString("\n\n")
			for _, line := range items {
				fmt.Fprintf(&b, "- %s\n", line)
			}
		}
		b.WriteString("\n")
	}
	flagList("Glosses not in citation form (automatic heuristic: a verb Gloss not starting \"to \", or a noun or adjective Gloss that starts \"to \", opens with an article, carries more than two words in a synonym, or looks inflected; Justin's judgement decides)", notForm)
	flagList("Highlights longer than three words", wide)
	flagList("ADR 0140 function words among the words (must be 0)", functionWords)
	b.WriteString(homographSection(results))
	b.WriteString("\n## Words\n\nA cited sense shows its native LSJ sense ID, its entry ID and the definition the provider saw. The translation's bold text is the English target highlight; the sentence's bold text is the target word. An omitted answer's Gloss and translation are shown only for context.\n\n")
	b.WriteString("| # | Lemma | Sentence | Gloss | Cited senses | Sentence translation | Passage citation | Outcome | Justin's judgement (acceptable / not, note) |\n|---|---|---|---|---|---|---|---|---|\n")
	for i, r := range results {
		b.WriteString(renderRow(i+1, r))
	}
	b.WriteString("\n## Judgement tally\n\nJustin fills this in after judging: acceptable Glosses ___ of ___ ready words; threshold (90%) met: yes / no.\n")
	return b.String()
}

// homographSection names the entry each homograph word's Gloss cites.
func homographSection(results []wordResult) string {
	var b strings.Builder
	b.WriteString("## Homograph words\n\nGreek Study offers every competing LSJ entry (ADR 0139). Words with more than one entry that has citable senses:\n\n")
	found := 0
	for i, r := range results {
		n := entryCount(r.Candidates)
		if n < 2 {
			continue
		}
		found++
		if r.Ready != nil && len(r.Ready.Cited.EntryIDs) > 0 {
			entry := r.Ready.Cited.EntryIDs[0]
			fmt.Fprintf(&b, "- **%s** (word %d): %d entries; the Gloss \"%s\" cites entry **%d of %d** (`%s`).\n", r.Identity.CanonicalLemma, i+1, n, r.Ready.Gloss, entryPosition(r.Candidates, entry), n, entry)
		} else {
			fmt.Fprintf(&b, "- **%s** (word %d): %d entries; outcome %s, no cited entry.\n", r.Identity.CanonicalLemma, i+1, n, r.Outcome)
		}
	}
	if found == 0 {
		b.WriteString("None of the words had more than one entry.\n")
	}
	return b.String()
}

// entryOrder lists the entry IDs of a request's candidate senses in order.
func entryOrder(senses []enrichment.LexicalSense) []string {
	var order []string
	for _, sense := range senses {
		entryID, _, ok := enrichment.SplitEntryEvidenceID(sense.EvidenceID)
		if ok && !slicesContains(order, entryID) {
			order = append(order, entryID)
		}
	}
	return order
}

func slicesContains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

func entryCount(senses []enrichment.LexicalSense) int { return len(entryOrder(senses)) }

func entryPosition(senses []enrichment.LexicalSense, entryID string) int {
	for i, id := range entryOrder(senses) {
		if id == entryID {
			return i + 1
		}
	}
	return 0
}
