package domain

// StudyOccurrenceRef names one analyzed occurrence: the Book, analysis run,
// corpus, sentence and token a Study sentence was taken from. It is plain
// data, not a foreign key, so a replaced analysis never deletes a sentence the
// learner has been shown (ADR 0134 §2).
type StudyOccurrenceRef struct {
	BookID, AnalysisRunID, CorpusID string
	SentenceOrdinal, TokenOrdinal   int64
}

// Source is the occurrence the material's sentence was taken from.
func (m StudyMaterial) Source() StudyOccurrenceRef {
	return StudyOccurrenceRef{
		BookID: m.BookID, AnalysisRunID: m.AnalysisRunID, CorpusID: m.CorpusID,
		SentenceOrdinal: m.SentenceOrdinal, TokenOrdinal: m.TokenOrdinal,
	}
}

// StudyEvidenceAction is what a word's progress needs once its introducing
// occurrence is checked against current evidence (ADR 0134 §6).
type StudyEvidenceAction string

const (
	// StudyEvidenceNone leaves the word as it is.
	StudyEvidenceNone StudyEvidenceAction = ""
	// StudyEvidenceReintroduce replaces the word's Study material: its
	// introducing occurrence no longer resolves to its identity, but another
	// occurrence does. The FSRS state is kept.
	StudyEvidenceReintroduce StudyEvidenceAction = "reintroduce"
	// StudyEvidenceWithdraw withdraws the word: no valid occurrence of its
	// identity remains in the library. Its progress is kept.
	StudyEvidenceWithdraw StudyEvidenceAction = "withdraw"
	// StudyEvidenceReturn returns a withdrawn word whose introducing
	// occurrence resolves to its identity again.
	StudyEvidenceReturn StudyEvidenceAction = "return"
	// StudyEvidenceReturnReintroduce returns a withdrawn word whose evidence
	// reappeared elsewhere, and re-introduces it there.
	StudyEvidenceReturnReintroduce StudyEvidenceAction = "return-reintroduce"
)

// StudyEvidence is what is known about one word in study when it is checked:
// whether it is withdrawn, the occurrence its material was taken from, and the
// occurrences that resolve to its identity under current evidence (confirmed
// Lemma corrections, Occurrence exclusions and the published analysis).
type StudyEvidence struct {
	Withdrawn bool
	Source    StudyOccurrenceRef
	// Valid lists the identity's occurrences under current evidence.
	Valid []StudyOccurrenceRef
	// CurrentUnpreparable reports that the word's current Study material was
	// found unpreparable: a withdrawn word does not return into material that
	// already failed, so an unpreparable record never loops through the
	// provider.
	CurrentUnpreparable bool
}

// ReviewStudyEvidence decides what a word's introducing occurrence requires. A
// word whose introducing occurrence still resolves to its identity is left
// alone. One that no longer does is re-introduced when another occurrence is
// valid, and withdrawn when none is. A withdrawn word returns when evidence
// reappears. The identity a correction points to is never put into study here:
// this only acts on words already in study.
func ReviewStudyEvidence(e StudyEvidence) StudyEvidenceAction {
	resolves := false
	for _, occurrence := range e.Valid {
		if occurrence == e.Source {
			resolves = true
			break
		}
	}
	switch {
	case e.Withdrawn && (len(e.Valid) == 0 || e.CurrentUnpreparable):
		return StudyEvidenceNone
	case e.Withdrawn && resolves:
		return StudyEvidenceReturn
	case e.Withdrawn:
		return StudyEvidenceReturnReintroduce
	case len(e.Valid) == 0:
		return StudyEvidenceWithdraw
	case resolves:
		return StudyEvidenceNone
	default:
		return StudyEvidenceReintroduce
	}
}
