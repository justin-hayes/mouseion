package domain

import (
	"strings"
	"time"
	"unicode/utf8"
)

// StudyReportKind is what a Study report says is wrong with a word's Study
// material (ADR 0135 §1).
type StudyReportKind string

const (
	// StudyReportWrongWord says the occurrence belongs to another vocabulary
	// identity. The word is held while the report is open.
	StudyReportWrongWord StudyReportKind = "wrong_word"
	// StudyReportUnsuitableContext says the identity is right but the context
	// does not teach the word's own meaning. The word is not held; the
	// reported sentence is retired as its context.
	StudyReportUnsuitableContext StudyReportKind = "unsuitable_context"
)

// ParseStudyReportKind reads a report kind from its stored or submitted name.
func ParseStudyReportKind(name string) (StudyReportKind, bool) {
	switch kind := StudyReportKind(name); kind {
	case StudyReportWrongWord, StudyReportUnsuitableContext:
		return kind, true
	}
	return "", false
}

// StudyReportEnd is how a Study report ended (ADR 0135 §2 and §3).
type StudyReportEnd string

const (
	// StudyReportEndCorrected and StudyReportEndExcluded: a Lemma correction or
	// an Occurrence exclusion was confirmed on the reported occurrence.
	StudyReportEndCorrected StudyReportEnd = "corrected"
	StudyReportEndExcluded  StudyReportEnd = "excluded"
	// StudyReportEndAnalysisUpdate: an Analysis update gave the occurrence
	// another effective vocabulary identity.
	StudyReportEndAnalysisUpdate StudyReportEnd = "analysis_update"
	// StudyReportEndOccurrenceGone: the occurrence no longer exists, for
	// example because its Book left the library.
	StudyReportEndOccurrenceGone StudyReportEnd = "occurrence_gone"
	// StudyReportEndDismissed: the learner dismissed a Wrong-word report.
	StudyReportEndDismissed StudyReportEnd = "dismissed"
	// StudyReportEndUndone: the learner undid an Unsuitable-context report.
	StudyReportEndUndone StudyReportEnd = "undone"
	// StudyReportEndRetired: the retired context is no longer relevant.
	StudyReportEndRetired StudyReportEnd = "retired"
)

// Repair reports whether the end is one the repair of the occurrence caused:
// a confirmed correction or exclusion, an Analysis update or a vanished
// occurrence. Dismissing and undoing have their own store operations.
func (e StudyReportEnd) Repair() bool {
	switch e {
	case StudyReportEndCorrected, StudyReportEndExcluded, StudyReportEndAnalysisUpdate, StudyReportEndOccurrenceGone, StudyReportEndRetired:
		return true
	case StudyReportEndDismissed, StudyReportEndUndone:
	}
	return false
}

// StudyReportNoteMax is the longest note, in characters, a report keeps.
const StudyReportNoteMax = 500

// CleanStudyReportNote trims a submitted note. It reports false for a note
// longer than StudyReportNoteMax.
func CleanStudyReportNote(note string) (string, bool) {
	note = strings.TrimSpace(note)
	return note, utf8.RuneCountInString(note) <= StudyReportNoteMax
}

// StudyReport is a learner's report that a word's Study material is wrong
// (ADR 0135). It names the word, the material and the occurrence reported,
// copied from the material when the report was made. It is open until EndedAt.
type StudyReport struct {
	ID, OwnerID string
	StudyIdentity
	ProgressID, MaterialID string
	Kind                   StudyReportKind
	Note                   string
	// Source is the reported occurrence; it is also the retired (word,
	// occurrence) pair while an Unsuitable-context report is open.
	Source                     StudyOccurrenceRef
	Sentence, TargetWord       string
	Gloss, SentenceTranslation string
	// PreviousMaterialID, PreviousCorrectedAt and PreviousLearnedMeaning are
	// what Undo restores for an Unsuitable-context report: the word's material,
	// correction time and learned meaning when it was reported. They are zero
	// for a Wrong-word report, which changes none of them.
	PreviousMaterialID     string
	PreviousCorrectedAt    time.Time
	PreviousLearnedMeaning string
	CreatedAt, EndedAt     time.Time
	EndReason              StudyReportEnd
}

// Open reports whether the report has not ended.
func (r StudyReport) Open() bool { return r.EndedAt.IsZero() }

// Held reports whether the report holds its word: an open Wrong-word report.
func (r StudyReport) Held() bool { return r.Open() && r.Kind == StudyReportWrongWord }

// StudyReportRecord is a report ready to be stored. ExpectedReps is the number
// of grades the learner saw recorded when the card was shown, so a report of a
// card that has since been answered is rejected like a repeated grade
// (ErrStudyStale). The store copies the occurrence, sentence, Gloss and
// translation from the word's current material.
type StudyReportRecord struct {
	ProgressID   string
	ExpectedReps int64
	Kind         StudyReportKind
	Note         string
	CreatedAt    time.Time
}

// StudyHeldDue is the due time of a word after its Wrong-word report ended
// without a repair: it moves later by the time it was held, so the report
// never makes the word overdue (ADR 0135 §2).
func StudyHeldDue(due, reportedAt, now time.Time) time.Time {
	if !now.After(reportedAt) {
		return due
	}
	return due.Add(now.Sub(reportedAt))
}

// StudyHeldIdentities names the words the open Wrong-word reports hold. A New-
// word batch passes them over, including a word the report left with no Study
// progress.
func StudyHeldIdentities(reports []StudyReport) []StudyIdentity {
	var held []StudyIdentity
	for _, report := range reports {
		if report.Held() {
			held = append(held, report.StudyIdentity)
		}
	}
	return held
}

// StudyOccurrenceState is what current evidence says about one occurrence a
// report names: whether it is still in the current analysis of a Book in the
// library, and whether the learner excluded it or corrected its lemma.
type StudyOccurrenceState struct {
	Present, Excluded, Corrected bool
}

// EndReason is why a Wrong-word report of an occurrence that no longer resolves
// to the word's identity ends: the occurrence is gone, the learner excluded or
// corrected it, or an Analysis update gave it another identity.
func (s StudyOccurrenceState) EndReason() StudyReportEnd {
	switch {
	case !s.Present:
		return StudyReportEndOccurrenceGone
	case s.Excluded:
		return StudyReportEndExcluded
	case s.Corrected:
		return StudyReportEndCorrected
	}
	return StudyReportEndAnalysisUpdate
}
