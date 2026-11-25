package domain

import (
	"errors"
	"sort"
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
	CreatedAt, UpdatedAt       time.Time
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
}

// Study material omission reasons. A sentence-quality omission and a meaning
// omission are terminal; a provider omission is retried and never recorded.
const (
	StudyOmissionSentenceQuality = "sentence-quality"
	StudyOmissionMeaning         = "meaning"
)

// ErrInvalidStudyMaterial rejects Study material input that names no word or
// records no reason for an omission.
var ErrInvalidStudyMaterial = errors.New("study: invalid study material")
