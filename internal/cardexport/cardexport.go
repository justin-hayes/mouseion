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
	"slices"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/dictionary"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/lemmadisplay"
	"github.com/justin-hayes/mouseion/internal/sentencequality"
	"github.com/justin-hayes/mouseion/internal/textmatch"
)

var ErrInvalidInput = errors.New("cardexport: invalid input")

type Entry struct {
	OwnerID, Language, CanonicalLemma, UPOS                                           string
	LexicalDiscriminator, Headword                                                    string
	CorpusID                                                                          string
	SentenceOrdinal                                                                   int64
	Sentence, Translation, SentenceTranslation, SentenceTranslationTarget, TargetWord string
	Gloss, Plural, IPA, PrincipalParts, DictionaryProviderVersion                     string
	PassageContext, PassageCitation, BrownsonPin                                      string
	// DictionaryEntry is the rendered hierarchical dictionary-entry display for
	// the matched source entry (currently the LSJ entry), DictionaryEntryID is
	// that source's native entry identity, and DictionaryPin is the pinned
	// revision it was derived from. All three are frozen on the deck item.
	DictionaryEntry, DictionaryEntryID, DictionaryPin string
	OmittedEvidenceCount                              int
	Morphology, SourceDocument, Notes                 string
	// Attribution is the optional typed structural attribution derived from
	// the exact source snapshot during preparation: a displayed speaker name or
	// stage-direction status. It is empty when the snapshot recorded no
	// attribution, including every historical manifest (ADR 0103).
	Attribution          domain.SourceAttribution
	CandidateSenses      []enrichment.LexicalSense
	SentenceTokens       []analyzer.Token
	SentenceStartOffset  uint64
	FirstEncounter       int64
	fallbackGlossApplied bool
	// LSJSummaryProviderVersion is the provider version of the LSJ sense
	// summary the entry's pinned LSJ entry may be condensed by. It is frozen at
	// freeze time and is empty when the summary is not offered, so no summary is
	// requested or shown (ADR 0101).
	LSJSummaryProviderVersion string
	// ProviderProposed marks a render entry built by the Ancient Greek
	// evidence-assisted pass. Its contextual Gloss, sentence translation and
	// English target alignment are provider-proposed and labelled as such; the
	// pass only adds to the source-evidence card (ADR 0101).
	ProviderProposed bool
}

type Note struct {
	Key, Identity, Text, Article, Lemma, Gender, Plural, IPA, PrincipalParts, POS, Gloss, English, EnglishSentence, BookTitle string
	PassageLabel, PassageText                                                                                                 string
	DictionaryEntry                                                                                                           string
	// LSJSenseSummary is Mouseion-composed HTML for a provider-written LSJ sense
	// summary, shown above the full LSJ entry it condenses and never replacing
	// it. Its text pieces are already escaped.
	LSJSenseSummary string
	// Attribution is the frozen, escaped speaker name or stage-direction label
	// shown separately on the card front. It is empty when the preparation
	// recorded no attribution (ADR 0103).
	Attribution string
	BackExtra   string
	Tags        []string
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

// SentenceQuality is sentencequality.SentenceQuality. It stays a cardexport
// type alias so the frozen manifest, persistence and presentation keep their
// existing types while the gate itself lives in the shared module (ADR 0106 §2).
type SentenceQuality = sentencequality.SentenceQuality

type lexicalResolver struct {
	lexical enrichment.LexicalProvider
}

func DedupKey(language, lemma, upos, owner string) string {
	sum := sha256.Sum256([]byte(language + " | " + lemma + " | " + upos + " | " + owner))
	return hex.EncodeToString(sum[:])
}

// DedupKeyWithDiscriminator includes the scheme-qualified lexical discriminator
// so homographs that share a canonical lemma and part of speech (δέω 1 and
// δέω 2) stay separate cards. The empty discriminator reproduces DedupKey
// byte-for-byte, so modern-language card keys are unchanged.
func DedupKeyWithDiscriminator(language, lemma, upos, discriminator, owner string) string {
	if discriminator == "" {
		return DedupKey(language, lemma, upos, owner)
	}
	sum := sha256.Sum256([]byte(language + " | " + lemma + " | " + upos + " | " + discriminator + " | " + owner))
	return hex.EncodeToString(sum[:])
}

// CardIdentity returns the stable, owner-scoped identity exported as Anki's
// sort and duplicate-detection field. It is deliberately separate from Key:
// Key preserves the existing lemma-level note GUID and persistence semantics,
// while Identity distinguishes the concrete target and source occurrence.
func CardIdentity(owner string, input RenderInput) string {
	target := testedRenderTarget(input)
	inputs := struct {
		Version              int    `json:"version"`
		Owner                string `json:"owner"`
		Language             string `json:"language"`
		CanonicalLemma       string `json:"canonical_lemma"`
		UPOS                 string `json:"upos"`
		LexicalDiscriminator string `json:"lexical_discriminator,omitempty"`
		Target               string `json:"target"`
		SourceSentence       string `json:"source_sentence"`
		SourceDocument       string `json:"source_document"`
		FirstEncounter       int64  `json:"first_encounter"`
	}{
		Version: 1, Owner: owner, Language: input.Language,
		CanonicalLemma: input.CanonicalLemma, UPOS: input.UPOS, LexicalDiscriminator: input.LexicalDiscriminator,
		Target:         target,
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
	return boldTargets(sentence, []string{target}, nil, 0)
}

// boldTargets renders sentence with each target bolded at its first
// case-insensitive occurrence. Targets that do not occur in the sentence, or
// that overlap an earlier matched span, are ignored. At least one target must
// match or ErrInvalidInput is returned. The persisted tokens and sentence start
// offset select the fallback anchor for a target no word-bounded match reaches.
func boldTargets(sentence string, targets []string, tokens []analyzer.Token, sentenceStartOffset uint64) (string, error) {
	spans, err := boldTargetSpans(sentence, targets, tokens, sentenceStartOffset)
	if err != nil {
		return "", err
	}
	var out strings.Builder
	last := 0
	for _, s := range spans {
		out.WriteString(escapeField(sentence[last:s.start]))
		out.WriteString("<b>")
		out.WriteString(escapeField(sentence[s.start:s.end]))
		out.WriteString("</b>")
		last = s.end
	}
	out.WriteString(escapeField(sentence[last:]))
	return out.String(), nil
}

// boldTargetSpans resolves the non-overlapping bold ranges for a sentence. It
// is shared by the prose renderer and the source-line renderer so both anchor
// the same target and particle evidence (ADR 0102).
func boldTargetSpans(sentence string, targets []string, tokens []analyzer.Token, sentenceStartOffset uint64) ([]targetSpan, error) {
	if strings.TrimSpace(sentence) == "" {
		return nil, ErrInvalidInput
	}
	spans := make([]targetSpan, 0, len(targets))
	for _, target := range targets {
		target = textmatch.CleanLexicalSurface(target)
		if target == "" {
			continue
		}
		start, end, ok := sentencequality.TargetSpan(sentence, target, tokens, sentenceStartOffset)
		if !ok {
			continue
		}
		spans = append(spans, targetSpan{start: start, end: end})
	}
	if len(spans) == 0 {
		return nil, fmt.Errorf("%w: target not found in sentence", ErrInvalidInput)
	}
	sort.Slice(spans, func(i, j int) bool { return spans[i].start < spans[j].start })
	nonOverlapping := spans[:0]
	last := 0
	for _, s := range spans {
		if s.start < last {
			continue
		}
		nonOverlapping = append(nonOverlapping, s)
		last = s.end
	}
	return nonOverlapping, nil
}

type targetSpan struct{ start, end int }

// boldSourceLines renders a multi-line source sentence as one explicit HTML
// block per source line. Escaping happens per segment before Mouseion's own
// markup is added, and a bold range that crosses a line break is reopened on
// the next line so emphasis never escapes its source line.
func boldSourceLines(sentence string, targets []string, tokens []analyzer.Token, sentenceStartOffset uint64) (string, error) {
	spans, err := boldTargetSpans(sentence, targets, tokens, sentenceStartOffset)
	if err != nil {
		return "", err
	}
	var out strings.Builder
	lineStart := 0
	for line := range strings.SplitSeq(sentence, "\n") {
		out.WriteString(`<span class="source-line">`)
		writeSourceLine(&out, line, lineStart, spans)
		out.WriteString(`</span>`)
		lineStart += len(line) + 1
	}
	return out.String(), nil
}

// writeSourceLine escapes one source line and adds its bold ranges.
func writeSourceLine(out *strings.Builder, line string, lineStart int, spans []targetSpan) {
	cursor := 0
	lineEnd := lineStart + len(line)
	for _, s := range spans {
		if s.end <= lineStart || s.start >= lineEnd {
			continue
		}
		start, end := max(s.start, lineStart)-lineStart, min(s.end, lineEnd)-lineStart
		if start < cursor {
			continue
		}
		out.WriteString(escapeField(line[cursor:start]))
		out.WriteString("<b>")
		out.WriteString(escapeField(line[start:end]))
		out.WriteString("</b>")
		cursor = end
	}
	out.WriteString(escapeField(line[cursor:]))
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
	wordCount := lexicalWordCount(translation)
	if wordCount > 1 && lexicalWordCount(translation[start:end]) == wordCount {
		return escapeField(translation)
	}
	return escapeField(translation[:start]) + "<b>" + escapeField(translation[start:end]) + "</b>" + escapeField(translation[end:])
}

// HighlightEnglishTargets renders validated structured excerpts. Historical
// cached single-phrase targets retain their legacy presentation.
func HighlightEnglishTargets(translation string, targets []string, legacyTarget string) string {
	validated := enrichment.ValidateSentenceTranslationTargets(translation, targets)
	if len(validated) == 0 {
		return HighlightEnglishTarget(translation, legacyTarget)
	}
	var output strings.Builder
	cursor := 0
	for _, target := range validated {
		start, end, ok := uniqueExactTargetMatch(translation, target)
		if !ok || start < cursor {
			return escapeField(translation)
		}
		output.WriteString(escapeField(translation[cursor:start]))
		output.WriteString("<b>")
		output.WriteString(escapeField(translation[start:end]))
		output.WriteString("</b>")
		cursor = end
	}
	output.WriteString(escapeField(translation[cursor:]))
	return output.String()
}

func lexicalWordCount(text string) int {
	count := 0
	inWord := false
	for _, r := range text {
		isWord := isWordRune(r)
		if isWord && !inWord {
			count++
		}
		inWord = isWord
	}
	return count
}

func uniqueTargetMatch(text, target string) (int, int, bool) {
	return uniqueTargetMatchUsing(text, target, strings.EqualFold)
}

func uniqueExactTargetMatch(text, target string) (int, int, bool) {
	return uniqueTargetMatchUsing(text, target, func(text, target string) bool { return text == target })
}

func uniqueTargetMatchUsing(text, target string, matches func(string, string) bool) (int, int, bool) {
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
		if !matches(text[offset:matchEnd], target) {
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
		if isWordRune(r) {
			return true
		}
	}
	return false
}

func isWordRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsNumber(r)
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

// IsAncientGreek reports whether a language tag is Ancient Greek, whose
// editions do not capitalise sentence starts.
func IsAncientGreek(language string) bool {
	return sentencequality.IsAncientGreek(language)
}

func nounArticle(language, upos, lemma, morphology string) (article, genderLabel string) {
	baseLanguage := baseLanguageCode(language)
	if !strings.EqualFold(strings.TrimSpace(upos), "NOUN") {
		return "", ""
	}
	variants, ok := morphologyVariants(morphology)
	if !ok {
		return "", ""
	}
	table, ok := nounArticleForms(baseLanguage)
	if !ok || len(variants) == 0 {
		return "", ""
	}
	if article, ok := explicitNounArticle(variants, table.validArticles); ok {
		return article, ""
	}
	if baseLanguage == "fr" {
		return frenchNounArticle(table, variants, lemma)
	}
	return genderDerivedArticle(baseLanguage, table, variants, lemma)
}

// frenchNounArticle resolves the French definite article and its companion
// gender label from one supported gender and the headword's initial letter
// (ADR 0102). The elided article "l'" and any h-initial whose aspirated-h
// category is absent hide the gender, so those rows carry a gender label. A
// plural-only or otherwise undecided reading stays bare with no label.
func frenchNounArticle(table nounArticleTable, variants []map[string]string, lemma string) (string, string) {
	evidence, ok := collectGenderEvidence(variants, table.articles)
	// A conflict, an unknown gender or a plural-only reading leaves the article
	// undecided, so the headword stays bare with no label.
	if !ok || evidence.invalidGender || evidence.allPlural {
		return "", ""
	}
	if len(evidence.genders) != 1 {
		return "", ""
	}
	return frenchNounPresentation(soleGender(evidence.genders), lemma, morphologyHStatus(variants))
}

// soleGender returns the only key of a gender set. Callers guarantee the set
// holds exactly one entry.
func soleGender(genders map[string]bool) string {
	for gender := range genders {
		return gender
	}
	return ""
}

// frenchNounPresentation renders one supported gender's French headword prefix
// and gender label. Vowel-initial nouns take the elided "l'" and an h-initial
// noun takes "le"/"la" only when the dictionary marks it aspirated; otherwise
// the headword is bare. A leading "y" is treated as uncertain. The gender label
// appears only where the article does not already reveal the gender.
func frenchNounPresentation(gender, lemma, hStatus string) (string, string) {
	first, _ := utf8.DecodeRuneInString(strings.ToLower(strings.TrimSpace(lemma)))
	switch {
	case first == utf8.RuneError:
		return "", ""
	case first == 'h':
		if hStatus == frenchAspiratedHStatus {
			return frenchDefiniteArticle(gender), ""
		}
		return "", frenchGenderLabel(gender)
	case strings.ContainsRune(frenchElisionVowels, first):
		return "l'", frenchGenderLabel(gender)
	case unicode.IsLetter(first) && first != 'y':
		return frenchDefiniteArticle(gender), ""
	default:
		return "", frenchGenderLabel(gender)
	}
}

// frenchElisionVowels are the initial letters that always elide in French: the
// plain and accented vowels plus the æ and œ ligatures.
const frenchElisionVowels = "aeiouàâäæèéêëîïôöœùûü"

// frenchAspiratedHStatus is the derivation's evidence value for a noun
// Wiktionary marks in "French terms with aspirated h".
const frenchAspiratedHStatus = "aspirated"

// frenchDefiniteArticle renders the definite singular article for one supported
// gender. An unknown gender yields no article.
func frenchDefiniteArticle(gender string) string {
	switch gender {
	case "masc":
		return "le"
	case "fem":
		return "la"
	default:
		return ""
	}
}

// frenchGenderLabel renders the compact gender cue a French card shows when its
// article cannot reveal the gender.
func frenchGenderLabel(gender string) string {
	switch gender {
	case "masc":
		return "m."
	case "fem":
		return "f."
	default:
		return ""
	}
}

// morphologyHStatus returns the first aspirated-h evidence among a candidate's
// morphology variants, or the empty string when none supplies it.
func morphologyHStatus(variants []map[string]string) string {
	for _, variant := range variants {
		if status := morphologyValue(variant, "HStatus"); status != "" {
			return status
		}
	}
	return ""
}

// baseLanguageCode normalises a language tag to its lowercase base language.
func baseLanguageCode(language string) string {
	language = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(language), "_", "-"))
	base, _, _ := strings.Cut(language, "-")
	return base
}

// genderDerivedArticle resolves the article from the candidate's gender
// evidence when no dictionary article names an accepted form.
func genderDerivedArticle(baseLanguage string, table nounArticleTable, variants []map[string]string, lemma string) (string, string) {
	evidence, ok := collectGenderEvidence(variants, table.articles)
	if !ok {
		return "", ""
	}
	genders, ok := decisiveGenders(baseLanguage, evidence)
	if !ok {
		return "", ""
	}
	if len(genders) == 1 {
		return singleGenderArticle(baseLanguage, soleGender(genders), lemma)
	}
	if len(evidence.genders) == 0 && evidence.allPlural {
		return table.pluralArticle, ""
	}
	return "", ""
}

// decisiveGenders selects the gender set that decides the article. German
// requires one unambiguous singular gender whenever any singular reading
// exists; an unrecognised gender is fatal unless a German singular reading
// rescues it.
func decisiveGenders(baseLanguage string, evidence genderEvidence) (map[string]bool, bool) {
	if evidence.invalidGender && (baseLanguage != "de" || len(evidence.singularGenders) == 0) {
		return nil, false
	}
	if baseLanguage == "de" && len(evidence.singularGenders) > 0 {
		if len(evidence.singularGenders) != 1 {
			return nil, false
		}
		return evidence.singularGenders, true
	}
	return evidence.genders, true
}

// nounArticleTable is one supported language's article rules: the gender to
// article output, the dictionary article forms accepted verbatim, and the
// plural article.
type nounArticleTable struct {
	articles      map[string]string
	validArticles map[string]bool
	pluralArticle string
}

// nounArticleForms returns the article rules for a supported base language.
// Unsupported languages report false.
func nounArticleForms(baseLanguage string) (nounArticleTable, bool) {
	switch baseLanguage {
	case "de":
		return nounArticleTable{
			articles:      map[string]string{"masc": "der", "masculine": "der", "fem": "die", "feminine": "die", "neut": "das", "neuter": "das"},
			validArticles: map[string]bool{"der": true, "die": true, "das": true},
			pluralArticle: "die",
		}, true
	case "it":
		return nounArticleTable{
			articles:      map[string]string{"masc": "masc", "masculine": "masc", "fem": "fem", "feminine": "fem"},
			validArticles: map[string]bool{"il": true, "lo": true, "la": true, "l'": true},
		}, true
	case "el":
		return nounArticleTable{
			articles:      map[string]string{"masc": "ο", "masculine": "ο", "fem": "η", "feminine": "η", "neut": "το", "neuter": "το"},
			validArticles: map[string]bool{"ο": true, "η": true, "το": true},
		}, true
	case "fr":
		// A French article is never stored: it depends on the headword's initial
		// letter and its aspirated-h evidence. The table maps the supported
		// gender to a marker that frenchNounPresentation turns into the article
		// and its gender label (ADR 0102).
		return nounArticleTable{
			articles: map[string]string{"masc": "masc", "masculine": "masc", "fem": "fem", "feminine": "fem"},
		}, true
	default:
		return nounArticleTable{}, false
	}
}

// explicitNounArticle returns the first dictionary article that names an
// accepted article form. A dictionary article is more reliable than the
// declined gender, so it wins before any gender reasoning.
func explicitNounArticle(variants []map[string]string, validArticles map[string]bool) (string, bool) {
	for _, variant := range variants {
		if article := morphologyValue(variant, "Article"); article != "" {
			if validArticles[article] {
				return article, true
			}
		}
	}
	return "", false
}

// genderEvidence is the accumulated gender across a candidate's morphology
// variants: every gendered article, the singular subset, whether an
// unrecognised gender appeared, and whether every variant was plural.
type genderEvidence struct {
	genders         map[string]bool
	singularGenders map[string]bool
	invalidGender   bool
	allPlural       bool
}

// collectGenderEvidence folds the morphology variants into one gender reading.
// An unrecognised singular gender is immediately fatal and reports false; an
// unrecognised plural gender is recorded so the caller can apply the
// language-specific fallback.
func collectGenderEvidence(variants []map[string]string, articles map[string]string) (genderEvidence, bool) {
	evidence := newGenderEvidence(articles)
	for _, variant := range variants {
		number := morphologyValue(variant, "Number")
		if !evidence.addGender(morphologyValue(variant, "Gender"), isSingularNumber(number), articles) {
			return genderEvidence{}, false
		}
		evidence.allPlural = evidence.allPlural && isPluralNumber(number)
	}
	return evidence, true
}

func newGenderEvidence(articles map[string]string) genderEvidence {
	return genderEvidence{
		genders:         make(map[string]bool, len(articles)),
		singularGenders: make(map[string]bool, len(articles)),
		allPlural:       true,
	}
}

// addGender folds one variant's gender tag into the evidence. An unrecognised
// singular gender is immediately fatal and reports false; an unrecognised
// plural gender is recorded for the caller's language-specific fallback.
func (evidence *genderEvidence) addGender(gender string, singular bool, articles map[string]string) bool {
	if gender == "" {
		return true
	}
	article, ok := articles[gender]
	if !ok {
		evidence.invalidGender = true
		return !singular
	}
	evidence.genders[article] = true
	if singular {
		evidence.singularGenders[article] = true
	}
	return true
}

func isSingularNumber(number string) bool { return number == "sing" || number == "singular" }

func isPluralNumber(number string) bool { return number == "plur" || number == "plural" }

// singleGenderArticle renders one resolved gender, resolving the Italian
// definite article from the lemma. Only German, Italian and Greek reach it;
// French is resolved by frenchNounArticle.
func singleGenderArticle(baseLanguage, article, lemma string) (string, string) {
	if baseLanguage == "it" {
		return italianDefiniteArticle(article, lemma), ""
	}
	return article, ""
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

// renderFront chooses the shipped prose rendering or the explicit source-line
// rendering a current manifest freezes. Historical manifests keep the prose
// rendering even when their sentence contains an authorial break.
func renderFront(input RenderInput, target string) (string, error) {
	if input.sourceLinePresentation && strings.Contains(input.Sentence, "\n") {
		return boldSourceLines(input.Sentence, targetSurfaces(target, input.SentenceTokens), input.SentenceTokens, input.SentenceStartOffset)
	}
	return boldTargets(input.Sentence, targetSurfaces(target, input.SentenceTokens), input.SentenceTokens, input.SentenceStartOffset)
}

// attributionField renders the frozen typed attribution as the escaped Anki
// field value. A stage direction is labelled generically; a speaker shows only
// the name, because nested directions are already omitted at freeze time.
func attributionField(attribution domain.SourceAttribution) string {
	switch attribution.Kind {
	case domain.AttributionSpeaker:
		return escapeField(strings.TrimSpace(attribution.Speaker))
	case domain.AttributionStageDirection:
		return escapeField(stageDirectionLabel)
	default:
		return ""
	}
}

const stageDirectionLabel = "Stage direction"

func makeNote(owner string, input RenderInput) (Note, error) {
	target := testedRenderTarget(input)
	front, err := renderFront(input, target)
	if err != nil {
		return Note{}, err
	}
	tags := uniqueTags("Mouseion", prefixedTag("lang", input.Language), prefixedTag("pos", input.UPOS), prefixedTag("source", input.SourceDocument))
	displayLemma := lemmadisplay.Format(input.Language, input.CanonicalLemma, input.UPOS)
	if strings.TrimSpace(input.Headword) != "" {
		// An imported Book stores the reviewed citation form; it is the
		// learner-facing headword, never the internal profile key.
		displayLemma = input.Headword
	}
	article, genderLabel := nounArticle(input.Language, input.UPOS, displayLemma, input.Morphology)
	note := Note{
		Key: DedupKeyWithDiscriminator(input.Language, input.CanonicalLemma, input.UPOS, input.LexicalDiscriminator, owner), Identity: CardIdentity(owner, input),
		Text: front, Article: escapeField(article), Lemma: escapeField(displayLemma), Gender: escapeField(genderLabel), POS: escapeField(input.UPOS),
		Plural:         escapeField(nounPlural(input.UPOS, input.Plural)),
		IPA:            escapeField(strings.TrimSpace(input.IPA)),
		PrincipalParts: escapeField(strings.TrimSpace(input.PrincipalParts)),
		Gloss:          escapeField(input.Gloss), English: escapeField(input.Translation),
		EnglishSentence: HighlightEnglishTargets(input.SentenceTranslation, input.SentenceTranslationTargets, input.SentenceTranslationTarget), BookTitle: escapeField(input.SourceDocument),
		// DictionaryEntry is Mouseion-composed HTML whose text pieces are already
		// escaped, so it is embedded as is rather than escaped again.
		DictionaryEntry: input.DictionaryEntry, Tags: tags,
		// LSJSenseSummary is Mouseion-composed HTML whose text pieces are already
		// escaped, so it is embedded as is rather than escaped again.
		LSJSenseSummary: input.LSJSenseSummary,
		Attribution:     attributionField(input.Attribution),
	}
	if strings.TrimSpace(input.PassageContext) != "" && strings.TrimSpace(input.PassageCitation) != "" {
		note.PassageLabel = escapeField(passageContextLabel(input.PassageCitation))
		note.PassageText = escapeField(input.PassageContext)
	}
	if input.ProviderProposed {
		// The pass only adds: every field it contributes is labelled, so a
		// learner never mistakes a provider-proposed meaning for source
		// evidence (ADR 0101).
		note.Gloss = labelledProviderProposed(note.Gloss)
		note.EnglishSentence = labelledProviderProposed(note.EnglishSentence)
	}
	articleLemma := note.Lemma
	if note.Article != "" {
		separator := " "
		if strings.HasSuffix(article, "'") {
			separator = ""
		}
		articleLemma = note.Article + separator + note.Lemma
	}
	if note.Gender != "" {
		articleLemma += " (" + note.Gender + ")"
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
	if note.PassageText != "" {
		back = append(back, note.PassageLabel, note.PassageText)
	}
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
	return []string{n.Identity, n.Text, n.Article, n.Lemma, n.Gender, n.Plural, n.IPA, n.PrincipalParts, n.POS, n.Gloss, n.English, n.EnglishSentence, n.BookTitle, n.PassageLabel, n.PassageText, n.DictionaryEntry, n.LSJSenseSummary, n.Attribution}
}

const noteTypeName = "Mouseion Vocab Recognition"

// providerProposedLabel prefixes each field the Ancient Greek evidence-assisted
// pass contributes, so a provider-proposed Gloss or sentence translation is
// never mistaken for source evidence (ADR 0101).
const providerProposedLabel = "Provider-proposed: "

// labelledProviderProposed prefixes a non-empty field the pass contributed.
func labelledProviderProposed(value string) string {
	if value == "" {
		return value
	}
	return escapeField(providerProposedLabel) + value
}

var fieldNames = []string{"Identity", "Text", "Article", "Lemma", "Gender", "Plural", "IPA", "PrincipalParts", "POS", "Gloss", "English", "EnglishSentence", "BookTitle", "PassageLabel", "PassageText", "DictionaryEntry", "LSJSenseSummary", "Attribution"}

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

// clearDictionaryFacts removes every local dictionary fact, both meaning
// evidence and dictionary-form evidence, from an entry built from source
// evidence alone.
func clearDictionaryFacts(entry *Entry) {
	entry.Gloss = ""
	entry.Plural = ""
	entry.IPA = ""
	entry.PrincipalParts = ""
	entry.DictionaryProviderVersion = ""
	entry.OmittedEvidenceCount = 0
	entry.CandidateSenses = nil
	entry.DictionaryEntry = ""
	entry.DictionaryEntryID = ""
	entry.DictionaryPin = ""
}

func (r *lexicalResolver) resolveLexicalEntry(ctx context.Context, entry *Entry) error {
	if r == nil || r.lexical == nil || entry == nil {
		return nil
	}
	entry.DictionaryProviderVersion = r.lexical.Version()
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
		entry.OmittedEvidenceCount = len(senses) - enrichment.DefaultMaxCandidateSenses + result.OmittedCandidateCount
		senses = senses[:enrichment.DefaultMaxCandidateSenses]
	} else {
		entry.OmittedEvidenceCount = result.OmittedCandidateCount
	}
	entry.CandidateSenses = enrichment.CloneLexicalSenses(senses)
	entry.Plural = plural
	entry.IPA = strings.TrimSpace(result.IPA)
	entry.PrincipalParts = strings.TrimSpace(result.PrincipalParts)
	hStatus := strings.TrimSpace(result.HStatus)
	if result.Gender == "" && result.Article == "" && plural == "" && hStatus == "" {
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
	if hStatus != "" {
		morphology["HStatus"] = hStatus
	}
	encoded, err := json.Marshal(morphology)
	if err != nil {
		return fmt.Errorf("encode dictionary morphology: %w", err)
	}
	entry.Morphology = string(encoded)
	return nil
}

func applySentenceDecision(entry *Entry, candidate domain.SelectionCandidate, persisted map[int64]analyzer.Sentence) {
	var evidence sentencequality.SentenceEvidence
	var ok bool
	if persisted != nil {
		evidence, ok = sentencequality.BestSentenceEvidenceFromCorpus(candidate, persisted)
	} else {
		evidence, ok = sentencequality.BestSentenceEvidence(candidate)
	}
	if ok {
		entry.CorpusID = evidence.CorpusID
		entry.Sentence = evidence.Sentence
		entry.TargetWord = evidence.Target
		entry.FirstEncounter = evidence.FirstEncounter
		entry.SentenceOrdinal = evidence.SentenceOrdinal
		entry.SentenceTokens = evidence.Tokens
		entry.SentenceStartOffset = evidence.SentenceStartOffset
	} else {
		entry.TargetWord = targetWord(entry.Sentence, candidate, nil, 0)
		if persisted != nil {
			// An unavailable persisted sentence is not eligible for syntax scoring.
			entry.SentenceTokens = []analyzer.Token{}
			entry.SentenceStartOffset = 0
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
		quality := sentencequality.ScoreSentenceQuality(entry.Language, entry.Sentence, entry.TargetWord, entry.FirstEncounter, entry.SentenceTokens, entry.SentenceStartOffset)
		if !quality.Accepted {
			manifest.omitted = append(manifest.omitted, Omission{Language: entry.Language, CanonicalLemma: entry.CanonicalLemma, UPOS: entry.UPOS, Score: quality.Score, Reasons: append([]string(nil), quality.Reasons...)})
			manifest.decisions = append(manifest.decisions, ManifestItem{Ordinal: ordinal, Disposition: ManifestQualityOmitted, CorpusID: entry.CorpusID, SentenceOrdinal: entry.SentenceOrdinal, Entry: decisionEntry, Quality: quality})
			continue
		}
		accepted := renderInputFromEntry(entry)
		// A manifest written by the current codec freezes the source-line
		// presentation contract; a historical manifest restores the prose one.
		accepted.sourceLinePresentation = true
		manifest.accepted = append(manifest.accepted, accepted)
		manifest.decisions = append(manifest.decisions, ManifestItem{Ordinal: ordinal, Disposition: ManifestAccepted, CorpusID: entry.CorpusID, SentenceOrdinal: entry.SentenceOrdinal, Entry: decisionEntry, Quality: quality})
		manifest.enrichmentCandidates = append(manifest.enrichmentCandidates, enrichment.Candidate{
			Language: entry.Language, CanonicalLemma: entry.CanonicalLemma, UPOS: entry.UPOS,
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
	available, err := validateEnrichmentProvenance(result, outcome.CacheKey)
	if err != nil {
		return nil, err
	}
	if err := validateEnrichmentCandidate(testedRenderTarget(*input), result, outcome.CacheKey, available); err != nil {
		return nil, err
	}
	if input.ProviderProposed {
		// The evidence-assisted pass only adds. A resolved contextual Gloss
		// brings its sentence translation and English target alignment; a
		// failed or abstaining answer leaves the source-evidence card as it was.
		applyEvidenceAssistedPass(input, result)
		return nil, nil
	}
	applyEnrichmentTranslations(input, result)
	if outcome.CacheKey.MeaningEvidenceHash != "" {
		return nil, applyContextualGloss(input, result)
	}
	return applySenseSelectionGloss(input, result), nil
}

// applyEvidenceAssistedPass overlays the provider-proposed contextual Gloss and
// sentence translation when the pass resolved a contextual Gloss. It never
// fails: the pass can only add to the source-evidence card, so an abstaining or
// failed answer is not an omission (ADR 0101). The lemma translation the shared
// contract carries is deliberately not applied, because the default Greek card
// shows no lemma English.
func applyEvidenceAssistedPass(input *RenderInput, result enrichment.Result) {
	gloss := strings.TrimSpace(result.FallbackGloss.Value)
	if !result.FallbackGloss.Available || !fallbackGlossEligible(gloss) {
		return
	}
	input.Gloss = gloss
	if result.SentenceTranslation.Available {
		input.SentenceTranslation = result.SentenceTranslation.Value
	}
	if result.SentenceTranslationTarget.Available {
		input.SentenceTranslationTarget = result.SentenceTranslationTarget.Value
	}
	if result.SentenceTranslationTargets.Available {
		input.SentenceTranslationTargets = enrichment.ValidateSentenceTranslationTargets(input.SentenceTranslation, result.SentenceTranslationTargets.Value)
	}
}

// validateEnrichmentProvenance rejects any available field whose provenance
// disagrees with the frozen cache identity, and reports whether any field is
// available at all.
func validateEnrichmentProvenance(result enrichment.Result, key enrichment.CacheKey) (bool, error) {
	fields := []struct {
		available  bool
		provenance enrichment.Provenance
	}{
		{result.Translation.Available, result.Translation.Provenance},
		{result.FallbackGloss.Available, result.FallbackGloss.Provenance},
		{result.SentenceTranslation.Available, result.SentenceTranslation.Provenance},
		{result.SentenceTranslationTarget.Available, result.SentenceTranslationTarget.Provenance},
		{result.SentenceTranslationTargets.Available, result.SentenceTranslationTargets.Provenance},
		{result.SenseSelection.Available, result.SenseSelection.Provenance},
	}
	available := false
	for _, field := range fields {
		available = available || field.available
		if field.available && (field.provenance.Provider != key.Provider || field.provenance.ProviderVersion != key.ProviderVersion) {
			return false, fmt.Errorf("%w: enrichment provenance does not match cache identity", ErrInvalidInput)
		}
	}
	return available, nil
}

// validateEnrichmentCandidate rejects a result whose candidate disagrees with
// the frozen cache identity. An entirely empty candidate with no available
// field selects nothing and is not checked. The sentence hash is checked only
// when the frozen identity carries one.
func validateEnrichmentCandidate(target string, result enrichment.Result, key enrichment.CacheKey, available bool) error {
	candidate := result.Candidate
	if !available && candidate.Language == "" && candidate.CanonicalLemma == "" && candidate.UPOS == "" && candidate.TargetWord == "" && candidate.ExampleSentence == "" && candidate.DictionaryProviderVersion == "" {
		return nil
	}
	if candidate.Language != key.Language || candidate.CanonicalLemma != key.CanonicalLemma || strings.ToUpper(candidate.UPOS) != key.UPOS || candidate.DictionaryProviderVersion != key.DictionaryProviderVersion || target != testedRenderTarget(RenderInput{CanonicalLemma: candidate.CanonicalLemma, TargetWord: candidate.TargetWord}) {
		return fmt.Errorf("%w: enrichment candidate does not match cache identity", ErrInvalidInput)
	}
	if key.SentenceHash != "" && enrichment.SentenceHash(candidate.ExampleSentence) != key.SentenceHash {
		return fmt.Errorf("%w: enrichment sentence does not match cache identity", ErrInvalidInput)
	}
	return nil
}

// applyEnrichmentTranslations overlays each available translation field. The
// structured targets are re-validated against the sentence translation.
func applyEnrichmentTranslations(input *RenderInput, result enrichment.Result) {
	if result.Translation.Available {
		input.Translation = result.Translation.Value
	}
	if result.SentenceTranslation.Available {
		input.SentenceTranslation = result.SentenceTranslation.Value
	}
	if result.SentenceTranslationTarget.Available {
		input.SentenceTranslationTarget = result.SentenceTranslationTarget.Value
	}
	if result.SentenceTranslationTargets.Available {
		input.SentenceTranslationTargets = enrichment.ValidateSentenceTranslationTargets(input.SentenceTranslation, result.SentenceTranslationTargets.Value)
	}
}

// applyContextualGloss applies the provider's contextual Gloss when the frozen
// identity carries meaning evidence. A contextual run must supply an eligible
// gloss or the whole enrichment is rejected.
func applyContextualGloss(input *RenderInput, result enrichment.Result) error {
	gloss := strings.TrimSpace(result.FallbackGloss.Value)
	if !result.FallbackGloss.Available || !fallbackGlossEligible(gloss) {
		return fmt.Errorf("%w: contextual Gloss is missing or invalid", ErrInvalidInput)
	}
	input.Gloss = gloss
	input.fallbackGlossApplied = true
	return nil
}

// applySenseSelectionGloss resolves the card Gloss from a validated sense
// selection, falling back to the provider's fallback gloss.
func applySenseSelectionGloss(input *RenderInput, result enrichment.Result) []string {
	choice := chooseSenseSelectionGloss(input, result)
	if choice.gloss != "" {
		input.Gloss = choice.gloss
	}
	if choice.fallbackApplied {
		input.fallbackGlossApplied = true
	}
	return appendUniqueCodes(nil, choice.diagnostics...)
}

// senseGlossChoice is the gloss resolved for one enrichment result and the
// degradation codes that explain it.
type senseGlossChoice struct {
	gloss           string
	fallbackApplied bool
	diagnostics     []string
}

// chooseSenseSelectionGloss chooses the gloss for one enrichment result without
// mutating the input. A malformed selection suppresses the fallback and records
// a degradation; an eligible fallback that is used or rejected is recorded the
// same way.
func chooseSenseSelectionGloss(input *RenderInput, result enrichment.Result) senseGlossChoice {
	var choice senseGlossChoice
	selectionValid := false
	selectionMalformed := false
	if result.SenseSelection.Available {
		if !enrichment.ValidateSenseSelection(result.SenseSelection.Value, len(input.CandidateSenses)) {
			selectionMalformed = true
			choice.diagnostics = append(choice.diagnostics, DegradationInvalidSenseSelection)
		} else {
			choice.gloss = selectedSenseGloss(input, result.SenseSelection.Value)
			selectionValid = choice.gloss != ""
		}
	}
	if result.FallbackGloss.Available {
		fallback := strings.TrimSpace(result.FallbackGloss.Value)
		switch {
		case !selectionValid && !selectionMalformed && fallbackGlossEligible(fallback):
			choice.gloss = fallback
			choice.fallbackApplied = true
			choice.diagnostics = append(choice.diagnostics, DegradationFallbackGlossApplied)
		case fallback != "" && !fallbackGlossEligible(fallback):
			choice.diagnostics = append(choice.diagnostics, DegradationFallbackGlossRejected)
		}
	}
	return choice
}

// selectedSenseGloss renders the gloss for a validated sense selection.
func selectedSenseGloss(input *RenderInput, selection []int) string {
	selected := make([]enrichment.LexicalSense, 0, len(selection))
	for _, index := range selection {
		selected = append(selected, input.CandidateSenses[index])
	}
	return enrichment.RenderGloss(selected, enrichment.DefaultMaxSenses, enrichment.DefaultMaxTokens)
}

func targetWord(sentence string, candidate domain.SelectionCandidate, tokens []analyzer.Token, sentenceStartOffset uint64) string {
	var forms []string
	if json.Unmarshal(candidate.ObservedForms, &forms) == nil {
		if target := sentencequality.MatchingSentenceTarget(sentence, forms, tokens, sentenceStartOffset); target != "" {
			return target
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
			Language: input.Language, CanonicalLemma: input.CanonicalLemma, UPOS: input.UPOS,
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
	tsv, err := RenderTSV(notes, attributionNotices(entries))
	if err != nil {
		return Artifact{}, fmt.Errorf("render TSV: %w", err)
	}
	language := "und"
	if len(entries) > 0 && strings.TrimSpace(entries[0].Language) != "" {
		language = entries[0].Language
	}
	ankiDeckName := DeckName(language, deckName)
	apkg, err := renderAPKG(ctx, ankiDeckName, notes, deckDescription(entries))
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
	if attribution := attributionNotices(entries); attribution != "" {
		return description + "\n\n" + attribution
	}
	return description
}

// attributionNotices returns every attribution and licence notice the entries
// require: Wiktionary whenever dictionary evidence was used, the LSJ/Perseus
// notice whenever a matched LSJ entry was frozen, the imported Ancient Greek
// source notices whenever the deck is Ancient Greek, and the Brownson notice
// whenever a passage was frozen.
func attributionNotices(entries []RenderInput) string {
	var notices []string
	if dictionaryAttribution(entries) != "" {
		notices = append(notices, dictionary.AttributionNotice)
	}
	if pin, ok := ancientGreekLSJPin(entries); ok {
		notices = append(notices, dictionary.AncientGreekLSJNotice(pin))
	}
	if deckIsAncientGreek(entries) {
		notices = append(notices, dictionary.AncientGreekSourceNotice)
	}
	if deckHasBrownsonPassage(entries) {
		notices = append(notices, dictionary.BrownsonSourceNotice)
	}
	return strings.Join(notices, "\n\n")
}

// deckHasBrownsonPassage reports whether any entry froze a Brownson passage.
func deckHasBrownsonPassage(entries []RenderInput) bool {
	for _, entry := range entries {
		if strings.TrimSpace(entry.BrownsonPin) != "" {
			return true
		}
	}
	return false
}

// ancientGreekLSJPin reports the pinned LSJ revision named by any entry that
// carries a matched LSJ dictionary entry.
func ancientGreekLSJPin(entries []RenderInput) (string, bool) {
	for _, entry := range entries {
		if strings.TrimSpace(entry.DictionaryEntryID) != "" {
			return strings.TrimSpace(entry.DictionaryPin), true
		}
	}
	return "", false
}

func dictionaryAttribution(entries []RenderInput) string {
	for _, entry := range entries {
		if strings.TrimSpace(entry.DictionaryProviderVersion) != "" && len(entry.CandidateSenses) > 0 {
			return dictionary.AttributionNotice
		}
	}
	return ""
}

// deckIsAncientGreek reports whether any entry belongs to the dedicated Ancient
// Greek language.
func deckIsAncientGreek(entries []RenderInput) bool {
	for _, entry := range entries {
		if IsAncientGreek(entry.Language) {
			return true
		}
	}
	return false
}
