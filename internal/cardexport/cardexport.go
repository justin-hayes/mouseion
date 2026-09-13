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
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/gdex"
	"github.com/justin-hayes/mouseion/internal/lemmadisplay"
	"github.com/justin-hayes/mouseion/internal/textmatch"
)

var ErrInvalidInput = errors.New("cardexport: invalid input")

type Entry struct {
	OwnerID, Language, CanonicalLemma, UPOS                                           string
	Sentence, Translation, SentenceTranslation, SentenceTranslationTarget, TargetWord string
	Morphology, SourceDocument, Notes                                                 string
	SentenceTokens                                                                    []analyzer.Token
	FirstEncounter                                                                    int64
}

type Note struct {
	Key, Identity, Text, Article, Lemma, POS, English, EnglishSentence, BookTitle string
	BackExtra                                                                     string
	Tags                                                                          []string
}

type Artifact struct {
	APKG                    []byte
	Filename, DeckName, TSV string
	Count                   int
	Completeness            Completeness
	Omitted                 []Omission
	EnrichmentCandidates    []enrichment.Candidate
	Generated               []GeneratedRecord
}

// Manifest freezes every selection and render decision made for one prepared
// deck before optional external enrichment starts. Its fields are private and
// all slice-returning methods copy their data so later learner state, database
// rows, or caller mutation cannot change the final artifact.
type Manifest struct {
	owner                string
	deckName             string
	accepted             []Entry
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
	Entry Entry
	Note  Note
}

// Completeness reports which optional enrichment fields were available for
// accepted cards. QualityOmitted counts selected candidates that were not
// exported because their best source sentence failed the quality gate.
type Completeness struct {
	TotalCards               int
	CardsWithEnglish         int
	CardsWithEnglishSentence int
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
	FirstEncounter   int64
	Quality          SentenceQuality
	Tokens           []analyzer.Token
}

type Store interface {
	ListSelectionCandidatesForBook(context.Context, string, string) ([]domain.SelectionCandidate, error)
	ListKnownVocabulary(context.Context, string, string) ([]domain.KnownVocabulary, error)
	ListGeneratedVocabulary(context.Context, string, string) ([]domain.GeneratedVocabulary, error)
	ListReservedVocabulary(context.Context, string, string) ([]domain.DeckPreparationVocabulary, error)
	ListUnattachedGeneratedVocabulary(context.Context, string, string) ([]domain.GeneratedVocabulary, error)
	GetCoverageEntryForBook(context.Context, string, string, domain.SelectionCandidate) (Entry, error)
	RecordGeneratedForBook(context.Context, string, string, string, Entry, Note) error
}

type analysisStore interface {
	GetSourceMaterial(context.Context, string, string) (domain.SourceMaterial, error)
	GetCorpusForAnalysis(context.Context, string, string) (domain.Corpus, error)
	ListSelectionCandidatesForCorpus(context.Context, string, string) ([]domain.SelectionCandidate, error)
	GetCoverageEntryForCorpus(context.Context, string, string, domain.SelectionCandidate) (Entry, error)
}

type corpusSentenceStore interface {
	ListCorpusSentences(context.Context, string, string, []int64) (map[int64]analyzer.Sentence, error)
}

// preparedEntryStore avoids the legacy cache lookup performed by the direct
// export path. Prepared decks receive enrichment only through exact results
// returned by the configured enrichment service.
type preparedEntryStore interface {
	GetPreparedCoverageEntryForBook(context.Context, string, string, domain.SelectionCandidate) (Entry, error)
	GetPreparedCoverageEntryForCorpus(context.Context, string, string, domain.SelectionCandidate) (Entry, error)
}

type Service struct{ store Store }

func NewService(store Store) *Service { return &Service{store: store} }

func DedupKey(language, lemma, upos, owner string) string {
	sum := sha256.Sum256([]byte(language + " | " + lemma + " | " + upos + " | " + owner))
	return hex.EncodeToString(sum[:])
}

// CardIdentity returns the stable, owner-scoped identity exported as Anki's
// sort and duplicate-detection field. It is deliberately separate from Key:
// Key preserves the existing lemma-level note GUID and persistence semantics,
// while Identity distinguishes the concrete target and source occurrence.
func CardIdentity(owner string, entry Entry) string {
	target := testedTarget(entry)
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
		Version: 1, Owner: owner, Language: entry.Language,
		CanonicalLemma: entry.CanonicalLemma, UPOS: entry.UPOS, Target: target,
		SourceSentence: entry.Sentence, SourceDocument: entry.SourceDocument,
		FirstEncounter: entry.FirstEncounter,
	}
	payload, _ := json.Marshal(inputs)
	sum := sha256.Sum256(append([]byte("mouseion-card-identity-v1\x00"), payload...))
	return hex.EncodeToString(sum[:])
}

func BoldTarget(sentence, target string) (string, error) {
	target = textmatch.CleanLexicalSurface(target)
	if target == "" || strings.TrimSpace(sentence) == "" {
		return "", ErrInvalidInput
	}
	start, end, ok := textmatch.FoldedWordSpan(sentence, target)
	if !ok {
		return "", fmt.Errorf("%w: target %q not found in sentence", ErrInvalidInput, target)
	}
	return escapeField(sentence[:start]) + "<b>" + escapeField(sentence[start:end]) + "</b>" + escapeField(sentence[end:]), nil
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
	_ = json.Unmarshal(candidate.ObservedForms, &forms)
	forms = append(forms, candidate.CanonicalLemma)
	ranked := make([]struct {
		evidence SentenceEvidence
		index    int
		sentence int
	}, 0, len(refs))
	for i, ref := range refs {
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
		ranked = append(ranked, struct {
			evidence SentenceEvidence
			index    int
			sentence int
		}{SentenceEvidence{Sentence: text, Target: target, FirstEncounter: location, Quality: quality, Tokens: evidenceTokens}, i, ref.SentenceIndex})
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
		if ranked[i].evidence.Quality.Score != ranked[j].evidence.Quality.Score {
			return ranked[i].evidence.Quality.Score > ranked[j].evidence.Quality.Score
		}
		if ranked[i].index != ranked[j].index {
			return ranked[i].index < ranked[j].index
		}
		if ranked[i].sentence != ranked[j].sentence {
			return ranked[i].sentence < ranked[j].sentence
		}
		return ranked[i].evidence.Sentence < ranked[j].evidence.Sentence
	})
	return ranked[0].evidence, true
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

func germanNounArticle(language, upos, morphology string) string {
	language = strings.ToLower(strings.ReplaceAll(strings.TrimSpace(language), "_", "-"))
	baseLanguage, _, _ := strings.Cut(language, "-")
	if baseLanguage != "de" || !strings.EqualFold(strings.TrimSpace(upos), "NOUN") {
		return ""
	}
	var variants []map[string]string
	if err := json.Unmarshal([]byte(morphology), &variants); err != nil {
		var single map[string]string
		if json.Unmarshal([]byte(morphology), &single) != nil || single == nil {
			return ""
		}
		variants = []map[string]string{single}
	}
	if len(variants) == 0 {
		return ""
	}
	genders := make(map[string]bool, 3)
	allPlural := true
	for _, variant := range variants {
		gender := morphologyValue(variant, "Gender")
		if gender != "" {
			switch gender {
			case "masc", "masculine":
				genders["der"] = true
			case "fem", "feminine":
				genders["die"] = true
			case "neut", "neuter":
				genders["das"] = true
			default:
				return ""
			}
		}
		number := morphologyValue(variant, "Number")
		allPlural = allPlural && (number == "plur" || number == "plural")
	}
	if len(genders) == 1 {
		for article := range genders {
			return article
		}
	}
	if len(genders) == 0 && allPlural {
		return "die"
	}
	return ""
}

func morphologyValue(morphology map[string]string, key string) string {
	for candidate, value := range morphology {
		if strings.EqualFold(strings.TrimSpace(candidate), key) {
			return strings.ToLower(strings.TrimSpace(value))
		}
	}
	return ""
}

func testedTarget(entry Entry) string {
	target := textmatch.CleanLexicalSurface(entry.TargetWord)
	if target == "" {
		target = textmatch.CleanLexicalSurface(entry.CanonicalLemma)
	}
	return target
}

func makeNote(owner string, entry Entry) (Note, error) {
	target := testedTarget(entry)
	front, err := BoldTarget(entry.Sentence, target)
	if err != nil {
		return Note{}, err
	}
	tags := uniqueTags("Mouseion", prefixedTag("lang", entry.Language), prefixedTag("pos", entry.UPOS), prefixedTag("source", entry.SourceDocument))
	displayLemma := lemmadisplay.Format(entry.Language, entry.CanonicalLemma, entry.UPOS)
	article := germanNounArticle(entry.Language, entry.UPOS, entry.Morphology)
	note := Note{
		Key: DedupKey(entry.Language, entry.CanonicalLemma, entry.UPOS, owner), Identity: CardIdentity(owner, entry),
		Text: front, Article: escapeField(article), Lemma: escapeField(displayLemma), POS: escapeField(entry.UPOS),
		English:         escapeField(entry.Translation),
		EnglishSentence: HighlightEnglishTarget(entry.SentenceTranslation, entry.SentenceTranslationTarget), BookTitle: escapeField(entry.SourceDocument), Tags: tags,
	}
	articleLemma := note.Lemma
	if note.Article != "" {
		articleLemma = note.Article + " " + note.Lemma
	}
	note.BackExtra = strings.Join([]string{articleLemma, note.POS, note.English, note.EnglishSentence}, "\n")
	return note, nil
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

func RenderTSV(notes []Note) (string, error) {
	var out bytes.Buffer
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
	return []string{n.Identity, n.Text, n.Article, n.Lemma, n.POS, n.English, n.EnglishSentence, n.BookTitle}
}

const noteTypeName = "Mouseion Vocab Recognition"

var fieldNames = []string{"Identity", "Text", "Article", "Lemma", "POS", "English", "EnglishSentence", "BookTitle"}

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

// ExportCoverage exports recurring unknown lemmas in reading order.
func (s *Service) ExportCoverage(ctx context.Context, owner, bookID string) (Artifact, error) {
	artifact, err := s.BuildCoverage(ctx, owner, bookID)
	if err != nil {
		return Artifact{}, err
	}
	for _, item := range artifact.Generated {
		if err := s.store.RecordGeneratedForBook(ctx, owner, bookID, item.Note.BookTitle, item.Entry, item.Note); err != nil {
			entry := item.Entry
			return Artifact{}, fmt.Errorf("record generated %s/%s/%s: %w", entry.Language, entry.CanonicalLemma, entry.UPOS, err)
		}
	}
	return artifact, nil
}

// BuildCoverage selects, quality-gates, and renders a deck without changing
// vocabulary or card state. Callers can persist Generated with the artifact.
func (s *Service) BuildCoverage(ctx context.Context, owner, bookID string) (Artifact, error) {
	if s == nil || s.store == nil || strings.TrimSpace(owner) == "" || strings.TrimSpace(bookID) == "" {
		return Artifact{}, ErrInvalidInput
	}
	candidates, err := s.store.ListSelectionCandidatesForBook(ctx, owner, bookID)
	if err != nil {
		return Artifact{}, fmt.Errorf("list selection candidates: %w", err)
	}
	selected, err := s.coverageCandidates(ctx, owner, bookID, candidates)
	if err != nil {
		return Artifact{}, err
	}
	persistedSentences, err := s.loadCorpusSentences(ctx, owner, selected)
	if err != nil {
		return Artifact{}, err
	}
	sort.SliceStable(selected, func(i, j int) bool {
		if selected[i].FirstEncounter != selected[j].FirstEncounter {
			return selected[i].FirstEncounter < selected[j].FirstEncounter
		}
		return candidateKey(selected[i]) < candidateKey(selected[j])
	})

	entries := make([]Entry, 0, len(selected))
	deckName := bookID
	for _, candidate := range selected {
		entry, err := s.store.GetCoverageEntryForBook(ctx, owner, bookID, candidate)
		if err != nil {
			return Artifact{}, fmt.Errorf("get coverage entry %s: %w", candidateKey(candidate), err)
		}
		applySentenceDecision(&entry, candidate, persistedSentences[candidate.CorpusID])
		entries = append(entries, entry)
		if strings.TrimSpace(entry.SourceDocument) != "" {
			deckName = entry.SourceDocument
		}
	}
	return s.render(ctx, owner, deckName, entries)
}

// BuildCoverageForAnalysis selects and renders only the immutable corpus
// produced by the completed scoped analysis. The analysis provenance is
// resolved by the owner-scoped persistence implementation, rather than by a
// mutable book-level "latest" projection.
func (s *Service) BuildCoverageForAnalysis(ctx context.Context, owner, analysisRunID string) (Artifact, error) {
	if s == nil || s.store == nil || strings.TrimSpace(owner) == "" || strings.TrimSpace(analysisRunID) == "" {
		return Artifact{}, ErrInvalidInput
	}
	store, ok := s.store.(analysisStore)
	if !ok {
		return Artifact{}, errors.New("cardexport: scoped analysis storage is unavailable")
	}
	corpus, err := store.GetCorpusForAnalysis(ctx, owner, analysisRunID)
	if err != nil {
		return Artifact{}, fmt.Errorf("load completed analysis corpus: %w", err)
	}
	candidates, err := store.ListSelectionCandidatesForCorpus(ctx, owner, corpus.ID)
	if err != nil {
		return Artifact{}, fmt.Errorf("list scoped selection candidates: %w", err)
	}
	selected, err := s.coverageCandidates(ctx, owner, corpus.SourceMaterialID, candidates)
	if err != nil {
		return Artifact{}, err
	}
	persistedSentences, err := s.loadCorpusSentences(ctx, owner, selected)
	if err != nil {
		return Artifact{}, err
	}
	sort.SliceStable(selected, func(i, j int) bool {
		if selected[i].FirstEncounter != selected[j].FirstEncounter {
			return selected[i].FirstEncounter < selected[j].FirstEncounter
		}
		return candidateKey(selected[i]) < candidateKey(selected[j])
	})

	entries := make([]Entry, 0, len(selected))
	source, err := store.GetSourceMaterial(ctx, owner, corpus.SourceMaterialID)
	if err != nil {
		return Artifact{}, fmt.Errorf("load scoped analysis source: %w", err)
	}
	deckName := source.Title
	for _, candidate := range selected {
		entry, err := store.GetCoverageEntryForCorpus(ctx, owner, corpus.ID, candidate)
		if err != nil {
			return Artifact{}, fmt.Errorf("get scoped coverage entry %s: %w", candidateKey(candidate), err)
		}
		applySentenceDecision(&entry, candidate, persistedSentences[candidate.CorpusID])
		entries = append(entries, entry)
		if strings.TrimSpace(entry.SourceDocument) != "" {
			deckName = entry.SourceDocument
		}
	}
	return s.render(ctx, owner, deckName, entries)
}

// PrepareCoverage freezes the legacy book-scoped candidate and render inputs
// without rendering an artifact or reading a non-exact enrichment cache row.
func (s *Service) PrepareCoverage(ctx context.Context, owner, bookID string) (Manifest, error) {
	if s == nil || s.store == nil || strings.TrimSpace(owner) == "" || strings.TrimSpace(bookID) == "" {
		return Manifest{}, ErrInvalidInput
	}
	candidates, err := s.store.ListSelectionCandidatesForBook(ctx, owner, bookID)
	if err != nil {
		return Manifest{}, fmt.Errorf("list selection candidates: %w", err)
	}
	selected, err := s.coverageCandidates(ctx, owner, bookID, candidates)
	if err != nil {
		return Manifest{}, err
	}
	persistedSentences, err := s.loadCorpusSentences(ctx, owner, selected)
	if err != nil {
		return Manifest{}, err
	}
	sortCandidatesByEncounter(selected)
	entries := make([]Entry, 0, len(selected))
	deckName := bookID
	for _, candidate := range selected {
		entry, err := s.preparedEntryForBook(ctx, owner, bookID, candidate)
		if err != nil {
			return Manifest{}, fmt.Errorf("get coverage entry %s: %w", candidateKey(candidate), err)
		}
		applySentenceDecision(&entry, candidate, persistedSentences[candidate.CorpusID])
		entries = append(entries, entry)
		if strings.TrimSpace(entry.SourceDocument) != "" {
			deckName = entry.SourceDocument
		}
	}
	return NewManifest(owner, deckName, entries), nil
}

// PrepareCoverageForAnalysis freezes only the immutable corpus produced by a
// completed scoped analysis, without rendering or assigning vocabulary state.
func (s *Service) PrepareCoverageForAnalysis(ctx context.Context, owner, analysisRunID string) (Manifest, error) {
	if s == nil || s.store == nil || strings.TrimSpace(owner) == "" || strings.TrimSpace(analysisRunID) == "" {
		return Manifest{}, ErrInvalidInput
	}
	store, ok := s.store.(analysisStore)
	if !ok {
		return Manifest{}, errors.New("cardexport: scoped analysis storage is unavailable")
	}
	corpus, err := store.GetCorpusForAnalysis(ctx, owner, analysisRunID)
	if err != nil {
		return Manifest{}, fmt.Errorf("load completed analysis corpus: %w", err)
	}
	candidates, err := store.ListSelectionCandidatesForCorpus(ctx, owner, corpus.ID)
	if err != nil {
		return Manifest{}, fmt.Errorf("list scoped selection candidates: %w", err)
	}
	selected, err := s.coverageCandidates(ctx, owner, corpus.SourceMaterialID, candidates)
	if err != nil {
		return Manifest{}, err
	}
	persistedSentences, err := s.loadCorpusSentences(ctx, owner, selected)
	if err != nil {
		return Manifest{}, err
	}
	sortCandidatesByEncounter(selected)
	source, err := store.GetSourceMaterial(ctx, owner, corpus.SourceMaterialID)
	if err != nil {
		return Manifest{}, fmt.Errorf("load scoped analysis source: %w", err)
	}
	entries := make([]Entry, 0, len(selected))
	deckName := source.Title
	for _, candidate := range selected {
		entry, err := s.preparedEntryForCorpus(ctx, store, owner, corpus.ID, candidate)
		if err != nil {
			return Manifest{}, fmt.Errorf("get scoped coverage entry %s: %w", candidateKey(candidate), err)
		}
		applySentenceDecision(&entry, candidate, persistedSentences[candidate.CorpusID])
		entries = append(entries, entry)
		if strings.TrimSpace(entry.SourceDocument) != "" {
			deckName = entry.SourceDocument
		}
	}
	return NewManifest(owner, deckName, entries), nil
}

func (s *Service) preparedEntryForBook(ctx context.Context, owner, bookID string, candidate domain.SelectionCandidate) (Entry, error) {
	if store, ok := s.store.(preparedEntryStore); ok {
		return store.GetPreparedCoverageEntryForBook(ctx, owner, bookID, candidate)
	}
	entry, err := s.store.GetCoverageEntryForBook(ctx, owner, bookID, candidate)
	clearExternalFields(&entry)
	return entry, err
}

func (s *Service) preparedEntryForCorpus(ctx context.Context, store analysisStore, owner, corpusID string, candidate domain.SelectionCandidate) (Entry, error) {
	if prepared, ok := s.store.(preparedEntryStore); ok {
		return prepared.GetPreparedCoverageEntryForCorpus(ctx, owner, corpusID, candidate)
	}
	entry, err := store.GetCoverageEntryForCorpus(ctx, owner, corpusID, candidate)
	clearExternalFields(&entry)
	return entry, err
}

func clearExternalFields(entry *Entry) {
	entry.Translation = ""
	entry.SentenceTranslation = ""
	entry.SentenceTranslationTarget = ""
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
		entry.Sentence = evidence.Sentence
		entry.TargetWord = evidence.Target
		entry.FirstEncounter = evidence.FirstEncounter
		entry.SentenceTokens = evidence.Tokens
	} else {
		entry.TargetWord = targetWord(entry.Sentence, candidate)
		if persisted != nil {
			// An unavailable persisted sentence is not eligible for syntax scoring.
			entry.SentenceTokens = []analyzer.Token{}
		}
	}
}

func (s *Service) loadCorpusSentences(ctx context.Context, owner string, candidates []domain.SelectionCandidate) (map[string]map[int64]analyzer.Sentence, error) {
	store, ok := s.store.(corpusSentenceStore)
	if !ok {
		return nil, nil
	}
	ordinalsByCorpus := make(map[string]map[int64]struct{})
	for _, candidate := range candidates {
		if strings.TrimSpace(candidate.CorpusID) == "" {
			continue
		}
		var refs []sentenceReference
		if json.Unmarshal(candidate.SentenceReferences, &refs) != nil {
			continue
		}
		ordinals := ordinalsByCorpus[candidate.CorpusID]
		if ordinals == nil {
			ordinals = make(map[int64]struct{})
			ordinalsByCorpus[candidate.CorpusID] = ordinals
		}
		for _, ref := range refs {
			if ref.SentenceIndex >= 0 {
				ordinals[int64(ref.SentenceIndex)] = struct{}{}
			}
		}
	}
	result := make(map[string]map[int64]analyzer.Sentence, len(ordinalsByCorpus))
	for corpusID, ordinalSet := range ordinalsByCorpus {
		ordinals := make([]int64, 0, len(ordinalSet))
		for ordinal := range ordinalSet {
			ordinals = append(ordinals, ordinal)
		}
		slices.Sort(ordinals)
		sentences, err := store.ListCorpusSentences(ctx, owner, corpusID, ordinals)
		if err != nil {
			return nil, fmt.Errorf("list persisted corpus sentences for %s: %w", corpusID, err)
		}
		if len(sentences) > 0 {
			for ordinal := range ordinalSet {
				if _, ok := sentences[ordinal]; !ok {
					return nil, fmt.Errorf("list persisted corpus sentences for %s: missing sentence ordinal %d", corpusID, ordinal)
				}
			}
		}
		if len(sentences) > 0 {
			result[corpusID] = sentences
		}
	}
	return result, nil
}

func sortCandidatesByEncounter(candidates []domain.SelectionCandidate) {
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].FirstEncounter != candidates[j].FirstEncounter {
			return candidates[i].FirstEncounter < candidates[j].FirstEncounter
		}
		return candidateKey(candidates[i]) < candidateKey(candidates[j])
	})
}

// NewManifest quality-gates entries once and returns an immutable render plan.
func NewManifest(owner, deckName string, entries []Entry) Manifest {
	manifest := Manifest{owner: owner, deckName: deckName, accepted: make([]Entry, 0, len(entries)), omitted: make([]Omission, 0), enrichmentCandidates: make([]enrichment.Candidate, 0, len(entries)), decisions: make([]ManifestItem, 0, len(entries))}
	for ordinal, entry := range entries {
		entry.UPOS = strings.ToUpper(strings.TrimSpace(entry.UPOS))
		entry.TargetWord = testedTarget(entry)
		decisionEntry := entry
		clearExternalFields(&decisionEntry)
		decisionEntry.OwnerID = ""
		decisionEntry.SentenceTokens = nil
		decisionEntry.SentenceTokens = nil
		quality := scoreSentenceQuality(entry.Language, entry.Sentence, entry.TargetWord, entry.FirstEncounter, entry.SentenceTokens)
		if !quality.Accepted {
			manifest.omitted = append(manifest.omitted, Omission{Language: entry.Language, CanonicalLemma: entry.CanonicalLemma, UPOS: entry.UPOS, Score: quality.Score, Reasons: append([]string(nil), quality.Reasons...)})
			manifest.decisions = append(manifest.decisions, ManifestItem{Ordinal: ordinal, Disposition: ManifestQualityOmitted, Entry: decisionEntry, Quality: quality})
			continue
		}
		manifest.accepted = append(manifest.accepted, entry)
		manifest.decisions = append(manifest.decisions, ManifestItem{Ordinal: ordinal, Disposition: ManifestAccepted, Entry: decisionEntry, Quality: quality})
		manifest.enrichmentCandidates = append(manifest.enrichmentCandidates, enrichment.Candidate{
			Identity:        enrichment.Identity{Language: entry.Language, CanonicalLemma: entry.CanonicalLemma, UPOS: entry.UPOS},
			TargetWord:      entry.TargetWord,
			ExampleSentence: strings.TrimSpace(entry.Sentence),
		})
	}
	return manifest
}

// EnrichmentCandidates returns the frozen candidates in final render order.
func (m Manifest) EnrichmentCandidates() []enrichment.Candidate {
	return append([]enrichment.Candidate(nil), m.enrichmentCandidates...)
}

// Completeness reports the manifest's pre-enrichment accepted and omitted
// counts. Final optional-field counts are calculated during the sole render.
func (m Manifest) Completeness() Completeness {
	result := Completeness{TotalCards: len(m.accepted), QualityOmitted: len(m.omitted)}
	for _, entry := range m.accepted {
		if strings.TrimSpace(entry.Translation) != "" {
			result.CardsWithEnglish++
		}
		if strings.TrimSpace(entry.SentenceTranslation) != "" {
			result.CardsWithEnglishSentence++
		}
	}
	return result
}

// BindCacheKeys returns a new manifest bound to the exact provider/version and
// sentence identities that enrichment will use. Existing external fields are
// cleared so a legacy or mismatched cache row can never survive finalization.
func (m Manifest) BindCacheKeys(keys []enrichment.CacheKey) (Manifest, error) {
	if len(keys) != len(m.enrichmentCandidates) {
		return Manifest{}, fmt.Errorf("%w: cache identity count does not match manifest candidates", ErrInvalidInput)
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
			return Manifest{}, fmt.Errorf("%w: cache identity does not match manifest candidate %d", ErrInvalidInput, i)
		}
		exactSentenceHash := enrichment.SentenceHash(candidate.ExampleSentence)
		if key.SentenceHash != "" && key.SentenceHash != exactSentenceHash {
			return Manifest{}, fmt.Errorf("%w: sentence cache identity does not match manifest candidate %d", ErrInvalidInput, i)
		}
		clearExternalFields(&bound.accepted[i])
		bound.decisions[decisionIndex].CacheKey = &key
		acceptedIndex++
	}
	return bound, nil
}

func (m Manifest) clone() Manifest {
	m.accepted = append([]Entry(nil), m.accepted...)
	m.omitted = append([]Omission(nil), m.omitted...)
	for i := range m.omitted {
		m.omitted[i].Reasons = append([]string(nil), m.omitted[i].Reasons...)
	}
	m.enrichmentCandidates = append([]enrichment.Candidate(nil), m.enrichmentCandidates...)
	m.cacheKeys = append([]enrichment.CacheKey(nil), m.cacheKeys...)
	m.decisions = cloneManifestItems(m.decisions)
	return m
}

// RenderManifest applies exact enrichment outcomes and renders the frozen plan
// once. Outcomes are positional so duplicate or missing assignment is rejected.
func (s *Service) RenderManifest(ctx context.Context, manifest Manifest, outcomes []ExactEnrichment) (Artifact, error) {
	entries := append([]Entry(nil), manifest.accepted...)
	omitted := append([]Omission(nil), manifest.omitted...)
	for i := range omitted {
		omitted[i].Reasons = append([]string(nil), omitted[i].Reasons...)
	}
	if len(outcomes) > 0 || len(manifest.cacheKeys) > 0 {
		if len(outcomes) != len(entries) || len(manifest.cacheKeys) != len(entries) {
			return Artifact{}, fmt.Errorf("%w: exact enrichment count does not match manifest", ErrInvalidInput)
		}
		for i, outcome := range outcomes {
			if outcome.CacheKey != manifest.cacheKeys[i] {
				return Artifact{}, fmt.Errorf("%w: exact enrichment cache identity mismatch at candidate %d", ErrInvalidInput, i)
			}
			if err := applyExactEnrichment(&entries[i], outcome); err != nil {
				return Artifact{}, fmt.Errorf("candidate %d: %w", i, err)
			}
		}
	}
	return s.renderAccepted(ctx, manifest.owner, manifest.deckName, entries, omitted)
}

func applyExactEnrichment(entry *Entry, outcome ExactEnrichment) error {
	result := outcome.Result
	fields := []struct {
		available  bool
		provenance enrichment.Provenance
	}{
		{result.Translation.Available, result.Translation.Provenance},
		{result.SentenceTranslation.Available, result.SentenceTranslation.Provenance},
		{result.SentenceTranslationTarget.Available, result.SentenceTranslationTarget.Provenance},
	}
	available := false
	for _, field := range fields {
		available = available || field.available
		if field.available && (field.provenance.Provider != outcome.CacheKey.Provider || field.provenance.ProviderVersion != outcome.CacheKey.ProviderVersion) {
			return fmt.Errorf("%w: enrichment provenance does not match cache identity", ErrInvalidInput)
		}
	}
	if available {
		candidate := result.Candidate
		if candidate.Language != outcome.CacheKey.Language || candidate.CanonicalLemma != outcome.CacheKey.CanonicalLemma || strings.ToUpper(candidate.UPOS) != outcome.CacheKey.UPOS || testedTarget(*entry) != testedTarget(Entry{CanonicalLemma: candidate.CanonicalLemma, TargetWord: candidate.TargetWord}) {
			return fmt.Errorf("%w: enrichment candidate does not match cache identity", ErrInvalidInput)
		}
		if outcome.CacheKey.SentenceHash != "" && enrichment.SentenceHash(candidate.ExampleSentence) != outcome.CacheKey.SentenceHash {
			return fmt.Errorf("%w: enrichment sentence does not match cache identity", ErrInvalidInput)
		}
	}
	if result.Translation.Available {
		entry.Translation = result.Translation.Value
	}
	if result.SentenceTranslation.Available {
		entry.SentenceTranslation = result.SentenceTranslation.Value
	}
	if result.SentenceTranslationTarget.Available {
		entry.SentenceTranslationTarget = result.SentenceTranslationTarget.Value
	}
	return nil
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

const defaultDeckMinOccurrences = 3

func selectRecurringCandidates(candidates []domain.SelectionCandidate, minOccurrences int) []domain.SelectionCandidate {
	selected := slices.Clone(candidates)
	if selected == nil {
		selected = []domain.SelectionCandidate{}
	}
	return slices.DeleteFunc(selected, func(candidate domain.SelectionCandidate) bool {
		return candidate.OccurrenceCount < minOccurrences
	})
}

func (s *Service) coverageCandidates(ctx context.Context, owner, bookID string, candidates []domain.SelectionCandidate) ([]domain.SelectionCandidate, error) {
	knownByLanguage := make(map[string]map[string]bool)
	generatedByLanguage := make(map[string]map[string]bool)
	unknown := make([]domain.SelectionCandidate, 0, len(candidates))
	for _, candidate := range candidates {
		known, ok := knownByLanguage[candidate.Language]
		if !ok {
			words, err := s.store.ListKnownVocabulary(ctx, owner, candidate.Language)
			if err != nil {
				return nil, fmt.Errorf("list known vocabulary for %s: %w", candidate.Language, err)
			}
			known = make(map[string]bool, len(words))
			for _, word := range words {
				known[word.CanonicalLemma+"\x00"+word.UPOS] = true
			}
			knownByLanguage[candidate.Language] = known
		}
		generated, ok := generatedByLanguage[candidate.Language]
		if !ok {
			words, err := s.store.ListUnattachedGeneratedVocabulary(ctx, owner, candidate.Language)
			if err != nil {
				return nil, fmt.Errorf("list generated vocabulary for %s: %w", candidate.Language, err)
			}
			generated = make(map[string]bool, len(words))
			for _, word := range words {
				// Unknown provenance is excluded conservatively. Explicit provenance
				// for this book remains eligible so repeating an export is idempotent.
				if word.FirstSourceMaterialID == nil || *word.FirstSourceMaterialID != bookID {
					generated[word.CanonicalLemma+"\x00"+word.UPOS] = true
				}
			}
			reservedWords, err := s.store.ListReservedVocabulary(ctx, owner, candidate.Language)
			if err != nil {
				return nil, fmt.Errorf("list reserved vocabulary for %s: %w", candidate.Language, err)
			}
			for _, word := range reservedWords {
				generated[word.CanonicalLemma+"\x00"+word.UPOS] = true
			}
			generatedByLanguage[candidate.Language] = generated
		}
		identity := candidate.CanonicalLemma + "\x00" + candidate.UPOS
		if !known[identity] && !known[candidate.CanonicalLemma+"\x00"] && !generated[identity] {
			unknown = append(unknown, candidate)
		}
	}
	return selectRecurringCandidates(unknown, defaultDeckMinOccurrences), nil
}

func candidateKey(candidate domain.SelectionCandidate) string {
	return candidate.Language + "\x00" + candidate.CanonicalLemma + "\x00" + candidate.UPOS
}

func (s *Service) render(ctx context.Context, owner, deckName string, entries []Entry) (Artifact, error) {
	return s.RenderManifest(ctx, NewManifest(owner, deckName, entries), nil)
}

func (s *Service) renderAccepted(ctx context.Context, owner, deckName string, entries []Entry, omitted []Omission) (Artifact, error) {
	type acceptedNote struct {
		entry Entry
		note  Note
	}
	accepted := make([]acceptedNote, 0, len(entries))
	enrichmentCandidates := make([]enrichment.Candidate, 0, len(entries))
	completeness := Completeness{QualityOmitted: len(omitted)}
	for _, entry := range entries {
		n, err := makeNote(owner, entry)
		if err != nil {
			return Artifact{}, fmt.Errorf("render %s/%s/%s: %w", entry.Language, entry.CanonicalLemma, entry.UPOS, err)
		}
		accepted = append(accepted, acceptedNote{entry: entry, note: n})
		completeness.TotalCards++
		if strings.TrimSpace(entry.Translation) != "" {
			completeness.CardsWithEnglish++
		}
		if strings.TrimSpace(entry.SentenceTranslation) != "" {
			completeness.CardsWithEnglishSentence++
		}
		enrichmentCandidates = append(enrichmentCandidates, enrichment.Candidate{
			Identity:        enrichment.Identity{Language: entry.Language, CanonicalLemma: entry.CanonicalLemma, UPOS: entry.UPOS},
			TargetWord:      testedTarget(entry),
			ExampleSentence: strings.TrimSpace(entry.Sentence),
		})
	}
	notes := make([]Note, len(accepted))
	for i := range accepted {
		notes[i] = accepted[i].note
	}
	tsv, err := RenderTSV(notes)
	if err != nil {
		return Artifact{}, fmt.Errorf("render TSV: %w", err)
	}
	language := "und"
	if len(entries) > 0 && strings.TrimSpace(entries[0].Language) != "" {
		language = entries[0].Language
	}
	ankiDeckName := DeckName(language, deckName)
	apkg, err := renderAPKG(ankiDeckName, notes)
	if err != nil {
		return Artifact{}, fmt.Errorf("render Anki package: %w", err)
	}
	generated := make([]GeneratedRecord, len(accepted))
	for i, item := range accepted {
		generated[i] = GeneratedRecord{Entry: item.entry, Note: item.note}
	}
	return Artifact{APKG: apkg, Filename: DownloadFilename(deckName), DeckName: ankiDeckName, TSV: tsv, Count: len(notes), Completeness: completeness, Omitted: omitted, EnrichmentCandidates: enrichmentCandidates, Generated: generated}, nil
}
