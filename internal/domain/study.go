package domain

import (
	"errors"
	"slices"
	"sort"
	"strings"
	"time"
)

// StudyPreparationWindow is how many words, in stream order, Study material is
// kept prepared ahead: about one Study day's maximum offer of three batches of
// ten (ADR 0134, spec #2691).
const StudyPreparationWindow = 30

// StudyItemKind is which kind of thing a Study item is (ADR 0137 §1). The zero
// value is StudyItemWord, so an item built without naming a kind is a word.
type StudyItemKind int

const (
	// StudyItemWord is a word, keyed by its full vocabulary identity.
	StudyItemWord StudyItemKind = iota
	// StudyItemIdiom is an Idiom, keyed by language and headword.
	StudyItemIdiom
)

// String is the kind as stored: "word" or "idiom".
func (k StudyItemKind) String() string {
	if k == StudyItemIdiom {
		return "idiom"
	}
	return "word"
}

// ParseStudyItemKind reads a stored kind. It reports false for any other text.
func ParseStudyItemKind(text string) (StudyItemKind, bool) {
	switch text {
	case "word":
		return StudyItemWord, true
	case "idiom":
		return StudyItemIdiom, true
	}
	return StudyItemWord, false
}

// StudyItem is what Study progress, Study material, Fluency contexts and Study
// reports are owned by: learner (the caller's owner), study language and a key
// that depends on the kind (ADR 0137 §1).
//
// A word is keyed by its full vocabulary identity: lemma, part of speech and
// lexical discriminator (ADR 0134 §2). An Idiom is keyed by its headword, held
// in CanonicalLemma, with UPOS and LexicalDiscriminator empty. Two items of
// different kinds are never equal, so a word and an Idiom with the same text
// never collide in a map or a uniqueness rule.
type StudyItem struct {
	ItemKind                                             StudyItemKind
	Language, CanonicalLemma, UPOS, LexicalDiscriminator string
}

// WordItem is the word item of a full vocabulary identity.
func WordItem(language, canonicalLemma, upos, lexicalDiscriminator string) StudyItem {
	return StudyItem{Language: language, CanonicalLemma: canonicalLemma, UPOS: upos, LexicalDiscriminator: lexicalDiscriminator}
}

// IdiomItem is the Idiom item of a headword.
func IdiomItem(language, headword string) StudyItem {
	return StudyItem{ItemKind: StudyItemIdiom, Language: language, CanonicalLemma: headword}
}

// IsIdiom reports whether the item is an Idiom.
func (i StudyItem) IsIdiom() bool { return i.ItemKind == StudyItemIdiom }

// Headword is an Idiom's headword. It reports false for a word.
func (i StudyItem) Headword() (string, bool) {
	if !i.IsIdiom() {
		return "", false
	}
	return i.CanonicalLemma, true
}

// StudyTargetCoverage is the Target coverage (ADR 0136 §2): the share of the
// Current reading's analyzable tokens, counting Mature and in-study words, at
// which its words stop coming first in New-word batches. It is one constant for
// every language, not a learner setting.
const StudyTargetCoverage = 0.98

// studyTargetCoveragePercent is StudyTargetCoverage as a whole percent, so the
// comparison is exact integer math (an untyped constant: the product is 98).
const studyTargetCoveragePercent = int64(StudyTargetCoverage * 100)

// StudyReadingCoverage is how far the Current reading is toward Target
// coverage: its in-study-or-Mature tokens over its analyzable tokens. Active is
// false when there is no Current reading, or its counts are not ready, and the
// other fields are then zero.
type StudyReadingCoverage struct {
	Active                          bool
	InStudyTokens, AnalyzableTokens int64
}

// NewStudyReadingCoverage is the Current reading's coverage from its projected
// vocabulary and the learner's Study progress. A Book without persisted
// statistics or ready counts has no coverage to speak of and no boost.
func NewStudyReadingCoverage(vocabulary ProjectedCorpusVocabulary, progress []StudyProgress) StudyReadingCoverage {
	if vocabulary.Statistics == nil || !vocabulary.Ready {
		return StudyReadingCoverage{}
	}
	index := NewStudyCoverageIndex(progress)
	var inStudy int64
	for _, lemma := range vocabulary.Lemmas {
		if studying, _ := index.Classify(lemma); studying {
			inStudy += lemma.OccurrenceCount
		}
	}
	analyzable := vocabulary.Statistics.AnalyzableTokenCount
	return StudyReadingCoverage{Active: true, InStudyTokens: min(inStudy, analyzable), AnalyzableTokens: analyzable}
}

// BoostsCurrentReading reports whether the Current reading's words come first:
// there is a Current reading and its coverage is below Target coverage. Exactly
// at the target the boost is off, and it returns when coverage falls below.
func (c StudyReadingCoverage) BoostsCurrentReading() bool {
	return c.Active && c.InStudyTokens*100 < c.AnalyzableTokens*studyTargetCoveragePercent
}

// StudyCandidate is one effective vocabulary identity that Study could offer
// as a new word. PersonalOccurrences is its total across the Corpus: the
// learner's currently analyzed Books in the study language, Read Books included.
// CurrentReadingOccurrences is its count within the Current reading, and
// ToReadOccurrences its sum over the To Read Books (the Current reading is one,
// ADR 0136 §2); Read Books add nothing to either, so they set no priority. Known
// and Reserved vocabulary are deliberately not part of a candidate: Study never
// consults them (ADR 0134 §3).
type StudyCandidate struct {
	StudyItem
	InCurrentReading          bool
	PersonalOccurrences       int64
	CurrentReadingOccurrences int64
	ToReadOccurrences         int64
}

// boosted reports whether the candidate is in the boosted group while the
// Current reading is below Target coverage: its words and its Idioms. Coverage
// counts word tokens only, so an Idiom joins the group but never extends the
// boost (ADR 0137).
func (c StudyCandidate) boosted() bool { return c.InCurrentReading }

// boostCount is the count a boosted candidate is ranked by.
func (c StudyCandidate) boostCount() int64 { return c.CurrentReadingOccurrences }

// frequency is the count a candidate is ranked by outside the boost: a word's
// occurrences summed over the To Read Books, and an Idiom's candidate count in
// the Corpus.
func (c StudyCandidate) frequency() int64 {
	if c.IsIdiom() {
		return c.PersonalOccurrences
	}
	return c.ToReadOccurrences
}

// OrderStudyStream returns the candidates in stream order. While boost is set
// (the Current reading is below Target coverage) its words come first, by their
// occurrences within it; every other candidate follows by frequency across the
// To Read Books (an Idiom's count is its candidate count in the Corpus), then
// by kind (words before Idioms) and key, so the order is deterministic. Without
// the boost every candidate is ranked by frequency. Words and Idioms share the
// one stream, with no tier for a part of speech (ADR 0137 §4). The input is not
// modified.
func OrderStudyStream(candidates []StudyCandidate, boost bool) []StudyCandidate {
	ordered := append([]StudyCandidate{}, candidates...)
	sort.SliceStable(ordered, func(i, j int) bool { return studyStreamBefore(ordered[i], ordered[j], boost) })
	return ordered
}

// studyStreamBefore reports whether a comes before b in stream order.
func studyStreamBefore(a, b StudyCandidate, boost bool) bool {
	if boost && a.boosted() != b.boosted() {
		return a.boosted()
	}
	if boost && a.boosted() && a.boostCount() != b.boostCount() {
		return a.boostCount() > b.boostCount()
	}
	if a.frequency() != b.frequency() {
		return a.frequency() > b.frequency()
	}
	return studyKeyBefore(a, b)
}

// studyKeyBefore orders by kind (words before Idioms) and key, the
// deterministic tie-break.
func studyKeyBefore(a, b StudyCandidate) bool {
	if a.ItemKind != b.ItemKind {
		return a.ItemKind < b.ItemKind
	}
	if a.CanonicalLemma != b.CanonicalLemma {
		return a.CanonicalLemma < b.CanonicalLemma
	}
	if a.UPOS != b.UPOS {
		return a.UPOS < b.UPOS
	}
	if a.LexicalDiscriminator != b.LexicalDiscriminator {
		return a.LexicalDiscriminator < b.LexicalDiscriminator
	}
	return a.Language < b.Language
}

// StudyMaterialState is where a word's Study material is in its preparation.
type StudyMaterialState string

const (
	// StudyMaterialPending is a word whose material is being prepared.
	StudyMaterialPending StudyMaterialState = "pending"
	// StudyMaterialReady is a word whose sentence, Gloss and translation exist.
	StudyMaterialReady StudyMaterialState = "ready"
	// StudyMaterialUnpreparable is a word with no usable sentence or no
	// defensible meaning. It is skipped in the stream with its reason.
	StudyMaterialUnpreparable StudyMaterialState = "unpreparable"
)

// StudyPreparationTargets returns the words, in stream order, whose material
// should exist now: the first StudyPreparationWindow candidates that are not
// unpreparable. A word whose material is already ready or pending still
// counts toward the window, so preparation stays about one day ahead and no
// further. states holds each word's current material state; a missing word has
// no material yet. ordered must already be in stream order.
func StudyPreparationTargets(ordered []StudyCandidate, states map[StudyItem]StudyMaterialState) []StudyCandidate {
	targets := make([]StudyCandidate, 0, StudyPreparationWindow)
	for _, candidate := range ordered {
		if states[candidate.StudyItem] == StudyMaterialUnpreparable {
			continue
		}
		targets = append(targets, candidate)
		if len(targets) == StudyPreparationWindow {
			break
		}
	}
	return targets
}

// StudyMaterial is one word's Study material: the introducing study context
// (its Representative sentence and source occurrence), the contextual Gloss,
// the sentence translation and any English target alignment. The evidence
// fields are empty until the material is ready, and an unpreparable word keeps
// only its reason.
type StudyMaterial struct {
	ID, OwnerID string
	StudyItem
	State          StudyMaterialState
	OmissionReason string
	// Source occurrence: the Book, analysis run, corpus, sentence and token the
	// Representative sentence was taken from.
	BookID, AnalysisRunID, CorpusID string
	SentenceOrdinal, TokenOrdinal   int64
	Sentence, TargetWord            string
	Gloss                           string
	SentenceTranslation             string
	// SentenceTranslationTargets is the English target alignment, if any.
	SentenceTranslationTargets []string
	// Cited is the dictionary senses the Gloss rests on and the pin they were
	// made against. It is empty for modern-language material.
	Cited StudyCitations
	// Passage is the translated passage the Representative sentence sits in. It
	// is empty when no passage applies.
	Passage StudyPassage
	// MatchedTokenOrdinals are an Idiom's matched words, as token ordinals in
	// the sentence in sentence order. It is empty for a word.
	MatchedTokenOrdinals []int
	// EvidenceFingerprint identifies the occurrences the attempt to prepare an
	// unpreparable word's material saw; see StudyEvidenceFingerprint.
	EvidenceFingerprint string
	// OmissionDictionaryPin is the dictionary pin a no-dictionary-entry omission
	// was recorded against. It is empty for any other material.
	OmissionDictionaryPin string
	CreatedAt, UpdatedAt  time.Time
}

// RetriesAgainstDictionary reports whether a no-dictionary-entry omission is
// worth trying again: the loaded dictionary's pin for the language is known and
// is not the one the omission was recorded against (#2775). A meaning or
// sentence-quality omission is never retried here, and an unknown pin never
// retries, since without a dictionary there is nothing new to ask.
func (m StudyMaterial) RetriesAgainstDictionary(currentPin string) bool {
	currentPin = strings.TrimSpace(currentPin)
	return m.State == StudyMaterialUnpreparable && m.OmissionReason == StudyOmissionNoDictionaryEntry &&
		currentPin != "" && m.OmissionDictionaryPin != currentPin
}

// StudyCitations are the dictionary senses a source-evidence Gloss rests on
// (ADR 0138 §2): the source's native sense IDs, the native ID of the entry each
// sense belongs to and the pinned dictionary revision they were made against.
// SenseIDs and EntryIDs are parallel: EntryIDs[i] is the entry of SenseIDs[i],
// because a native sense ID may be shared by stacked homograph entries and the
// sense alone would not say which one was cited. They are empty for a
// modern-language word, whose Learned meaning is a Gloss alone.
type StudyCitations struct {
	SenseIDs, EntryIDs []string
	// DictionaryPin is the pinned dictionary revision. It is set exactly when
	// SenseIDs is not empty.
	DictionaryPin string
}

// Clone returns citations that share no slice with the receiver.
func (c StudyCitations) Clone() StudyCitations {
	c.SenseIDs, c.EntryIDs = append([]string{}, c.SenseIDs...), append([]string{}, c.EntryIDs...)
	return c
}

// Empty reports whether nothing is cited.
func (c StudyCitations) Empty() bool {
	return len(c.SenseIDs) == 0 && strings.TrimSpace(c.DictionaryPin) == ""
}

// Valid reports whether the citations are all or nothing: a pin comes with at
// least one cited sense, and every cited sense names its entry.
func (c StudyCitations) Valid() bool {
	if (len(c.SenseIDs) > 0) != (strings.TrimSpace(c.DictionaryPin) != "") {
		return false
	}
	return len(c.EntryIDs) == len(c.SenseIDs)
}

// ReadyValid reports whether the citations may stand on a ready row. Empty
// citations do (a modern-language word); cited ones are valid and name no blank
// entry or sense.
func (c StudyCitations) ReadyValid() bool {
	if !c.Valid() {
		return false
	}
	return c.Empty() || (!slices.Contains(c.EntryIDs, "") && !slices.Contains(c.SenseIDs, ""))
}

// Has reports whether the sense of the entry is cited.
func (c StudyCitations) Has(entryID, senseID string) bool {
	for i, id := range c.SenseIDs {
		if id == senseID && i < len(c.EntryIDs) && c.EntryIDs[i] == entryID {
			return true
		}
	}
	return false
}

// EntrySet is the distinct entries the cited senses belong to, in citation order.
func (c StudyCitations) EntrySet() []string {
	var entries []string
	for _, id := range c.EntryIDs {
		if !slices.Contains(entries, id) {
			entries = append(entries, id)
		}
	}
	return entries
}

// StudyPassage is the translated passage a Representative sentence sits in: its
// text, the cited sections as one range label and the pin of the translation it
// came from. The three are set together.
type StudyPassage struct {
	Text, Citation, TranslationPin string
}

// Empty reports whether no passage applies.
func (p StudyPassage) Empty() bool { return strings.TrimSpace(p.Text) == "" }

// Valid reports whether the passage is all or nothing.
func (p StudyPassage) Valid() bool {
	text := strings.TrimSpace(p.Text) != ""
	return text == (strings.TrimSpace(p.Citation) != "") && text == (strings.TrimSpace(p.TranslationPin) != "")
}

// StudyMaterialCounts is what the Study page reports about preparation: the
// words whose material is ready, the words being prepared, and the words
// skipped as unpreparable.
type StudyMaterialCounts struct {
	Ready, Preparing, Unpreparable int
}

// StudyMaterialReadyContent is the prepared content recorded when a word's
// material becomes ready.
type StudyMaterialReadyContent struct {
	BookID, AnalysisRunID, CorpusID string
	SentenceOrdinal, TokenOrdinal   int64
	Sentence, TargetWord            string
	Gloss                           string
	SentenceTranslation             string
	SentenceTranslationTargets      []string
	// Cited and Passage are empty for modern-language material. A
	// source-evidence word's material carries its citations and pin, and its
	// passage when one applies.
	Cited   StudyCitations
	Passage StudyPassage
	// MatchedTokenOrdinals are an Idiom's matched words in the sentence; a
	// word has none.
	MatchedTokenOrdinals []int
}

// Complete reports whether the sentence, target word, Gloss and translation
// are all present: material is never ready without them. Its citations and
// passage are each all or nothing.
func (c StudyMaterialReadyContent) Complete() bool {
	for _, text := range []string{c.Sentence, c.TargetWord, c.Gloss, c.SentenceTranslation} {
		if strings.TrimSpace(text) == "" {
			return false
		}
	}
	return c.Cited.ReadyValid() && c.Passage.Valid()
}

// Study material omission reasons. A sentence-quality omission and a meaning
// omission are terminal; a provider omission is retried and never recorded.
const (
	StudyOmissionSentenceQuality = "sentence-quality"
	StudyOmissionMeaning         = "meaning"
	// StudyOmissionLiteralUse is the omission of an Idiom whose sentence uses
	// its words literally (ADR 0137 "Idiom Study material"). It is terminal.
	StudyOmissionLiteralUse = "literal-use"
	// StudyOmissionNoDictionaryEntry is the omission of a source-evidence word
	// the configured dictionary has no entry for (ADR 0138 §1). It is terminal:
	// no provider is asked.
	StudyOmissionNoDictionaryEntry = "no-dictionary-entry"
)

// ErrInvalidStudyMaterial rejects Study material input that names no word or
// records no reason for an omission.
var ErrInvalidStudyMaterial = errors.New("study: invalid study material")

// IdiomCandidateCount is one Idiom headword's candidate count across the
// Corpus: the sum of its per-Book counts and the number of Books containing it.
type IdiomCandidateCount struct {
	Headword   string
	Candidates int64
	Books      int64
	// InCurrentReading is set when the Current reading contains the headword.
	InCurrentReading bool
	// CurrentReadingCandidates is the headword's candidate count within the
	// Current reading, zero when it is not there.
	CurrentReadingCandidates int64
}

// IdiomStudyCandidates are the Idioms of a language as Study candidates: each
// headword is an Idiom item whose Personal occurrences are its candidate count
// across the Corpus (ADR 0137 §4).
func IdiomStudyCandidates(language string, counts []IdiomCandidateCount) []StudyCandidate {
	candidates := make([]StudyCandidate, 0, len(counts))
	for _, count := range counts {
		if count.Candidates <= 0 {
			continue
		}
		candidates = append(candidates, StudyCandidate{
			StudyItem: IdiomItem(language, count.Headword), InCurrentReading: count.InCurrentReading, PersonalOccurrences: count.Candidates,
			CurrentReadingOccurrences: count.CurrentReadingCandidates,
		})
	}
	return candidates
}

// CorpusIdiomCounts is the Corpus sum of Idiom candidate counts. PendingBooks
// counts the Books whose counts are not ready yet: while it is above zero a
// headword missing from Counts may only be waiting, not have no candidates.
type CorpusIdiomCounts struct {
	Counts       []IdiomCandidateCount
	PendingBooks int64
}
