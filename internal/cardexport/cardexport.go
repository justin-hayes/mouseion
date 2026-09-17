// Package cardexport produces owner-scoped Anki recognition deck packages.
package cardexport

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"math"
	"slices"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/dictionary"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/gdex"
	"github.com/justin-hayes/mouseion/internal/lemmadisplay"
	"github.com/justin-hayes/mouseion/internal/textmatch"
)

var ErrInvalidInput = errors.New("cardexport: invalid input")

type Entry struct {
	OwnerID, Language, CanonicalLemma, UPOS                                           string
	CorpusID                                                                          string
	SentenceOrdinal                                                                   int64
	Sentence, Translation, SentenceTranslation, SentenceTranslationTarget, TargetWord string
	Gloss, Plural, IPA, PrincipalParts, DictionaryProviderVersion                     string
	Morphology, SourceDocument, Notes                                                 string
	CandidateSenses                                                                   []enrichment.LexicalSense
	SentenceTokens                                                                    []analyzer.Token
	FirstEncounter                                                                    int64
	fallbackGlossApplied                                                              bool
}

type Note struct {
	Key, Identity, Text, Article, Lemma, Plural, IPA, PrincipalParts, POS, Gloss, English, EnglishSentence, BookTitle string
	BackExtra                                                                                                         string
	Tags                                                                                                              []string
}

type Artifact struct {
	APKG                    []byte
	Filename, DeckName, TSV string
	Count                   int
	Completeness            Completeness
	Diagnostics             Diagnostics
	Omitted                 []Omission
	EnrichmentCandidates    []enrichment.Candidate
	Generated               []GeneratedRecord
}

// The private manifest freezes every selection and render decision made for one
// prepared deck before optional external enrichment starts. Its fields are
// private and all slice-returning methods copy their data so later learner
// state, database rows, or caller mutation cannot change the final artifact.
type manifest struct {
	owner                string
	deckName             string
	schemaVersion        int
	accepted             []RenderInput
	omitted              []Omission
	enrichmentCandidates []enrichment.Candidate
	cacheKeys            []enrichment.CacheKey
	decisions            []ManifestItem
}

// ExactEnrichment is the result for one manifest candidate under the exact
// cache identity used by the enrichment service.
type ExactEnrichment struct {
	CacheKey enrichment.CacheKey
	Result   enrichment.Result
}

// GeneratedRecord is the card provenance to persist only after an artifact is
// successfully rendered. Prepared-deck completion persists these atomically.
type GeneratedRecord struct {
	Input RenderInput
	Note  Note
}

// Completeness reports which optional enrichment fields were available for
// accepted cards. QualityOmitted counts selected candidates that were not
// exported because their best source sentence failed the quality gate.
type Completeness struct {
	TotalCards               int
	CardsWithEnglish         int
	CardsWithEnglishSentence int
	CardsWithFallbackGloss   int
	QualityOmitted           int
}

type Omission struct {
	Language, CanonicalLemma, UPOS string
	Score                          int
	Reasons                        []string
}

type SentenceQuality struct {
	Accepted  bool
	Score     int
	Reasons   []string
	GDEXScore float64
}

type SentenceEvidence struct {
	Sentence, Target string
	CorpusID         string
	FirstEncounter   int64
	SentenceOrdinal  int64
	Quality          SentenceQuality
	Tokens           []analyzer.Token
}

type lexicalResolver struct {
	lexical enrichment.LexicalProvider
}

func DedupKey(language, lemma, upos, owner string) string {
	sum := sha256.Sum256([]byte(language + " | " + lemma + " | " + upos + " | " + owner))
	return hex.EncodeToString(sum[:])
}

// CardIdentity returns the stable, owner-scoped identity exported as Anki's
// sort and duplicate-detection field. It is deliberately separate from Key:
// Key preserves the existing lemma-level note GUID and persistence semantics,
// while Identity distinguishes the concrete target and source occurrence.
func CardIdentity(owner string, input RenderInput) string {
	target := testedRenderTarget(input)
	inputs := struct {
		Version        int    `json:"version"`
		Owner          string `json:"owner"`
		Language       string `json:"language"`
		CanonicalLemma string `json:"canonical_lemma"`
		UPOS           string `json:"upos"`
		Target         string `json:"target"`
		SourceSentence string `json:"source_sentence"`
		SourceDocument string `json:"source_document"`
		FirstEncounter int64  `json:"first_encounter"`
	}{
		Version: 1, Owner: owner, Language: input.Language,
		CanonicalLemma: input.CanonicalLemma, UPOS: input.UPOS, Target: target,
		SourceSentence: input.Sentence, SourceDocument: input.SourceDocument,
		FirstEncounter: input.FirstEncounter,
	}
	payload, err := json.Marshal(inputs)
	if err != nil {
		panic(fmt.Sprintf("card identity inputs are not JSON encodable: %v", err))
	}
	sum := sha256.Sum256(append([]byte("mouseion-card-identity-v1\x00"), payload...))
	return hex.EncodeToString(sum[:])
}

func BoldTarget(sentence, target string) (string, error) {
	return boldTargets(sentence, []string{target})
}

// boldTargets renders sentence with each target bolded at its first
// case-insensitive, word-bounded occurrence. Targets that do not occur in the
// sentence, or that overlap an earlier matched span, are ignored. At least one
// target must match or ErrInvalidInput is returned.
func boldTargets(sentence string, targets []string) (string, error) {
	if strings.TrimSpace(sentence) == "" {
		return "", ErrInvalidInput
	}
	type span struct{ start, end int }
	spans := make([]span, 0, len(targets))
	for _, target := range targets {
		target = textmatch.CleanLexicalSurface(target)
		if target == "" {
			continue
		}
		start, end, ok := textmatch.FoldedWordSpan(sentence, target)
		if !ok {
			continue
		}
		spans = append(spans, span{start, end})
	}
	if len(spans) == 0 {
		return "", fmt.Errorf("%w: target not found in sentence", ErrInvalidInput)
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
	var out strings.Builder
	last := 0
	for _, s := range spans {
		if s.start < last {
			continue
		}
		out.WriteString(escapeField(sentence[last:s.start]))
		out.WriteString("<b>")
		out.WriteString(escapeField(sentence[s.start:s.end]))
		out.WriteString("</b>")
		last = s.end
	}
	out.WriteString(escapeField(sentence[last:]))
	return out.String(), nil
}

// targetSurfaces resolves the surfaces to bold for a card. It returns the
// observed target followed by each compound:prt particle token whose head is the
// token matching the observed target, resolved from the persisted dependency
// parse. Without a parse, or when no token matches the target, only the observed
// target is returned. Duplicate surfaces are collapsed so repeated exports are
// stable.
func targetSurfaces(target string, tokens []analyzer.Token) []string {
	target = textmatch.CleanLexicalSurface(target)
	if target == "" {
		return nil
	}
	spans := []string{target}
	seen := map[string]bool{strings.ToLower(target): true}
	for index, token := range tokens {
		if !strings.EqualFold(textmatch.CleanLexicalSurface(token.Surface), target) {
			continue
		}
		for _, dependent := range tokens {
			if dependent.Dependency != "compound:prt" || uint64(dependent.Head) != uint64(index) {
				continue
			}
			surface := textmatch.CleanLexicalSurface(dependent.Surface)
			if surface == "" || seen[strings.ToLower(surface)] {
				continue
			}
			seen[strings.ToLower(surface)] = true
			spans = append(spans, surface)
		}
	}
	return spans
}

// HighlightEnglishTarget renders an optional provider alignment only when it
// is a unique, word-bounded match in the complete translation. The provider
// supplies plain text; all HTML is owned and escaped by Mouseion.
func HighlightEnglishTarget(translation, target string) string {
	start, end, ok := uniqueTargetMatch(translation, target)
	if !ok {
		return escapeField(translation)
	}
	return escapeField(translation[:start]) + "<b>" + escapeField(translation[start:end]) + "</b>" + escapeField(translation[end:])
}

func uniqueTargetMatch(text, target string) (int, int, bool) {
	target = strings.TrimSpace(target)
	if target == "" || !utf8.ValidString(text) || !utf8.ValidString(target) || !hasWordOrNumber(target) {
		return 0, 0, false
	}
	start, end := -1, -1
	for offset := range text {
		matchEnd := offset + len(target)
		if matchEnd > len(text) || (matchEnd < len(text) && !utf8.RuneStart(text[matchEnd])) || !wordBoundaryBefore(text, offset) || !wordBoundaryAfter(text, matchEnd) {
			continue
		}
		if !strings.EqualFold(text[offset:matchEnd], target) {
			continue
		}
		if start >= 0 {
			return 0, 0, false
		}
		start, end = offset, matchEnd
	}
	return start, end, start >= 0
}

func hasWordOrNumber(value string) bool {
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			return true
		}
	}
	return false
}

func wordBoundaryBefore(text string, index int) bool {
	if index == 0 {
		return true
	}
	if index < 0 || index > len(text) || !utf8.RuneStart(text[index]) {
		return false
	}
	r, _ := utf8.DecodeLastRuneInString(text[:index])
	return !unicode.IsLetter(r) && !unicode.IsNumber(r)
}

func wordBoundaryAfter(text string, index int) bool {
	if index == len(text) {
		return true
	}
	if index < 0 || index > len(text) || !utf8.RuneStart(text[index]) {
		return false
	}
	r, _ := utf8.DecodeRuneInString(text[index:])
	return !unicode.IsLetter(r) && !unicode.IsNumber(r)
}

func escapeField(value string) string {
	value = html.EscapeString(value)
	// Anki serializes note fields with U+001F as the field separator. Never let
	// source or provider text inject an extra field into an APKG note.
	value = strings.ReplaceAll(value, "\x1f", "&#31;")
	value = strings.ReplaceAll(value, "{", "&#123;")
	return strings.ReplaceAll(value, "}", "&#125;")
}

const (
	minimumSentenceWords         = 6
	maximumSentenceWords         = 50
	maximumSentenceCharacters    = 400
	minimumSentenceScore         = 70
	reasonNoFiniteVerbAndSubject = "no finite verb and subject"
)

// ScoreSentenceQuality applies a deliberately small, explainable export gate
// using only sentence text, the selected target form, and source location.
func ScoreSentenceQuality(sentence, target string, firstEncounter int64) SentenceQuality {
	return scoreSentenceQuality("", sentence, target, firstEncounter, nil)
}

func scoreSentenceQuality(language, sentence, target string, firstEncounter int64, tokens []analyzer.Token) SentenceQuality {
	text := strings.TrimSpace(sentence)
	words := strings.Fields(text)
	quality := SentenceQuality{Reasons: make([]string, 0, 8)}
	reject := func(reason string) { quality.Reasons = append(quality.Reasons, reason) }

	if !utf8.ValidString(text) || len(words) < minimumSentenceWords {
		reject("too short or fragmented")
	} else if sentenceTooLong(text) {
		reject("too long")
	} else {
		quality.Score += 30
		quality.Reasons = append(quality.Reasons, "usable length")
		if len(words) >= 8 && len(words) <= 30 {
			quality.Score += 10
			quality.Reasons = append(quality.Reasons, "useful context window")
		}
	}
	if targetIndex(text, strings.TrimSpace(target)) < 0 {
		reject("target not present as a word")
	} else {
		quality.Score += 25
		quality.Reasons = append(quality.Reasons, "target present")
	}
	if firstEncounter < 0 || firstEncounter == math.MaxInt64 {
		reject("invalid source location")
	} else {
		quality.Score += 15
		quality.Reasons = append(quality.Reasons, "valid source location")
	}
	if hasCompleteBoundary(text) {
		quality.Score += 20
		quality.Reasons = append(quality.Reasons, "complete sentence boundaries")
	} else {
		reject("incomplete sentence boundaries")
	}
	if structuralNoise(text) {
		reject("structural noise or boilerplate")
	} else {
		quality.Score += 10
		quality.Reasons = append(quality.Reasons, "no obvious structural noise")
	}
	if tokens != nil && isGerman(language) {
		targetIndices := make([]int, 0, 1)
		cleanTarget := textmatch.CleanLexicalSurface(target)
		for index, token := range tokens {
			if cleanTarget != "" && strings.EqualFold(textmatch.CleanLexicalSurface(token.Surface), cleanTarget) {
				targetIndices = append(targetIndices, index)
			}
		}
		gdexQuality := gdex.ScoreSentenceQuality(analyzer.Sentence{Text: text, Tokens: tokens}, targetIndices)
		quality.GDEXScore = gdexQuality.Score
		for _, reason := range gdexQuality.Reasons {
			if !slices.Contains(quality.Reasons, reason) {
				quality.Reasons = append(quality.Reasons, reason)
			}
		}
		if !gdexQuality.Accepted {
			quality.Score = 0
			if !slices.Contains(quality.Reasons, reasonNoFiniteVerbAndSubject) {
				reject(reasonNoFiniteVerbAndSubject)
			}
		}
	}

	quality.Accepted = quality.Score >= minimumSentenceScore && !containsRejection(quality.Reasons)
	return quality
}

func isGerman(language string) bool {
	base, _, _ := strings.Cut(strings.ToLower(strings.ReplaceAll(strings.TrimSpace(language), "_", "-")), "-")
	return base == "de"
}

func sentenceTooLong(text string) bool {
	text = strings.TrimSpace(text)
	return len(strings.Fields(text)) > maximumSentenceWords || utf8.RuneCountInString(text) > maximumSentenceCharacters
}

func containsRejection(reasons []string) bool {
	for _, reason := range reasons {
		switch reason {
		case "too short or fragmented", "too long", "target not present as a word", "invalid source location", "incomplete sentence boundaries", "structural noise or boilerplate", reasonNoFiniteVerbAndSubject:
			return true
		}
	}
	return false
}

func hasCompleteBoundary(text string) bool {
	if text == "" || !strings.ContainsAny(text[len(text)-1:], ".!?") {
		return false
	}
	first, _ := utf8.DecodeRuneInString(text)
	return unicode.IsUpper(first) || unicode.IsNumber(first) || strings.ContainsRune("\"'“„«", first)
}

func structuralNoise(text string) bool {
	lower := strings.ToLower(strings.TrimSpace(text))
	for _, marker := range []string{"table of contents", "inhaltsverzeichnis", "bibliography", "bibliografie", "references", "literaturverzeichnis", "notes", "anmerkungen", "footnote", "endnote", "chapter", "kapitel", "section", "abschnitt"} {
		if structuralHeading(lower, marker) {
			return true
		}
	}
	return strings.Contains(lower, "all rights reserved") || strings.Contains(lower, "project gutenberg") ||
		strings.Contains(lower, ".....") || strings.Count(text, "\t") >= 2 ||
		listOrPageFragment(text) || citationDense(text) || extractionAnomaly(text)
}

func listOrPageFragment(text string) bool {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return true
	}
	firstLine := strings.TrimSpace(strings.SplitN(trimmed, "\n", 2)[0])
	if strings.Contains(trimmed, "\n") || strings.HasPrefix(firstLine, "•") || strings.HasPrefix(firstLine, "- ") || strings.HasPrefix(firstLine, "* ") {
		return true
	}
	fields := strings.Fields(firstLine)
	if len(fields) <= 12 && len(fields) > 0 {
		prefix := strings.TrimRight(fields[0], ".)")
		if _, err := fmt.Sscan(prefix, new(int)); err == nil {
			return true
		}
	}
	allDigits := true
	for _, r := range strings.Trim(trimmed, "-–— ") {
		if !unicode.IsDigit(r) {
			allDigits = false
			break
		}
	}
	return allDigits
}

func citationDense(text string) bool {
	brackets := strings.Count(text, "[") + strings.Count(text, "]")
	years := 0
	for year := 1500; year <= 2099; year++ {
		if strings.Contains(text, fmt.Sprintf("%d", year)) {
			years++
		}
	}
	lower := strings.ToLower(text)
	citationMarkers := strings.Count(lower, " et al.") + strings.Count(lower, "doi:") + strings.Count(lower, " pp.") + strings.Count(lower, " vol.")
	return brackets >= 4 || years >= 2 || citationMarkers >= 2 || strings.Count(lower, "doi:") >= 1
}

func extractionAnomaly(text string) bool {
	controlOrReplacement := 0
	for _, r := range text {
		if r == unicode.ReplacementChar || (unicode.IsControl(r) && r != '\n' && r != '\t') {
			controlOrReplacement++
		}
	}
	words := strings.Fields(strings.ToLower(text))
	repeated := 0
	for i := 1; i < len(words); i++ {
		if strings.Trim(words[i], ".,;:!?") == strings.Trim(words[i-1], ".,;:!?") {
			repeated++
		}
	}
	return controlOrReplacement > 0 || repeated >= 3 || strings.Contains(text, "\u00ad \u00ad")
}

type sentenceReference struct {
	SentenceIndex int             `json:"sentence_index"`
	Text          string          `json:"text"`
	Location      json.RawMessage `json:"location"`
}

// BestSentenceEvidence ranks every text-only source reference without changing
// the candidate's first-encounter ordering. Persisted exports use
// BestSentenceEvidenceFromCorpus below.
func BestSentenceEvidence(candidate domain.SelectionCandidate) (SentenceEvidence, bool) {
	return bestSentenceEvidence(candidate, nil)
}

// BestSentenceEvidenceFromCorpus ranks source references using the persisted
// sentence text and dependency tokens for each sentence ordinal.
func BestSentenceEvidenceFromCorpus(candidate domain.SelectionCandidate, sentences map[int64]analyzer.Sentence) (SentenceEvidence, bool) {
	return bestSentenceEvidence(candidate, sentences)
}

func bestSentenceEvidence(candidate domain.SelectionCandidate, persisted map[int64]analyzer.Sentence) (SentenceEvidence, bool) {
	var refs []sentenceReference
	if json.Unmarshal(candidate.SentenceReferences, &refs) != nil {
		return SentenceEvidence{}, false
	}
	var forms []string
	if err := json.Unmarshal(candidate.ObservedForms, &forms); err != nil {
		return SentenceEvidence{}, false
	}
	forms = append(forms, candidate.CanonicalLemma)
	type rankedSentence struct {
		evidence SentenceEvidence
		sentence int
	}
	ranked := make([]rankedSentence, 0, len(refs))
	for _, ref := range refs {
		text := ref.Text
		var tokens []analyzer.Token
		if persisted != nil {
			sentence, ok := persisted[int64(ref.SentenceIndex)]
			if !ok {
				continue
			}
			text = sentence.Text
			tokens = sentence.Tokens
			if tokens == nil {
				tokens = []analyzer.Token{}
			}
		}
		location, valid := referenceStartOffset(ref.Location)
		if !valid {
			location = -1
		}
		target := ""
		for _, form := range forms {
			cleanForm := textmatch.CleanLexicalSurface(form)
			if targetIndex(text, cleanForm) >= 0 {
				target = cleanForm
				break
			}
		}
		quality := scoreSentenceQuality(candidate.Language, text, target, location, tokens)
		var evidenceTokens []analyzer.Token
		if persisted != nil {
			evidenceTokens = append([]analyzer.Token{}, tokens...)
		}
		ranked = append(ranked, rankedSentence{evidence: SentenceEvidence{Sentence: text, Target: target, FirstEncounter: location, Quality: quality, Tokens: evidenceTokens}, sentence: ref.SentenceIndex})
	}
	if len(ranked) == 0 {
		return SentenceEvidence{}, false
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].evidence.Quality.Accepted != ranked[j].evidence.Quality.Accepted {
			return ranked[i].evidence.Quality.Accepted
		}
		if ranked[i].evidence.Quality.GDEXScore != ranked[j].evidence.Quality.GDEXScore {
			return ranked[i].evidence.Quality.GDEXScore > ranked[j].evidence.Quality.GDEXScore
		}
		if ranked[i].evidence.FirstEncounter != ranked[j].evidence.FirstEncounter {
			return ranked[i].evidence.FirstEncounter < ranked[j].evidence.FirstEncounter
		}
		if ranked[i].sentence != ranked[j].sentence {
			return ranked[i].sentence < ranked[j].sentence
		}
		return ranked[i].evidence.Sentence < ranked[j].evidence.Sentence
	})
	evidence := ranked[0].evidence
	evidence.CorpusID = candidate.CorpusID
	evidence.SentenceOrdinal = int64(ranked[0].sentence)
	return evidence, true
}

func referenceStartOffset(raw json.RawMessage) (int64, bool) {
	var fields map[string]json.RawMessage
	if json.Unmarshal(raw, &fields) != nil {
		return 0, false
	}
	for _, key := range []string{"start_offset", "StartOffset"} {
		if value, ok := fields[key]; ok {
			var offset uint64
			if json.Unmarshal(value, &offset) == nil && offset <= math.MaxInt64 {
				return int64(offset), true
			}
		}
	}
	return 0, false
}

func structuralHeading(text, marker string) bool {
	if !strings.HasPrefix(text, marker) {
		return false
	}
	rest := strings.TrimSpace(strings.TrimPrefix(text, marker))
	return rest == "" || strings.ContainsRune(":.-–—0123456789", []rune(rest)[0])
}

func targetIndex(sentence, target string) int {
	target = textmatch.CleanLexicalSurface(target)
	start, _, ok := textmatch.FoldedWordSpan(sentence, target)
	if ok {
		return start
	}
	return -1
}

func nounArticle(language, upos, lemma, morphology string) string {
	language = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(language), "_", "-"))
	baseLanguage, _, _ := strings.Cut(language, "-")
	if !strings.EqualFold(strings.TrimSpace(upos), "NOUN") {
		return ""
	}
	variants, ok := morphologyVariants(morphology)
	if !ok {
		return ""
	}
	var articles map[string]string
	var validArticles map[string]bool
	pluralArticle := ""
	switch baseLanguage {
	case "de":
		articles = map[string]string{"masc": "der", "masculine": "der", "fem": "die", "feminine": "die", "neut": "das", "neuter": "das"}
		validArticles = map[string]bool{"der": true, "die": true, "das": true}
		pluralArticle = "die"
	case "it":
		articles = map[string]string{"masc": "masc", "masculine": "masc", "fem": "fem", "feminine": "fem"}
		validArticles = map[string]bool{"il": true, "lo": true, "la": true, "l'": true}
	default:
		return ""
	}
	if len(variants) == 0 {
		return ""
	}
	for _, variant := range variants {
		if article := morphologyValue(variant, "Article"); article != "" {
			if validArticles[article] {
				return article
			}
		}
	}
	genders := make(map[string]bool, len(articles))
	allPlural := true
	for _, variant := range variants {
		gender := morphologyValue(variant, "Gender")
		if gender != "" {
			article, ok := articles[gender]
			if !ok {
				return ""
			}
			genders[article] = true
		}
		number := morphologyValue(variant, "Number")
		allPlural = allPlural && (number == "plur" || number == "plural")
	}
	if len(genders) == 1 {
		for article := range genders {
			if baseLanguage == "it" {
				return italianDefiniteArticle(article, lemma)
			}
			return article
		}
	}
	if len(genders) == 0 && allPlural {
		return pluralArticle
	}
	return ""
}

func italianDefiniteArticle(gender, lemma string) string {
	lemma = strings.ToLower(strings.TrimSpace(lemma))
	if gender == "fem" {
		if italianStartsWithVowel(lemma) {
			return "l'"
		}
		return "la"
	}
	if italianStartsWithVowel(lemma) {
		return "l'"
	}
	if italianTakesLo(lemma) {
		return "lo"
	}
	return "il"
}

func italianStartsWithVowel(value string) bool {
	runeValue, _ := utf8.DecodeRuneInString(value)
	return strings.ContainsRune("aeiouàèéìòóù", runeValue)
}

func italianTakesLo(value string) bool {
	if value == "" {
		return false
	}
	if strings.HasPrefix(value, "z") || strings.HasPrefix(value, "x") || strings.HasPrefix(value, "y") || strings.HasPrefix(value, "gn") || strings.HasPrefix(value, "ps") || strings.HasPrefix(value, "pn") {
		return true
	}
	if !strings.HasPrefix(value, "s") {
		return false
	}
	_, size := utf8.DecodeRuneInString(value)
	next, _ := utf8.DecodeRuneInString(value[size:])
	return next != utf8.RuneError && !italianStartsWithVowel(string(next)) && unicode.IsLetter(next)
}

func morphologyVariants(morphology string) ([]map[string]string, bool) {
	var variants []map[string]string
	if err := json.Unmarshal([]byte(morphology), &variants); err == nil {
		return variants, true
	}
	var single map[string]string
	if json.Unmarshal([]byte(morphology), &single) != nil || single == nil {
		return nil, false
	}
	return []map[string]string{single}, true
}

func morphologyValue(morphology map[string]string, key string) string {
	for candidate, value := range morphology {
		if strings.EqualFold(strings.TrimSpace(candidate), key) {
			return strings.ToLower(strings.TrimSpace(value))
		}
	}
	return ""
}

func testedEntryTarget(entry Entry) string {
	return testedTarget(entry.TargetWord, entry.CanonicalLemma)
}

func testedRenderTarget(input RenderInput) string {
	return testedTarget(input.TargetWord, input.CanonicalLemma)
}

func testedTarget(targetWord, canonicalLemma string) string {
	target := textmatch.CleanLexicalSurface(targetWord)
	if target == "" {
		target = textmatch.CleanLexicalSurface(canonicalLemma)
	}
	return target
}

func makeNote(owner string, input RenderInput) (Note, error) {
	target := testedRenderTarget(input)
	front, err := boldTargets(input.Sentence, targetSurfaces(target, input.SentenceTokens))
	if err != nil {
		return Note{}, err
	}
	tags := uniqueTags("Mouseion", prefixedTag("lang", input.Language), prefixedTag("pos", input.UPOS), prefixedTag("source", input.SourceDocument))
	displayLemma := lemmadisplay.Format(input.Language, input.CanonicalLemma, input.UPOS)
	article := nounArticle(input.Language, input.UPOS, displayLemma, input.Morphology)
	note := Note{
		Key: DedupKey(input.Language, input.CanonicalLemma, input.UPOS, owner), Identity: CardIdentity(owner, input),
		Text: front, Article: escapeField(article), Lemma: escapeField(displayLemma), POS: escapeField(input.UPOS),
		Plural:         escapeField(nounPlural(input.UPOS, input.Plural)),
		IPA:            escapeField(strings.TrimSpace(input.IPA)),
		PrincipalParts: escapeField(strings.TrimSpace(input.PrincipalParts)),
		Gloss:          escapeField(input.Gloss), English: escapeField(input.Translation),
		EnglishSentence: HighlightEnglishTarget(input.SentenceTranslation, input.SentenceTranslationTarget), BookTitle: escapeField(input.SourceDocument), Tags: tags,
	}
	articleLemma := note.Lemma
	if note.Article != "" {
		separator := " "
		if strings.HasSuffix(article, "'") {
			separator = ""
		}
		articleLemma = note.Article + separator + note.Lemma
	}
	if note.Plural != "" {
		articleLemma += " (Pl. " + note.Plural + ")"
	}
	back := []string{articleLemma}
	if note.IPA != "" {
		back = append(back, note.IPA)
	}
	if note.PrincipalParts != "" {
		back = append(back, note.PrincipalParts)
	}
	back = append(back, note.POS, note.Gloss, note.EnglishSentence)
	note.BackExtra = strings.Join(back, "\n")
	return note, nil
}

func nounPlural(upos, plural string) string {
	if !strings.EqualFold(strings.TrimSpace(upos), "NOUN") {
		return ""
	}
	return strings.TrimSpace(plural)
}

func prefixedTag(prefix, value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return prefix + "::" + value
}

func uniqueTags(values ...string) []string {
	var tags []string
	for _, value := range values {
		value = strings.Join(strings.Fields(value), "_")
		value = strings.NewReplacer("\\", "_", "/", "_", "#", "_", "^", "_", "\x00", "_").Replace(value)
		if value != "" && !slices.Contains(tags, value) {
			tags = append(tags, value)
		}
	}
	return tags
}

func RenderTSV(notes []Note, attribution string) (string, error) {
	var out bytes.Buffer
	if attribution = strings.TrimSpace(attribution); attribution != "" {
		_, _ = out.WriteString("# " + attribution + "\n")
	}
	w := csv.NewWriter(&out)
	w.Comma, w.UseCRLF = '\t', false
	for _, n := range notes {
		if err := w.Write(append(noteFields(n), strings.Join(n.Tags, " "))); err != nil {
			return "", err
		}
	}
	w.Flush()
	return out.String(), w.Error()
}

func noteFields(n Note) []string {
	return []string{n.Identity, n.Text, n.Article, n.Lemma, n.Plural, n.IPA, n.PrincipalParts, n.POS, n.Gloss, n.English, n.EnglishSentence, n.BookTitle}
}

const noteTypeName = "Mouseion Vocab Recognition"

var fieldNames = []string{"Identity", "Text", "Article", "Lemma", "Plural", "IPA", "PrincipalParts", "POS", "Gloss", "English", "EnglishSentence", "BookTitle"}

func DeckName(language, bookTitle string) string {
	return "Mouseion::" + strings.TrimSpace(language) + "::" + strings.TrimSpace(bookTitle)
}

func DownloadFilename(bookTitle string) string {
	original := strings.TrimSpace(bookTitle)
	var b strings.Builder
	hasNameRune := false
	for _, r := range original {
		if unicode.IsLetter(r) || unicode.IsNumber(r) {
			hasNameRune = true
		}
		if unicode.IsControl(r) || strings.ContainsRune(`<>:"/\\|?*`, r) {
			b.WriteRune('_')
		} else {
			b.WriteRune(r)
		}
	}
	name := strings.Trim(strings.Join(strings.Fields(b.String()), " "), " .")
	if len([]rune(name)) > 120 {
		name = string([]rune(name)[:120])
		name = strings.TrimRight(name, " .")
	}
	if name == "" || name == "." || name == ".." || !hasNameRune {
		sum := sha256.Sum256([]byte(original))
		name = "mouseion-deck-" + hex.EncodeToString(sum[:6])
	}
	return name + ".apkg"
}

func clearExternalFields(entry *Entry) {
	entry.Translation = ""
	entry.SentenceTranslation = ""
	entry.SentenceTranslationTarget = ""
}

func (r *lexicalResolver) resolveLexicalEntry(ctx context.Context, entry *Entry) error {
	if r == nil || r.lexical == nil || entry == nil {
		return nil
	}
	result, found, err := r.lexical.Lookup(ctx, enrichment.LexicalLookupRequest{
		Language: entry.Language, CanonicalLemma: entry.CanonicalLemma, UPOS: entry.UPOS,
		TargetWord: testedEntryTarget(*entry), RepresentativeSentence: strings.TrimSpace(entry.Sentence), SentenceTokens: entry.SentenceTokens,
	})
	if err != nil || !found {
		return err
	}
	gloss := enrichment.RenderGloss(result.Senses, enrichment.DefaultMaxSenses, enrichment.DefaultMaxTokens)
	plural := nounPlural(entry.UPOS, result.Plural)
	entry.Gloss = gloss
	senses := result.CandidateSenses
	if len(senses) == 0 {
		senses = result.Senses
	}
	if len(senses) > enrichment.DefaultMaxCandidateSenses {
		senses = senses[:enrichment.DefaultMaxCandidateSenses]
	}
	entry.CandidateSenses = enrichment.CloneLexicalSenses(senses)
	entry.Plural = plural
	entry.IPA = strings.TrimSpace(result.IPA)
	entry.PrincipalParts = strings.TrimSpace(result.PrincipalParts)
	entry.DictionaryProviderVersion = r.lexical.Version()
	if result.Gender == "" && result.Article == "" && plural == "" {
		return nil
	}
	morphology := map[string]string{}
	if err := json.Unmarshal([]byte(entry.Morphology), &morphology); err != nil {
		morphology = map[string]string{}
	}
	if result.Gender != "" {
		morphology["Gender"] = result.Gender
	}
	if result.Article != "" {
		morphology["Article"] = result.Article
	}
	if plural != "" {
		morphology["Plural"] = plural
	}
	encoded, err := json.Marshal(morphology)
	if err != nil {
		return fmt.Errorf("encode dictionary morphology: %w", err)
	}
	entry.Morphology = string(encoded)
	return nil
}

func applySentenceDecision(entry *Entry, candidate domain.SelectionCandidate, persisted map[int64]analyzer.Sentence) {
	var evidence SentenceEvidence
	var ok bool
	if persisted != nil {
		evidence, ok = BestSentenceEvidenceFromCorpus(candidate, persisted)
	} else {
		evidence, ok = BestSentenceEvidence(candidate)
	}
	if ok {
		entry.CorpusID = evidence.CorpusID
		entry.Sentence = evidence.Sentence
		entry.TargetWord = evidence.Target
		entry.FirstEncounter = evidence.FirstEncounter
		entry.SentenceOrdinal = evidence.SentenceOrdinal
		entry.SentenceTokens = evidence.Tokens
	} else {
		entry.TargetWord = targetWord(entry.Sentence, candidate)
		if persisted != nil {
			// An unavailable persisted sentence is not eligible for syntax scoring.
			entry.SentenceTokens = []analyzer.Token{}
		}
	}
}

// newManifest quality-gates entries once and returns an immutable render plan.
func newManifest(owner, deckName string, entries []Entry) manifest {
	manifest := manifest{owner: owner, deckName: deckName, schemaVersion: ManifestSchemaVersion, accepted: make([]RenderInput, 0, len(entries)), omitted: make([]Omission, 0), enrichmentCandidates: make([]enrichment.Candidate, 0, len(entries)), decisions: make([]ManifestItem, 0, len(entries))}
	for ordinal, entry := range entries {
		entry = cloneEntry(entry)
		entry.UPOS = strings.ToUpper(strings.TrimSpace(entry.UPOS))
		entry.TargetWord = testedEntryTarget(entry)
		if len(entry.CandidateSenses) > enrichment.DefaultMaxCandidateSenses {
			entry.CandidateSenses = enrichment.CloneLexicalSenses(entry.CandidateSenses[:enrichment.DefaultMaxCandidateSenses])
		}
		decisionEntry := cloneEntry(entry)
		clearExternalFields(&decisionEntry)
		decisionEntry.OwnerID = ""
		quality := scoreSentenceQuality(entry.Language, entry.Sentence, entry.TargetWord, entry.FirstEncounter, entry.SentenceTokens)
		if !quality.Accepted {
			manifest.omitted = append(manifest.omitted, Omission{Language: entry.Language, CanonicalLemma: entry.CanonicalLemma, UPOS: entry.UPOS, Score: quality.Score, Reasons: append([]string(nil), quality.Reasons...)})
			manifest.decisions = append(manifest.decisions, ManifestItem{Ordinal: ordinal, Disposition: ManifestQualityOmitted, CorpusID: entry.CorpusID, SentenceOrdinal: entry.SentenceOrdinal, Entry: decisionEntry, Quality: quality})
			continue
		}
		manifest.accepted = append(manifest.accepted, renderInputFromEntry(entry))
		manifest.decisions = append(manifest.decisions, ManifestItem{Ordinal: ordinal, Disposition: ManifestAccepted, CorpusID: entry.CorpusID, SentenceOrdinal: entry.SentenceOrdinal, Entry: decisionEntry, Quality: quality})
		manifest.enrichmentCandidates = append(manifest.enrichmentCandidates, enrichment.Candidate{
			Identity:                  enrichment.Identity{Language: entry.Language, CanonicalLemma: entry.CanonicalLemma, UPOS: entry.UPOS},
			TargetWord:                entry.TargetWord,
			ExampleSentence:           strings.TrimSpace(entry.Sentence),
			DictionaryProviderVersion: entry.DictionaryProviderVersion,
			CandidateSenses:           enrichment.CloneLexicalSenses(entry.CandidateSenses),
		})
	}
	return manifest
}

// EnrichmentCandidates returns the frozen candidates in final render order.
func (m manifest) enrichmentCandidatesProjection() []enrichment.Candidate {
	result := append([]enrichment.Candidate(nil), m.enrichmentCandidates...)
	for i := range result {
		result[i].CandidateSenses = enrichment.CloneLexicalSenses(result[i].CandidateSenses)
	}
	return result
}

// Completeness reports the manifest's pre-enrichment accepted and omitted
// counts. Final optional-field counts are calculated during the sole render.
func (m manifest) completeness() Completeness {
	result := Completeness{TotalCards: len(m.accepted), QualityOmitted: len(m.omitted)}
	for _, entry := range m.accepted {
		if strings.TrimSpace(entry.Translation) != "" {
			result.CardsWithEnglish++
		}
		if strings.TrimSpace(entry.SentenceTranslation) != "" {
			result.CardsWithEnglishSentence++
		}
		if entry.fallbackGlossApplied {
			result.CardsWithFallbackGloss++
		}
	}
	return result
}

// BindCacheKeys returns a new manifest bound to the exact provider/version and
// sentence identities that enrichment will use. Existing external fields are
// cleared so a legacy or mismatched cache row can never survive finalization.
func (m manifest) bindCacheKeys(keys []enrichment.CacheKey) (manifest, error) {
	if len(keys) != len(m.enrichmentCandidates) {
		return manifest{}, fmt.Errorf("%w: cache identity count does not match manifest candidates", ErrInvalidInput)
	}
	bound := m.clone()
	bound.cacheKeys = append([]enrichment.CacheKey(nil), keys...)
	acceptedIndex := 0
	for decisionIndex := range bound.decisions {
		if bound.decisions[decisionIndex].Disposition != ManifestAccepted {
			continue
		}
		key := bound.cacheKeys[acceptedIndex]
		i := acceptedIndex
		candidate := bound.enrichmentCandidates[i]
		if key.Language != candidate.Language || key.CanonicalLemma != candidate.CanonicalLemma || key.UPOS != strings.ToUpper(candidate.UPOS) || strings.TrimSpace(key.Provider) == "" || strings.TrimSpace(key.ProviderVersion) == "" {
			return manifest{}, fmt.Errorf("%w: cache identity does not match manifest candidate %d", ErrInvalidInput, i)
		}
		if key.DictionaryProviderVersion != candidate.DictionaryProviderVersion {
			return manifest{}, fmt.Errorf("%w: dictionary identity does not match manifest candidate %d", ErrInvalidInput, i)
		}
		exactSentenceHash := enrichment.SentenceHash(candidate.ExampleSentence)
		if key.SentenceHash != "" && key.SentenceHash != exactSentenceHash {
			return manifest{}, fmt.Errorf("%w: sentence cache identity does not match manifest candidate %d", ErrInvalidInput, i)
		}
		clearExternalRenderInputFields(&bound.accepted[i])
		bound.decisions[decisionIndex].CacheKey = &key
		acceptedIndex++
	}
	return bound, nil
}

func (m manifest) clone() manifest {
	m.accepted = cloneRenderInputs(m.accepted)
	m.omitted = append([]Omission(nil), m.omitted...)
	for i := range m.omitted {
		m.omitted[i].Reasons = append([]string(nil), m.omitted[i].Reasons...)
	}
	m.enrichmentCandidates = append([]enrichment.Candidate(nil), m.enrichmentCandidates...)
	for i := range m.enrichmentCandidates {
		m.enrichmentCandidates[i].CandidateSenses = enrichment.CloneLexicalSenses(m.enrichmentCandidates[i].CandidateSenses)
	}
	m.cacheKeys = append([]enrichment.CacheKey(nil), m.cacheKeys...)
	m.decisions = cloneManifestItems(m.decisions)
	return m
}

func renderManifest(ctx context.Context, manifest manifest, outcomes []ExactEnrichment) (Artifact, []string, error) {
	entries := cloneRenderInputs(manifest.accepted)
	omitted := append([]Omission(nil), manifest.omitted...)
	for i := range omitted {
		omitted[i].Reasons = append([]string(nil), omitted[i].Reasons...)
	}
	if len(outcomes) > 0 || len(manifest.cacheKeys) > 0 {
		if len(outcomes) != len(entries) || len(manifest.cacheKeys) != len(entries) {
			return Artifact{}, nil, fmt.Errorf("%w: exact enrichment count does not match manifest", ErrInvalidInput)
		}
		var diagnostics []string
		for i, outcome := range outcomes {
			if outcome.CacheKey != manifest.cacheKeys[i] {
				return Artifact{}, nil, fmt.Errorf("%w: exact enrichment cache identity mismatch at candidate %d", ErrInvalidInput, i)
			}
			codes, err := applyExactEnrichment(&entries[i], outcome)
			if err != nil {
				return Artifact{}, nil, fmt.Errorf("candidate %d: %w", i, err)
			}
			diagnostics = appendUniqueCodes(diagnostics, codes...)
		}
		artifact, err := renderAccepted(ctx, manifest.owner, manifest.deckName, entries, omitted)
		return artifact, diagnostics, err
	}
	artifact, err := renderAccepted(ctx, manifest.owner, manifest.deckName, entries, omitted)
	return artifact, nil, err
}

func applyExactEnrichment(input *RenderInput, outcome ExactEnrichment) ([]string, error) {
	input.fallbackGlossApplied = false
	result := outcome.Result
	fields := []struct {
		available  bool
		provenance enrichment.Provenance
	}{
		{result.Translation.Available, result.Translation.Provenance},
		{result.FallbackGloss.Available, result.FallbackGloss.Provenance},
		{result.SentenceTranslation.Available, result.SentenceTranslation.Provenance},
		{result.SentenceTranslationTarget.Available, result.SentenceTranslationTarget.Provenance},
		{result.SenseSelection.Available, result.SenseSelection.Provenance},
	}
	available := false
	for _, field := range fields {
		available = available || field.available
		if field.available && (field.provenance.Provider != outcome.CacheKey.Provider || field.provenance.ProviderVersion != outcome.CacheKey.ProviderVersion) {
			return nil, fmt.Errorf("%w: enrichment provenance does not match cache identity", ErrInvalidInput)
		}
	}
	candidate := result.Candidate
	if available || candidate.Language != "" || candidate.CanonicalLemma != "" || candidate.UPOS != "" || candidate.TargetWord != "" || candidate.ExampleSentence != "" || candidate.DictionaryProviderVersion != "" {
		if candidate.Language != outcome.CacheKey.Language || candidate.CanonicalLemma != outcome.CacheKey.CanonicalLemma || strings.ToUpper(candidate.UPOS) != outcome.CacheKey.UPOS || candidate.DictionaryProviderVersion != outcome.CacheKey.DictionaryProviderVersion || testedRenderTarget(*input) != testedRenderTarget(RenderInput{CanonicalLemma: candidate.CanonicalLemma, TargetWord: candidate.TargetWord}) {
			return nil, fmt.Errorf("%w: enrichment candidate does not match cache identity", ErrInvalidInput)
		}
		if outcome.CacheKey.SentenceHash != "" && enrichment.SentenceHash(candidate.ExampleSentence) != outcome.CacheKey.SentenceHash {
			return nil, fmt.Errorf("%w: enrichment sentence does not match cache identity", ErrInvalidInput)
		}
	}
	if result.Translation.Available {
		input.Translation = result.Translation.Value
	}
	if result.SentenceTranslation.Available {
		input.SentenceTranslation = result.SentenceTranslation.Value
	}
	if result.SentenceTranslationTarget.Available {
		input.SentenceTranslationTarget = result.SentenceTranslationTarget.Value
	}
	selectionValid := false
	selectionMalformed := false
	var diagnostics []string
	if result.SenseSelection.Available {
		selection := result.SenseSelection.Value
		if !enrichment.ValidateSenseSelection(selection, len(input.CandidateSenses)) {
			selectionMalformed = true
			diagnostics = append(diagnostics, DegradationInvalidSenseSelection)
		} else if len(selection) > 0 {
			selected := make([]enrichment.LexicalSense, 0, len(selection))
			for _, index := range selection {
				selected = append(selected, input.CandidateSenses[index])
			}
			if gloss := enrichment.RenderGloss(selected, enrichment.DefaultMaxSenses, enrichment.DefaultMaxTokens); gloss != "" {
				input.Gloss = gloss
				selectionValid = true
			}
		}
	}
	if result.FallbackGloss.Available && !selectionValid && !selectionMalformed {
		fallback := strings.TrimSpace(result.FallbackGloss.Value)
		if fallbackGlossEligible(fallback) {
			input.Gloss = fallback
			input.fallbackGlossApplied = true
			diagnostics = append(diagnostics, DegradationFallbackGlossApplied)
		}
	}
	if result.FallbackGloss.Available && strings.TrimSpace(result.FallbackGloss.Value) != "" && !fallbackGlossEligible(result.FallbackGloss.Value) {
		diagnostics = append(diagnostics, DegradationFallbackGlossRejected)
	}
	return appendUniqueCodes(nil, diagnostics...), nil
}

func targetWord(sentence string, candidate domain.SelectionCandidate) string {
	var forms []string
	if json.Unmarshal(candidate.ObservedForms, &forms) == nil {
		for _, form := range forms {
			cleanForm := textmatch.CleanLexicalSurface(form)
			if targetIndex(sentence, cleanForm) >= 0 {
				return cleanForm
			}
		}
	}
	return textmatch.CleanLexicalSurface(candidate.CanonicalLemma)
}

func candidateKey(candidate domain.SelectionCandidate) string {
	return candidate.Language + "\x00" + candidate.CanonicalLemma + "\x00" + candidate.UPOS
}

func renderAccepted(ctx context.Context, owner, deckName string, entries []RenderInput, omitted []Omission) (Artifact, error) {
	type acceptedNote struct {
		input RenderInput
		note  Note
	}
	accepted := make([]acceptedNote, 0, len(entries))
	enrichmentCandidates := make([]enrichment.Candidate, 0, len(entries))
	completeness := Completeness{QualityOmitted: len(omitted)}
	for _, input := range entries {
		n, err := makeNote(owner, input)
		if err != nil {
			return Artifact{}, fmt.Errorf("render %s/%s/%s: %w", input.Language, input.CanonicalLemma, input.UPOS, err)
		}
		accepted = append(accepted, acceptedNote{input: input, note: n})
		completeness.TotalCards++
		if strings.TrimSpace(input.Translation) != "" {
			completeness.CardsWithEnglish++
		}
		if strings.TrimSpace(input.SentenceTranslation) != "" {
			completeness.CardsWithEnglishSentence++
		}
		if input.fallbackGlossApplied {
			completeness.CardsWithFallbackGloss++
		}
		enrichmentCandidates = append(enrichmentCandidates, enrichment.Candidate{
			Identity:                  enrichment.Identity{Language: input.Language, CanonicalLemma: input.CanonicalLemma, UPOS: input.UPOS},
			TargetWord:                testedRenderTarget(input),
			ExampleSentence:           strings.TrimSpace(input.Sentence),
			DictionaryProviderVersion: input.DictionaryProviderVersion,
			CandidateSenses:           enrichment.CloneLexicalSenses(input.CandidateSenses),
		})
	}
	notes := make([]Note, len(accepted))
	for i := range accepted {
		notes[i] = accepted[i].note
	}
	tsv, err := RenderTSV(notes, dictionaryAttribution(entries))
	if err != nil {
		return Artifact{}, fmt.Errorf("render TSV: %w", err)
	}
	language := "und"
	if len(entries) > 0 && strings.TrimSpace(entries[0].Language) != "" {
		language = entries[0].Language
	}
	ankiDeckName := DeckName(language, deckName)
	apkg, err := renderAPKG(ankiDeckName, notes, deckDescription(entries))
	if err != nil {
		return Artifact{}, fmt.Errorf("render Anki package: %w", err)
	}
	generated := make([]GeneratedRecord, len(accepted))
	for i, item := range accepted {
		generated[i] = GeneratedRecord{Input: item.input, Note: item.note}
	}
	return Artifact{APKG: apkg, Filename: DownloadFilename(deckName), DeckName: ankiDeckName, TSV: tsv, Count: len(notes), Completeness: completeness, Omitted: omitted, EnrichmentCandidates: enrichmentCandidates, Generated: generated}, nil
}

func deckDescription(entries []RenderInput) string {
	description := "Generated by Mouseion"
	if attribution := dictionaryAttribution(entries); attribution != "" {
		return description + "\n\n" + attribution
	}
	return description
}

func dictionaryAttribution(entries []RenderInput) string {
	for _, entry := range entries {
		if strings.TrimSpace(entry.DictionaryProviderVersion) != "" {
			return dictionary.AttributionNotice
		}
	}
	return ""
}
