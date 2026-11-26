package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"
)

// StudyOccurrenceRef names one analyzed occurrence: the Book, analysis run,
// corpus, sentence and token a Study sentence was taken from. It is plain
// data, not a foreign key, so a replaced analysis never deletes a sentence the
// learner has been shown (ADR 0134 §2).
type StudyOccurrenceRef struct {
	BookID, AnalysisRunID, CorpusID string
	SentenceOrdinal, TokenOrdinal   int64
	// UnitID, StartOffset and EndOffset locate the token in the Book's
	// extracted unit. They are what an Analysis update carries decisions across
	// runs by (ADR 0126), so they find the same occurrence in a new run. They
	// are empty when unknown.
	UnitID                 string
	StartOffset, EndOffset int64
}

// sameRun reports that the other ref is this occurrence in the same analysis
// run: the same sentence and token ordinals.
func (r StudyOccurrenceRef) sameRun(o StudyOccurrenceRef) bool {
	return r.BookID == o.BookID && r.AnalysisRunID == o.AnalysisRunID && r.CorpusID == o.CorpusID &&
		r.SentenceOrdinal == o.SentenceOrdinal && r.TokenOrdinal == o.TokenOrdinal
}

// sameToken reports that the other ref is the same token of the same Book,
// wherever its run puts it.
func (r StudyOccurrenceRef) sameToken(o StudyOccurrenceRef) bool {
	return r.UnitID != "" && r.BookID == o.BookID && r.UnitID == o.UnitID && r.StartOffset == o.StartOffset && r.EndOffset == o.EndOffset
}

// StudyEvidenceFingerprint identifies a set of occurrences, whatever their
// order, so a word whose material could not be prepared is only tried again
// when its evidence is not what the failed attempt saw.
func StudyEvidenceFingerprint(refs []StudyOccurrenceRef) string {
	lines := make([]string, 0, len(refs))
	for _, ref := range refs {
		lines = append(lines, fmt.Sprintf("%s|%s|%s|%d|%d", ref.BookID, ref.AnalysisRunID, ref.CorpusID, ref.SentenceOrdinal, ref.TokenOrdinal))
	}
	slices.Sort(lines)
	sum := sha256.Sum256([]byte(strings.Join(lines, "\n")))
	return hex.EncodeToString(sum[:])
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
	// found unpreparable, and FailedEvidence is the fingerprint of the
	// occurrences that attempt saw. A withdrawn word is only tried again when
	// its evidence differs, so an unpreparable record never loops through the
	// provider.
	CurrentUnpreparable bool
	FailedEvidence      string
}

// StudyRelocation finds the word's introducing occurrence in a new analysis
// run: the same token of the same Book, which still resolves to the word's
// identity. ok is false when the occurrence is still in its own run or cannot
// be found.
func StudyRelocation(e StudyEvidence) (StudyOccurrenceRef, bool) {
	if slices.ContainsFunc(e.Valid, e.Source.sameRun) {
		return StudyOccurrenceRef{}, false
	}
	for _, occurrence := range e.Valid {
		if e.Source.sameToken(occurrence) {
			return occurrence, true
		}
	}
	return StudyOccurrenceRef{}, false
}

// ReviewStudyEvidence decides what a word's introducing occurrence requires. A
// word whose introducing occurrence still resolves to its identity is left
// alone. One that no longer does is re-introduced when another occurrence is
// valid, and withdrawn when none is. A withdrawn word returns when evidence
// reappears. The identity a correction points to is never put into study here:
// this only acts on words already in study.
func ReviewStudyEvidence(e StudyEvidence) StudyEvidenceAction {
	_, relocated := StudyRelocation(e)
	resolves := relocated || slices.ContainsFunc(e.Valid, e.Source.sameRun)
	switch {
	case e.Withdrawn && (len(e.Valid) == 0 || (e.CurrentUnpreparable && e.FailedEvidence == StudyEvidenceFingerprint(e.Valid))):
		return StudyEvidenceNone
	case e.Withdrawn && resolves && !e.CurrentUnpreparable:
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
