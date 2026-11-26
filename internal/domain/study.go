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

// StudyIdentity is the full vocabulary identity Study progress and Study
// material are owned by: learner (the caller's owner), study language, lemma,
// part of speech and lexical discriminator (ADR 0134 §2).
type StudyIdentity struct {
	Language, CanonicalLemma, UPOS, LexicalDiscriminator string
}

// StudyCandidate is one effective vocabulary identity that Study could offer
// as a new word. PersonalOccurrences is its total across the learner's
// currently analyzed Books in the study language. Known and Reserved
// vocabulary are deliberately not part of a candidate: Study never consults
// them (ADR 0134 §3).
type StudyCandidate struct {
	StudyIdentity
	InCurrentReading    bool
	PersonalOccurrences int64
}

// OrderStudyStream returns the candidates in stream order: identities that
// occur in the Current reading first, then all others; within each group by
// Personal analyzed-Book occurrences descending, then by lemma, part of speech
// and discriminator so the order is deterministic. The input is not modified.
func OrderStudyStream(candidates []StudyCandidate) []StudyCandidate {
	ordered := append([]StudyCandidate{}, candidates...)
	sort.SliceStable(ordered, func(i, j int) bool {
		a, b := ordered[i], ordered[j]
		if a.InCurrentReading != b.InCurrentReading {
			return a.InCurrentReading
		}
		if a.PersonalOccurrences != b.PersonalOccurrences {
			return a.PersonalOccurrences > b.PersonalOccurrences
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
	})
	return ordered
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
func StudyPreparationTargets(ordered []StudyCandidate, states map[StudyIdentity]StudyMaterialState) []StudyCandidate {
	targets := make([]StudyCandidate, 0, StudyPreparationWindow)
	for _, candidate := range ordered {
		if states[candidate.StudyIdentity] == StudyMaterialUnpreparable {
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
	StudyIdentity
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
	// StudyOmissionNoDictionaryEntry is the omission of a source-evidence word
	// the configured dictionary has no entry for (ADR 0138 §1). It is terminal:
	// no provider is asked.
	StudyOmissionNoDictionaryEntry = "no-dictionary-entry"
)

// ErrInvalidStudyMaterial rejects Study material input that names no word or
// records no reason for an omission.
var ErrInvalidStudyMaterial = errors.New("study: invalid study material")
