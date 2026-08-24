// Package cardexport produces owner-scoped Anki Cloze deck packages.
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
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/lemmadisplay"
)

var ErrInvalidInput = errors.New("cardexport: invalid input")

type Entry struct {
	OwnerID, Language, CanonicalLemma, UPOS                string
	Sentence, Translation, SentenceTranslation, TargetWord string
	Morphology, SourceDocument, Notes                      string
	FirstEncounter                                         int64
}

type Note struct {
	Key, Text, Lemma, POS, Morph, English, EnglishSentence, BookTitle, SourceSentence string
	BackExtra                                                                         string
	Tags                                                                              []string
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
	Accepted bool
	Score    int
	Reasons  []string
}

type SentenceEvidence struct {
	Sentence, Target string
	FirstEncounter   int64
	Quality          SentenceQuality
}

type Store interface {
	ListSelectionCandidatesForBook(context.Context, string, string) ([]domain.SelectionCandidate, error)
	ListKnownVocabulary(context.Context, string, string) ([]domain.KnownVocabulary, error)
	ListGeneratedVocabulary(context.Context, string, string) ([]domain.GeneratedVocabulary, error)
	GetCoverageEntryForBook(context.Context, string, string, domain.SelectionCandidate) (Entry, error)
	RecordGeneratedForBook(context.Context, string, string, string, Entry, Note) error
}

type Service struct{ store Store }

func NewService(store Store) *Service { return &Service{store: store} }

func DedupKey(language, lemma, upos, owner string) string {
	sum := sha256.Sum256([]byte(language + " | " + lemma + " | " + upos + " | " + owner))
	return hex.EncodeToString(sum[:])
}

func Cloze(sentence, target, hint string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" || strings.TrimSpace(sentence) == "" {
		return "", ErrInvalidInput
	}
	start := targetIndex(sentence, target)
	if start < 0 {
		return "", fmt.Errorf("%w: target %q not found in sentence", ErrInvalidInput, target)
	}
	end := start + len(target)
	mark := "{{c1::" + escapeField(sentence[start:end])
	if hint = strings.TrimSpace(hint); hint != "" {
		mark += "::" + escapeField(hint)
	}
	mark += "}}"
	return escapeField(sentence[:start]) + mark + escapeField(sentence[end:]), nil
}

func escapeField(value string) string {
	value = html.EscapeString(value)
	value = strings.ReplaceAll(value, "{", "&#123;")
	return strings.ReplaceAll(value, "}", "&#125;")
}

const (
	minimumSentenceWords      = 6
	maximumSentenceWords      = 50
	maximumSentenceCharacters = 400
	minimumSentenceScore      = 70
)

// ScoreSentenceQuality applies a deliberately small, explainable export gate
// using only sentence text, the selected target form, and source location.
func ScoreSentenceQuality(sentence, target string, firstEncounter int64) SentenceQuality {
	text := strings.TrimSpace(sentence)
	words := strings.Fields(text)
	quality := SentenceQuality{Reasons: make([]string, 0, 8)}
	reject := func(reason string) { quality.Reasons = append(quality.Reasons, reason) }

	if !utf8.ValidString(text) || len(words) < minimumSentenceWords {
		reject("too short or fragmented")
	} else if len(words) > maximumSentenceWords || utf8.RuneCountInString(text) > maximumSentenceCharacters {
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

	quality.Accepted = quality.Score >= minimumSentenceScore && !containsRejection(quality.Reasons)
	return quality
}

func containsRejection(reasons []string) bool {
	for _, reason := range reasons {
		switch reason {
		case "too short or fragmented", "too long", "target not present as a word", "invalid source location", "incomplete sentence boundaries", "structural noise or boilerplate":
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

// BestSentenceEvidence ranks every eligible source reference without changing
// the candidate's first-encounter ordering. Ties prefer source order, then
// sentence index and text, so repeated exports are stable.
func BestSentenceEvidence(candidate domain.SelectionCandidate) (SentenceEvidence, bool) {
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
		location, valid := referenceStartOffset(ref.Location)
		if !valid {
			location = -1
		}
		target := ""
		for _, form := range forms {
			if targetIndex(ref.Text, form) >= 0 {
				target = form
				break
			}
		}
		quality := ScoreSentenceQuality(ref.Text, target, location)
		ranked = append(ranked, struct {
			evidence SentenceEvidence
			index    int
			sentence int
		}{SentenceEvidence{strings.TrimSpace(ref.Text), target, location, quality}, i, ref.SentenceIndex})
	}
	if len(ranked) == 0 {
		return SentenceEvidence{}, false
	}
	sort.SliceStable(ranked, func(i, j int) bool {
		if ranked[i].evidence.Quality.Accepted != ranked[j].evidence.Quality.Accepted {
			return ranked[i].evidence.Quality.Accepted
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
	if target == "" {
		return -1
	}
	lowerSentence, lowerTarget := strings.ToLower(sentence), strings.ToLower(target)
	for offset := 0; offset <= len(lowerSentence)-len(lowerTarget); {
		relative := strings.Index(lowerSentence[offset:], lowerTarget)
		if relative < 0 {
			return -1
		}
		start := offset + relative
		end := start + len(lowerTarget)
		if wordBoundaryBefore(lowerSentence, start) && wordBoundaryAfter(lowerSentence, end) {
			return start
		}
		offset = start + 1
	}
	return -1
}

func wordBoundaryBefore(text string, index int) bool {
	if index == 0 {
		return true
	}
	r, _ := utf8.DecodeLastRuneInString(text[:index])
	return !unicode.IsLetter(r) && !unicode.IsNumber(r)
}

func wordBoundaryAfter(text string, index int) bool {
	if index == len(text) {
		return true
	}
	r, _ := utf8.DecodeRuneInString(text[index:])
	return !unicode.IsLetter(r) && !unicode.IsNumber(r)
}

func makeNote(owner string, entry Entry) (Note, error) {
	target := strings.TrimSpace(entry.TargetWord)
	if target == "" {
		target = entry.CanonicalLemma
	}
	text, err := Cloze(entry.Sentence, target, entry.Translation)
	if err != nil {
		return Note{}, err
	}
	tags := uniqueTags("Mouseion", prefixedTag("lang", entry.Language), prefixedTag("pos", entry.UPOS), prefixedTag("source", entry.SourceDocument))
	note := Note{
		Key:  DedupKey(entry.Language, entry.CanonicalLemma, entry.UPOS, owner),
		Text: text, Lemma: escapeField(lemmadisplay.Format(entry.Language, entry.CanonicalLemma, entry.UPOS)),
		POS: escapeField(entry.UPOS), Morph: escapeField(entry.Morphology), English: escapeField(entry.Translation),
		EnglishSentence: escapeField(entry.SentenceTranslation), BookTitle: escapeField(entry.SourceDocument), SourceSentence: escapeField(entry.Sentence), Tags: tags,
	}
	note.BackExtra = strings.Join([]string{note.Lemma, note.Morph, note.POS, note.English, note.EnglishSentence}, "\n")
	return note, nil
}

func prefixedTag(prefix, value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return prefix + "::" + value
}

func uniqueTags(values ...string) []string {
	seen := map[string]bool{}
	var tags []string
	for _, value := range values {
		value = strings.Join(strings.Fields(value), "_")
		value = strings.NewReplacer("\\", "_", "/", "_", "#", "_", "^", "_", "\x00", "_").Replace(value)
		if value != "" && !seen[value] {
			seen[value] = true
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
	return []string{n.Text, n.Lemma, n.POS, n.Morph, n.English, n.EnglishSentence, n.BookTitle, n.SourceSentence}
}

const noteTypeName = "Mouseion Vocab Cloze"

var fieldNames = []string{"Text", "Lemma", "POS", "Morph", "English", "EnglishSentence", "BookTitle", "SourceSentence"}

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

// ExportCoverage exports the smallest set of unknown lemmas accounting for at
// least 97 percent of the book's unknown lemma tokens, in reading order.
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
		if evidence, ok := BestSentenceEvidence(candidate); ok {
			entry.Sentence = evidence.Sentence
			entry.TargetWord = evidence.Target
			entry.FirstEncounter = evidence.FirstEncounter
		} else {
			entry.TargetWord = targetWord(entry.Sentence, candidate)
		}
		entries = append(entries, entry)
		if strings.TrimSpace(entry.SourceDocument) != "" {
			deckName = entry.SourceDocument
		}
	}
	return s.render(ctx, owner, deckName, entries)
}

func targetWord(sentence string, candidate domain.SelectionCandidate) string {
	var forms []string
	if json.Unmarshal(candidate.ObservedForms, &forms) == nil {
		for _, form := range forms {
			if targetIndex(sentence, form) >= 0 {
				return form
			}
		}
	}
	return candidate.CanonicalLemma
}

const coveragePercent = 97

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
			words, err := s.store.ListGeneratedVocabulary(ctx, owner, candidate.Language)
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
			generatedByLanguage[candidate.Language] = generated
		}
		identity := candidate.CanonicalLemma + "\x00" + candidate.UPOS
		if !known[identity] && !known[candidate.CanonicalLemma+"\x00"] && !generated[identity] {
			unknown = append(unknown, candidate)
		}
	}
	sort.SliceStable(unknown, func(i, j int) bool {
		if unknown[i].OccurrenceCount != unknown[j].OccurrenceCount {
			return unknown[i].OccurrenceCount > unknown[j].OccurrenceCount
		}
		return candidateKey(unknown[i]) < candidateKey(unknown[j])
	})
	total := 0
	for _, candidate := range unknown {
		total += candidate.OccurrenceCount
	}
	cumulative, count := 0, 0
	for count < len(unknown) && cumulative*100 < total*coveragePercent {
		cumulative += unknown[count].OccurrenceCount
		count++
	}
	return unknown[:count], nil
}

func candidateKey(candidate domain.SelectionCandidate) string {
	return candidate.Language + "\x00" + candidate.CanonicalLemma + "\x00" + candidate.UPOS
}

func (s *Service) render(ctx context.Context, owner, deckName string, entries []Entry) (Artifact, error) {
	type acceptedNote struct {
		entry Entry
		note  Note
	}
	accepted := make([]acceptedNote, 0, len(entries))
	enrichmentCandidates := make([]enrichment.Candidate, 0, len(entries))
	omitted := make([]Omission, 0)
	completeness := Completeness{}
	for _, entry := range entries {
		quality := ScoreSentenceQuality(entry.Sentence, entry.TargetWord, entry.FirstEncounter)
		if !quality.Accepted {
			omitted = append(omitted, Omission{Language: entry.Language, CanonicalLemma: entry.CanonicalLemma, UPOS: entry.UPOS, Score: quality.Score, Reasons: quality.Reasons})
			continue
		}
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
			ExampleSentence: strings.TrimSpace(entry.Sentence),
		})
	}
	completeness.QualityOmitted = len(omitted)
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
