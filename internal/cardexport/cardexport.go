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
	"strconv"
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
	LexicalDiscriminator, Headword                                                    string
	CorpusID                                                                          string
	SentenceOrdinal                                                                   int64
	Sentence, Translation, SentenceTranslation, SentenceTranslationTarget, TargetWord string
	Gloss, Plural, IPA, PrincipalParts, DictionaryProviderVersion                     string
	OmittedEvidenceCount                                                              int
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
	quality := SentenceQuality{Reasons: make([]string, 0, 8)}
	scoreLengthQuality(&quality, text)
	scoreTargetQuality(&quality, text, target)
	scoreSourceLocationQuality(&quality, firstEncounter)
	scoreBoundaryQuality(&quality, language, text)
	scoreStructuralNoiseQuality(&quality, text)
	scoreGermanGDEXQuality(language, &quality, text, target, tokens)
	quality.Accepted = quality.Score >= minimumSentenceScore && !containsRejection(quality.Reasons)
	return quality
}

// award grants points and records the matching acceptance reason.
func (quality *SentenceQuality) award(points int, reason string) {
	quality.Score += points
	quality.Reasons = append(quality.Reasons, reason)
}

// reject records a rejection reason without changing the score.
func (quality *SentenceQuality) reject(reason string) {
	quality.Reasons = append(quality.Reasons, reason)
}

// scoreLengthQuality applies the length gate: a fragmented or over-long
// sentence is rejected, otherwise it earns the base score and a context bonus
// when its word count sits in the useful window.
func scoreLengthQuality(quality *SentenceQuality, text string) {
	words := strings.Fields(text)
	if !utf8.ValidString(text) || len(words) < minimumSentenceWords {
		quality.reject("too short or fragmented")
		return
	}
	if sentenceTooLong(text) {
		quality.reject("too long")
		return
	}
	quality.award(30, "usable length")
	if len(words) >= 8 && len(words) <= 30 {
		quality.award(10, "useful context window")
	}
}

// scoreTargetQuality requires the selected target to appear as a whole word.
func scoreTargetQuality(quality *SentenceQuality, text, target string) {
	if targetIndex(text, strings.TrimSpace(target)) < 0 {
		quality.reject("target not present as a word")
		return
	}
	quality.award(25, "target present")
}

// scoreSourceLocationQuality requires a valid first-encounter offset.
func scoreSourceLocationQuality(quality *SentenceQuality, firstEncounter int64) {
	if firstEncounter < 0 || firstEncounter == math.MaxInt64 {
		quality.reject("invalid source location")
		return
	}
	quality.award(15, "valid source location")
}

// scoreBoundaryQuality requires a terminal sentence boundary and, for
// languages that capitalise sentences, a capitalised or quoted start.
func scoreBoundaryQuality(quality *SentenceQuality, language, text string) {
	if !hasCompleteBoundary(language, text) {
		quality.reject("incomplete sentence boundaries")
		return
	}
	quality.award(20, "complete sentence boundaries")
}

// scoreStructuralNoiseQuality rejects headings, boilerplate and extraction
// anomalies that pass the length gate.
func scoreStructuralNoiseQuality(quality *SentenceQuality, text string) {
	if structuralNoise(text) {
		quality.reject("structural noise or boilerplate")
		return
	}
	quality.award(10, "no obvious structural noise")
}

// scoreGermanGDEXQuality overlays the German GDEX grader. It contributes the
// grader score and merges its reasons, and a rejected grade zeroes the score.
// Other languages, and calls without a dependency parse, keep only the text
// criteria scored above.
func scoreGermanGDEXQuality(language string, quality *SentenceQuality, text, target string, tokens []analyzer.Token) {
	if tokens == nil || !isGerman(language) {
		return
	}
	gdexQuality := gdex.ScoreSentenceQuality(analyzer.Sentence{Text: text, Tokens: tokens}, gdexTargetIndices(target, tokens))
	quality.GDEXScore = gdexQuality.Score
	for _, reason := range gdexQuality.Reasons {
		if !slices.Contains(quality.Reasons, reason) {
			quality.Reasons = append(quality.Reasons, reason)
		}
	}
	if !gdexQuality.Accepted {
		quality.Score = 0
		if !slices.Contains(quality.Reasons, reasonNoFiniteVerbAndSubject) {
			quality.Reasons = append(quality.Reasons, reasonNoFiniteVerbAndSubject)
		}
	}
}

// gdexTargetIndices reports the token ordinals whose lexical surface matches the
// selected target.
func gdexTargetIndices(target string, tokens []analyzer.Token) []int {
	targetIndices := make([]int, 0, 1)
	cleanTarget := textmatch.CleanLexicalSurface(target)
	for index, token := range tokens {
		if cleanTarget != "" && strings.EqualFold(textmatch.CleanLexicalSurface(token.Surface), cleanTarget) {
			targetIndices = append(targetIndices, index)
		}
	}
	return targetIndices
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

func hasCompleteBoundary(language, text string) bool {
	if text == "" {
		return false
	}
	if IsAncientGreek(language) {
		// Greek editions start sentences in lowercase, so only the terminal
		// boundary is required. The source terminators are the full stop, the
		// semicolon / Greek question mark (U+037E) and the raised dot / ano
		// teleia (U+00B7 / U+0387).
		last, _ := utf8.DecodeLastRuneInString(text)
		return strings.ContainsRune(".;\u037e·\u0387", last)
	}
	if !strings.ContainsAny(text[len(text)-1:], ".!?") {
		return false
	}
	first, _ := utf8.DecodeRuneInString(text)
	return unicode.IsUpper(first) || unicode.IsNumber(first) || strings.ContainsRune("\"'“„«", first)
}

// IsAncientGreek reports whether a language tag is Ancient Greek, whose
// editions do not capitalise sentence starts.
func IsAncientGreek(language string) bool {
	return baseLanguageCode(language) == "grc"
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
		if strings.Contains(text, strconv.Itoa(year)) {
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

// rankedSentenceEvidence is one source reference paired with the evidence that
// ranks it.
type rankedSentenceEvidence struct {
	evidence SentenceEvidence
	sentence int
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
	ranked := rankSentenceEvidence(candidate.Language, refs, forms, persisted)
	if len(ranked) == 0 {
		return SentenceEvidence{}, false
	}
	sort.SliceStable(ranked, func(i, j int) bool { return betterSentenceEvidence(ranked[i], ranked[j]) })
	evidence := ranked[0].evidence
	evidence.CorpusID = candidate.CorpusID
	evidence.SentenceOrdinal = int64(ranked[0].sentence)
	return evidence, true
}

// rankSentenceEvidence scores every source reference the persisted corpus can
// resolve. Unresolvable references are dropped so they cannot displace the rest.
func rankSentenceEvidence(language string, refs []sentenceReference, forms []string, persisted map[int64]analyzer.Sentence) []rankedSentenceEvidence {
	ranked := make([]rankedSentenceEvidence, 0, len(refs))
	for _, ref := range refs {
		entry, ok := sentenceEvidenceForReference(language, ref, forms, persisted)
		if ok {
			ranked = append(ranked, entry)
		}
	}
	return ranked
}

// sentenceEvidenceForReference resolves one source reference to its text,
// target, quality and tokens. It reports false when a persisted run cannot
// resolve the reference's sentence.
func sentenceEvidenceForReference(language string, ref sentenceReference, forms []string, persisted map[int64]analyzer.Sentence) (rankedSentenceEvidence, bool) {
	text, tokens, ok := persistedSentence(persisted, ref)
	if !ok {
		return rankedSentenceEvidence{}, false
	}
	location, valid := referenceStartOffset(ref.Location)
	if !valid {
		location = -1
	}
	target := matchingSentenceTarget(text, forms)
	quality := scoreSentenceQuality(language, text, target, location, tokens)
	var evidenceTokens []analyzer.Token
	if persisted != nil {
		evidenceTokens = append([]analyzer.Token{}, tokens...)
	}
	return rankedSentenceEvidence{
		evidence: SentenceEvidence{Sentence: text, Target: target, FirstEncounter: location, Quality: quality, Tokens: evidenceTokens},
		sentence: ref.SentenceIndex,
	}, true
}

// persistedSentence resolves a reference against the persisted corpus. Without
// a persisted run it returns the reference's own text and no tokens; with one,
// an unresolvable reference reports false.
func persistedSentence(persisted map[int64]analyzer.Sentence, ref sentenceReference) (string, []analyzer.Token, bool) {
	if persisted == nil {
		return ref.Text, nil, true
	}
	sentence, ok := persisted[int64(ref.SentenceIndex)]
	if !ok {
		return "", nil, false
	}
	if sentence.Tokens == nil {
		return sentence.Text, []analyzer.Token{}, true
	}
	return sentence.Text, sentence.Tokens, true
}

// matchingSentenceTarget returns the first observed form that appears as a
// whole word in text, or the empty string when none does.
func matchingSentenceTarget(text string, forms []string) string {
	for _, form := range forms {
		cleanForm := textmatch.CleanLexicalSurface(form)
		if targetIndex(text, cleanForm) >= 0 {
			return cleanForm
		}
	}
	return ""
}

// betterSentenceEvidence orders accepted candidates first, then by descending
// GDEX score, ascending first encounter, sentence ordinal and sentence text.
func betterSentenceEvidence(a, b rankedSentenceEvidence) bool {
	switch {
	case a.evidence.Quality.Accepted != b.evidence.Quality.Accepted:
		return a.evidence.Quality.Accepted
	case a.evidence.Quality.GDEXScore != b.evidence.Quality.GDEXScore:
		return a.evidence.Quality.GDEXScore > b.evidence.Quality.GDEXScore
	case a.evidence.FirstEncounter != b.evidence.FirstEncounter:
		return a.evidence.FirstEncounter < b.evidence.FirstEncounter
	case a.sentence != b.sentence:
		return a.sentence < b.sentence
	default:
		return a.evidence.Sentence < b.evidence.Sentence
	}
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
	baseLanguage := baseLanguageCode(language)
	if !strings.EqualFold(strings.TrimSpace(upos), "NOUN") {
		return ""
	}
	variants, ok := morphologyVariants(morphology)
	if !ok {
		return ""
	}
	table, ok := nounArticleForms(baseLanguage)
	if !ok || len(variants) == 0 {
		return ""
	}
	if article, ok := explicitNounArticle(variants, table.validArticles); ok {
		return article
	}
	return genderDerivedArticle(baseLanguage, table, variants, lemma)
}

// baseLanguageCode normalises a language tag to its lowercase base language.
func baseLanguageCode(language string) string {
	language = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(language), "_", "-"))
	base, _, _ := strings.Cut(language, "-")
	return base
}

// genderDerivedArticle resolves the article from the candidate's gender
// evidence when no dictionary article names an accepted form.
func genderDerivedArticle(baseLanguage string, table nounArticleTable, variants []map[string]string, lemma string) string {
	evidence, ok := collectGenderEvidence(variants, table.articles)
	if !ok {
		return ""
	}
	genders, ok := decisiveGenders(baseLanguage, evidence)
	if !ok {
		return ""
	}
	if len(genders) == 1 {
		for article := range genders {
			return singleGenderArticle(baseLanguage, article, lemma)
		}
	}
	if len(evidence.genders) == 0 && evidence.allPlural {
		return table.pluralArticle
	}
	return ""
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
// definite article from the lemma.
func singleGenderArticle(baseLanguage, article, lemma string) string {
	if baseLanguage == "it" {
		return italianDefiniteArticle(article, lemma)
	}
	return article
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
	if strings.TrimSpace(input.Headword) != "" {
		// An imported Book stores the reviewed citation form; it is the
		// learner-facing headword, never the internal profile key.
		displayLemma = input.Headword
	}
	article := nounArticle(input.Language, input.UPOS, displayLemma, input.Morphology)
	note := Note{
		Key: DedupKeyWithDiscriminator(input.Language, input.CanonicalLemma, input.UPOS, input.LexicalDiscriminator, owner), Identity: CardIdentity(owner, input),
		Text: front, Article: escapeField(article), Lemma: escapeField(displayLemma), POS: escapeField(input.UPOS),
		Plural:         escapeField(nounPlural(input.UPOS, input.Plural)),
		IPA:            escapeField(strings.TrimSpace(input.IPA)),
		PrincipalParts: escapeField(strings.TrimSpace(input.PrincipalParts)),
		Gloss:          escapeField(input.Gloss), English: escapeField(input.Translation),
		EnglishSentence: HighlightEnglishTargets(input.SentenceTranslation, input.SentenceTranslationTargets, input.SentenceTranslationTarget), BookTitle: escapeField(input.SourceDocument), Tags: tags,
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
	applyEnrichmentTranslations(input, result)
	if outcome.CacheKey.MeaningEvidenceHash != "" {
		return nil, applyContextualGloss(input, result)
	}
	return applySenseSelectionGloss(input, result), nil
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

func targetWord(sentence string, candidate domain.SelectionCandidate) string {
	var forms []string
	if json.Unmarshal(candidate.ObservedForms, &forms) == nil {
		if target := matchingSentenceTarget(sentence, forms); target != "" {
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
// require: Wiktionary whenever dictionary evidence was used, and the imported
// Ancient Greek source notices whenever the deck is Ancient Greek.
func attributionNotices(entries []RenderInput) string {
	var notices []string
	if dictionaryAttribution(entries) != "" {
		notices = append(notices, dictionary.AttributionNotice)
	}
	if deckIsAncientGreek(entries) {
		notices = append(notices, dictionary.AncientGreekSourceNotice)
	}
	return strings.Join(notices, "\n\n")
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
