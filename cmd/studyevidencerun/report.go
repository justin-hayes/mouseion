package main

import (
	"fmt"
	"sort"
	"strings"

	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/lexical"
	"github.com/justin-hayes/mouseion/internal/sourceevidence"
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

// languageFacts are the per-language words of the report.
type languageFacts struct {
	Name, Dictionary, DictionaryName, Treebank, Branch, Issue, Account string
	Contract                                                           string
	ExcludedADR                                                        string
}

func factsFor(language string) languageFacts {
	if language == "la" {
		return languageFacts{
			Name: "Latin", Dictionary: "Lewis & Short", DictionaryName: "L&S",
			Treebank: "OLA v0.2.0 (Opera Latina Adnotata, LatinPipe output; Zenodo `10.5281/zenodo.15183688`)",
			Branch:   "prototype/2768-latin-evidence-run", Issue: "2768", Account: "Justin's Latin (`la`)",
			Contract: "Study's Latin pass, which looks up every Lewis & Short entry under the lemma's key (stacked homographs) and carries each sense's entry handle",
		}
	}
	return languageFacts{
		Name: "Greek", Dictionary: "LSJ", DictionaryName: "LSJ",
		Treebank: "OGA `oga_src` v0.2.0 annotations (Zenodo `10.5281/zenodo.15199546`)",
		Branch:   "prototype/2767-greek-evidence-run", Issue: "2767", Account: "Justin's Greek (`grc`)",
		Contract: "Study's Greek stacked-homograph pass, which looks up every competing LSJ entry (ADR 0139) and carries each sense's entry handle",
	}
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
			if n := r.entryTotal(); n > 1 {
				label = fmt.Sprintf(" [entry %d of %d]", r.entryPos(entryID), n)
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
		if lexical.IsVocabularyStopword(meta.Language, r.Identity.CanonicalLemma, r.Identity.UPOS) ||
			(meta.Language == "la" && slicesContains(adr0140Latin, r.Identity.CanonicalLemma) && r.Identity.UPOS == "ADV") {
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

	facts := factsFor(meta.Language)
	fmt.Fprintf(&b, "# %s Study evidence run, %s\n\n", facts.Name, meta.Date)
	fmt.Fprintf(&b, "This is the evidence run for turning %s's Study switch on ([#%s](https://github.com/justin-hayes/mouseion/issues/%s), spec", facts.Name, facts.Issue, facts.Issue)
	b.WriteString(" [#2759](https://github.com/justin-hayes/mouseion/issues/2759), [ADR 0138](../adr/0138-study-for-source-evidence-languages.md) §6). It runs the production Study material path once per word and records what Study would have shown. Nothing here was written to a database. Justin judges each row; the last column is his.\n\n")
	b.WriteString("## Threshold\n\nStated before the run (ADR 0138 §6): **at least 90% of Glosses acceptable, and no answer cites an invented sense ID.** Justin's judgement column decides the first half. The second half is counted automatically below.\n\n")
	b.WriteString("## Inputs and pins\n\n")
	fmt.Fprintf(&b, "- **Words:** the first %d in Study's new-word order for %s account: Current reading first, then Personal occurrences descending, then lemma. Words already in study or held by an open Wrong-word report are skipped, as Study skips them.\n", len(results), facts.Account)
	fmt.Fprintf(&b, "- **Books drawn on:** %s.\n", pins(meta.BookList))
	fmt.Fprintf(&b, "- **Model:** `%s`, reasoning effort `%s`.\n", meta.Model, meta.Reasoning)
	fmt.Fprintf(&b, "- **Prompt contract (provider version):** `%s`, payload kind `%s`: %s.\n", meta.ProviderVersion, meta.PayloadKind, facts.Contract)
	fmt.Fprintf(&b, "- **Dictionary pin:** %s index `%s`.\n", facts.Dictionary, meta.DictionaryPin)
	fmt.Fprintf(&b, "- **Translation pins:** %s.\n", pins(meta.TranslationPins))
	fmt.Fprintf(&b, "- **Treebank:** %s annotations; analysis run(s): %s.\n", facts.Treebank, pins(meta.Analyzers))
	fmt.Fprintf(&b, "- **Gate:** `ProviderAssessments().MaySend` accepted the compiled codec for `%s` before anything was sent.\n", meta.Language)
	fmt.Fprintf(&b, "- **Provider calls made:** %d HTTP requests; the largest request body was %d bytes (one call per word that had a dictionary entry with citable senses, plus any retry of a failed request; the other words cost no call).\n", meta.Calls, meta.MaxRequest)
	fmt.Fprintf(&b, "- **Tool:** `cmd/studyevidencerun` on the branch `%s`, never merged. It runs `study.MaterialWorker` unchanged against a restored copy of the database, with a store that records nothing.\n\n", facts.Branch)

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
	if meta.Language == "la" {
		var sameLemma []string
		for _, r := range results {
			if slicesContains(adr0140Latin, r.Identity.CanonicalLemma) {
				sameLemma = append(sameLemma, fmt.Sprintf("%s (%s)", r.Identity.CanonicalLemma, r.Identity.UPOS))
			}
		}
		flagList("Words whose lemma is on ADR 0140's Latin list under any part of speech (a content-word homograph such as VERB *uero* may stay eligible)", sameLemma)
	}
	b.WriteString(homographSection(meta.Language, results))
	fmt.Fprintf(&b, "\n## Words\n\nA cited sense shows its native %s sense ID, its entry ID and the definition the provider saw. The translation's bold text is the English target highlight; the sentence's bold text is the target word. An omitted answer's Gloss and translation are shown only for context.\n\n", facts.DictionaryName)
	b.WriteString("| # | Lemma | Sentence | Gloss | Cited senses | Sentence translation | Passage citation | Outcome | Justin's judgement (acceptable / not, note) |\n|---|---|---|---|---|---|---|---|---|\n")
	for i, r := range results {
		b.WriteString(renderRow(i+1, r))
	}
	b.WriteString("\n## Judgement tally\n\nJustin fills this in after judging: acceptable Glosses ___ of ___ ready words; threshold (90%) met: yes / no.\n")
	return b.String()
}

// entryTotal is the number of dictionary entries the lemma resolves to; it
// falls back to the entries with citable senses when the tool kept none.
func (r wordResult) entryTotal() int {
	if len(r.Entries) > 0 {
		return len(r.Entries)
	}
	return entryCount(r.Candidates)
}

// entryPos is the 1-based place of an entry among the lemma's entries.
func (r wordResult) entryPos(entryID string) int {
	for i, entry := range r.Entries {
		if entry.ID == entryID {
			return i + 1
		}
	}
	return entryPosition(r.Candidates, entryID)
}

// entryLabel is a short label for one dictionary entry: its number, headword
// and the opening of its first definition, or its cross-reference.
func entryLabel(entry sourceevidence.Entry) string {
	var parts []string
	headword := entry.LewisShort.Headword
	if headword == "" {
		headword = entry.LSJ.Headword
	}
	if headword != "" {
		parts = append(parts, headword)
	}
	text := ""
	for _, sense := range entry.Senses {
		if len(sense.Definitions) > 0 {
			text = strings.Join(sense.Definitions, "; ")
			break
		}
		if len(sense.Glosses) > 0 {
			text = strings.Join(sense.Glosses, "; ")
			break
		}
	}
	if text == "" && len(entry.Targets) > 0 {
		text = "see " + entry.Targets[0].Reference + " (no sense of its own)"
	}
	if text == "" {
		text = "no citable sense"
	}
	runes := []rune(text)
	if len(runes) > 70 {
		text = string(runes[:70]) + "…"
	}
	parts = append(parts, text)
	return strings.Join(parts, ": ")
}

// homographSection names every entry of each homograph word and the entry its
// Gloss cites, and flags the word for the learner's judgement.
func homographSection(language string, results []wordResult) string {
	facts := factsFor(language)
	var b strings.Builder
	fmt.Fprintf(&b, "## Homograph words\n\nWords with more than one %s entry (stacked homographs). Every entry's number and a short label are listed, and the entry the Gloss cited is marked **cited**. **Each is flagged for judgement: a Gloss that cites the wrong entry is not acceptable.**\n\n", facts.Dictionary)
	found := 0
	for i, r := range results {
		n := r.entryTotal()
		if n < 2 {
			continue
		}
		found++
		cited := map[string]bool{}
		if r.Ready != nil {
			for _, id := range r.Ready.Cited.EntryIDs {
				cited[id] = true
			}
		}
		gloss := "no Gloss (" + r.Outcome + ")"
		if r.Ready != nil {
			gloss = fmt.Sprintf("Gloss \"%s\"", r.Ready.Gloss)
		}
		fmt.Fprintf(&b, "- **%s** (word %d), %d entries, %s. **[homograph: judge the cited entry]**\n", r.Identity.CanonicalLemma, i+1, n, gloss)
		for k, entry := range r.Entries {
			mark := ""
			if cited[entry.ID] {
				mark = " **← cited**"
			}
			number := k + 1
			if entry.Number > 0 {
				number = entry.Number
			}
			fmt.Fprintf(&b, "  - entry %d (`%s`): %s%s\n", number, entry.ID, cell(entryLabel(entry)), mark)
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

// adr0140Latin is ADR 0140's closed list of Latin ADV function words, as
// canonical lemmas.
var adr0140Latin = []string{"et", "ue", "uel", "nec", "atque", "enim", "nam", "uero", "igitur", "ergo", "itaque", "tamen", "quoque", "quidem", "etiam", "non", "ne", "nonne", "num", "haud"}
