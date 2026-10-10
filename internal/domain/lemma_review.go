package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"
)

type LemmaReviewIdentity struct {
	Language, CanonicalLemma, UPOS string
}

// LemmaReviewIdentityState is everything a lemma review preview reads about one
// identity its proposal moves: its effective count in the reviewed Book now and
// after the proposal, its count across the learner's other currently analyzed
// Books, and whether it is Known or Reserved.
type LemmaReviewIdentityState struct {
	Identity            LemmaReviewIdentity
	InBook, AfterInBook int64
	OtherBooks          int64
	Known, Reserved     bool
}

// LemmaReviewPreview is what previewing one proposal reads: the states of the
// identities it moves, and the fingerprint that binds a confirm to exactly that
// state.
type LemmaReviewPreview struct {
	States      []LemmaReviewIdentityState
	Fingerprint string
}

// LemmaReviewProposal is one previewed lemma decision: an action ("correct",
// "keep" or "exclude") applied to the selected occurrences of one observed
// surface form in one Book.
type LemmaReviewProposal struct {
	OwnerID, BookID, Language, Surface string
	Action, Lemma                      string
	Occurrences                        []LemmaReviewOccurrence
	NormalizationProfile               string
	NormalizationVersion               string
}

// Decisions returns the decision each selected occurrence receives: keep
// restores the analyzer lemma, exclude removes the occurrence, and correct sets
// Lemma.
func (p LemmaReviewProposal) Decisions() []LemmaReviewDecision {
	decisions := make([]LemmaReviewDecision, 0, len(p.Occurrences))
	for _, occurrence := range p.Occurrences {
		decision := LemmaReviewDecision{
			Occurrence: occurrence, CanonicalLemma: p.Lemma, Excluded: p.Action == "exclude",
			NormalizationProfile: p.NormalizationProfile, NormalizationVersion: p.NormalizationVersion,
		}
		switch p.Action {
		case "keep":
			decision.CanonicalLemma = occurrence.CanonicalLemma
		case "exclude":
			decision.CanonicalLemma = ""
		}
		decisions = append(decisions, decision)
	}
	return decisions
}

// Resolution is the outcome a decision records on the occurrence's review flag:
// "exclude", "keep" when the analyzer lemma stands, or "correct".
func (d LemmaReviewDecision) Resolution() string {
	switch {
	case d.Excluded:
		return "exclude"
	case d.CanonicalLemma == d.Occurrence.CanonicalLemma:
		return "keep"
	default:
		return "correct"
	}
}

// StoredCorrection is the corrected lemma a decision persists. Exclusions and
// keeps store none, because a stored correction records only a change to the
// analyzer lemma.
func (d LemmaReviewDecision) StoredCorrection() string {
	if d.Excluded || d.CanonicalLemma == d.Occurrence.CanonicalLemma {
		return ""
	}
	return d.CanonicalLemma
}

// LemmaReviewFingerprint binds a previewed proposal to exactly the state it was
// previewed against: the surface form's occurrences, the states of the
// identities the proposal moves, and the proposal's action, lemma and selected
// occurrences. Confirming is rejected as stale whenever any of them changed.
func LemmaReviewFingerprint(proposal LemmaReviewProposal, occurrences []LemmaReviewOccurrence, states []LemmaReviewIdentityState) string {
	h := sha256.New()
	for _, occurrence := range occurrences {
		_, _ = h.Write([]byte(occurrenceFingerprintLine(occurrence) + "\n"))
	}
	lines := make([]string, 0, len(states))
	for _, state := range states {
		lines = append(lines, strings.Join([]string{
			state.Identity.Language, state.Identity.CanonicalLemma, state.Identity.UPOS,
			strconv.FormatInt(state.InBook, 10), strconv.FormatInt(state.AfterInBook, 10), strconv.FormatInt(state.OtherBooks, 10),
			strconv.FormatBool(state.Known), strconv.FormatBool(state.Reserved),
		}, "\x00"))
	}
	sort.Strings(lines)
	for _, line := range lines {
		_, _ = h.Write([]byte(line + "\n"))
	}
	_, _ = h.Write([]byte(proposal.Action + "\x00" + proposal.Lemma + "\n"))
	selected := make([]string, 0, len(proposal.Occurrences))
	for _, occurrence := range proposal.Occurrences {
		selected = append(selected, occurrenceFingerprintLine(occurrence))
	}
	sort.Strings(selected)
	for _, line := range selected {
		_, _ = h.Write([]byte(line + "\n"))
	}
	return hex.EncodeToString(h.Sum(nil))
}

func occurrenceFingerprintLine(occurrence LemmaReviewOccurrence) string {
	return strings.Join([]string{
		occurrence.AnalysisRunID, occurrence.SourceDocumentID,
		strconv.FormatInt(occurrence.StartOffset, 10), strconv.FormatInt(occurrence.EndOffset, 10),
		occurrence.Surface, occurrence.RawLemma, occurrence.CanonicalLemma, occurrence.UPOS,
		occurrence.CorrectedLemma, strconv.FormatBool(occurrence.Excluded),
	}, "\x00")
}
