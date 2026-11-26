package domain

import (
	"errors"
	"strings"
	"time"
)

// Fluency is the ungraded third phase of Study (ADR 0134 §7, spec #2691): it
// shows mature words in other sentences from the library. Nothing here reads
// or writes a word's schedule beyond deciding whether it is mature.

const (
	// FluencyMaturityDays is the scheduled interval, in days, from which a
	// word is mature.
	FluencyMaturityDays = 21
	// FluencyContextsPerWord is how many Fluency contexts a mature word has at
	// most.
	FluencyContextsPerWord = 5
)

// IsStudyMature reports whether the word is mature: its current scheduled
// interval is at least FluencyMaturityDays. A lapse puts the word into
// relearning with a short interval, so it loses maturity until it is on a
// day-scale schedule of that length again. A withdrawn word is never mature.
func IsStudyMature(p StudyProgress) bool {
	return !p.Withdrawn && p.Memory.State == StudyStateReview && p.Memory.ScheduledDays >= FluencyMaturityDays
}

// FluencyUnlocked reports whether Fluency is reachable: the finish line is
// reached and no New-word batch on offer awaits an answer. A batch on offer
// must be taken or declined, including one offered after an earlier batch was
// taken. Once the learner has declined, Today offers no more: a batch still
// takeable then no longer holds Fluency back, and the learner may take it from
// inside Fluency. It is the one place the page and the Fluency response read
// the unlock rule.
func FluencyUnlocked(today StudyToday, ledger StudyBatchLedger, offerOnHand bool) bool {
	return today.Finished && (!offerOnHand || ledger.Declined > 0)
}

// FluencyAgreement is whether a Fluency context uses the word's learned
// meaning.
type FluencyAgreement string

const (
	// FluencyAgrees is a context whose contextual Gloss is the learned
	// meaning. Only these are shown.
	FluencyAgrees FluencyAgreement = "agrees"
	// FluencyDisagrees is a context that does not use the learned meaning. It
	// is kept, with its reason, Gloss and translation, so the occurrence is not
	// asked about again and as evidence of the word's other uses; it is never
	// shown.
	FluencyDisagrees FluencyAgreement = "disagrees"
)

// FluencyReason is why a Fluency context does or does not use the word's
// learned meaning. Only FluencyReasonSameMeaning agrees.
type FluencyReason string

const (
	FluencyReasonSameMeaning      FluencyReason = "same-meaning"
	FluencyReasonDifferentMeaning FluencyReason = "different-meaning"
	FluencyReasonIdiomatic        FluencyReason = "idiomatic"
	// FluencyReasonNotThisLemma is evidence only: it raises no Lemma review
	// flag.
	FluencyReasonNotThisLemma FluencyReason = "not-this-lemma"
	FluencyReasonUndefendable FluencyReason = "undefendable"
)

// ParseFluencyReason reads a reason from its stored or provider name.
func ParseFluencyReason(name string) (FluencyReason, bool) {
	switch reason := FluencyReason(name); reason {
	case FluencyReasonSameMeaning, FluencyReasonDifferentMeaning, FluencyReasonIdiomatic, FluencyReasonNotThisLemma, FluencyReasonUndefendable:
		return reason, true
	}
	return "", false
}

// Agreement is what the reason means for the learned meaning.
func (r FluencyReason) Agreement() FluencyAgreement {
	if r == FluencyReasonSameMeaning {
		return FluencyAgrees
	}
	return FluencyDisagrees
}

// FluencyForm is the form a word takes in one occurrence: its surface form and
// morphological features, and whether it is a separable verb split from its
// prefix.
type FluencyForm struct {
	Surface  string
	Features string
	Split    bool
}

// Key is the form's identity apart from its surface: its morphological
// features and whether it is split.
func (f FluencyForm) Key() string {
	if f.Split {
		return f.Features + "|split"
	}
	return f.Features
}

func (f FluencyForm) surfaceKey() string { return strings.ToLower(strings.TrimSpace(f.Surface)) }

// FluencyCandidate is one occurrence of a mature word that could become a
// Fluency context.
type FluencyCandidate struct {
	// Key identifies the occurrence, so the ranking is deterministic.
	Key              string
	Form             FluencyForm
	InCurrentReading bool
	// Quality is the sentence's score under the existing sentence-quality
	// rules, higher first.
	Quality float64
}

// RankFluencyCandidates orders candidates to prefer a different form: each
// pick is, in turn, the candidate that adds a new set of morphological
// features (a split separable verb counts as one), then one that adds only a
// new surface form, then one of a form already chosen, so different sentences
// of the same form come after. The word's introducing form counts as chosen.
// Ties go to the Current reading, then to the higher sentence quality, then to
// the key. The input is not modified.
func RankFluencyCandidates(introduced FluencyForm, candidates []FluencyCandidate) []FluencyCandidate {
	remaining := append([]FluencyCandidate{}, candidates...)
	seenKeys := map[string]bool{introduced.Key(): true}
	seenSurfaces := map[string]bool{introduced.surfaceKey(): true}
	ranked := make([]FluencyCandidate, 0, len(remaining))
	for len(remaining) > 0 {
		best := 0
		for i := 1; i < len(remaining); i++ {
			if fluencyCandidateBefore(remaining[i], remaining[best], seenKeys, seenSurfaces) {
				best = i
			}
		}
		pick := remaining[best]
		ranked = append(ranked, pick)
		seenKeys[pick.Form.Key()] = true
		seenSurfaces[pick.Form.surfaceKey()] = true
		remaining = append(remaining[:best], remaining[best+1:]...)
	}
	return ranked
}

func fluencyNovelty(c FluencyCandidate, seenKeys, seenSurfaces map[string]bool) int {
	switch {
	case !seenKeys[c.Form.Key()]:
		return 2
	case !seenSurfaces[c.Form.surfaceKey()]:
		return 1
	}
	return 0
}

func fluencyCandidateBefore(a, b FluencyCandidate, seenKeys, seenSurfaces map[string]bool) bool {
	if na, nb := fluencyNovelty(a, seenKeys, seenSurfaces), fluencyNovelty(b, seenKeys, seenSurfaces); na != nb {
		return na > nb
	}
	if a.InCurrentReading != b.InCurrentReading {
		return a.InCurrentReading
	}
	if a.Quality != b.Quality {
		return a.Quality > b.Quality
	}
	return a.Key < b.Key
}

// FluencyContext is one stored Fluency context of a mature word: another
// sentence where the word has its learned meaning.
type FluencyContext struct {
	ID, OwnerID, ProgressID string
	Agreement               FluencyAgreement
	// Reason is why the context does or does not use the learned meaning.
	Reason FluencyReason
	// Source occurrence of the sentence.
	BookID, AnalysisRunID, CorpusID string
	SentenceOrdinal, TokenOrdinal   int64
	Sentence, TargetWord            string
	// Gloss is the contextual Gloss the provider gave this occurrence, and
	// SentenceTranslation its translation. A disagreeing context keeps both as
	// evidence of the word's other meanings.
	Gloss                      string
	SentenceTranslation        string
	SentenceTranslationTargets []string
	// Form is the form the context was chosen for.
	Form FluencyForm
	// Cited are the dictionary senses the context's Gloss cites, for a
	// source-evidence language; they are empty for a modern language.
	Cited     StudyCitations
	CreatedAt time.Time
	// LastResponseAt is when the learner last answered the context, zero when
	// never.
	LastResponseAt time.Time
	// Responses are the learner's answers to the context, oldest first. They
	// are the evidence the weighted rotation reads.
	Responses []FluencyResponseEvent
}

// FluencyResponseEvent is one answer to a Fluency context and when it was given.
type FluencyResponseEvent struct {
	Response FluencyResponse
	At       time.Time
}

// FluencyCard is a Fluency context with the word it belongs to.
type FluencyCard struct {
	Context  FluencyContext
	Progress StudyProgress
}

func studyIdentityBefore(a, b StudyIdentity) bool {
	if a.CanonicalLemma != b.CanonicalLemma {
		return a.CanonicalLemma < b.CanonicalLemma
	}
	if a.UPOS != b.UPOS {
		return a.UPOS < b.UPOS
	}
	return a.LexicalDiscriminator < b.LexicalDiscriminator
}

// FluencyResponse is the learner's answer to a Fluency card. Fluency is
// ungraded: a response is evidence and never changes Study progress.
type FluencyResponse string

const (
	// FluencyRecognised is a context the learner recognised.
	FluencyRecognised FluencyResponse = "recognised"
	// FluencyNotRecognised is a context the learner did not recognise.
	FluencyNotRecognised FluencyResponse = "not-recognised"
)

// ParseFluencyResponse reads a response from its stored or submitted name.
func ParseFluencyResponse(name string) (FluencyResponse, bool) {
	switch response := FluencyResponse(name); response {
	case FluencyRecognised, FluencyNotRecognised:
		return response, true
	}
	return "", false
}

// FluencyResponseRecord is a response ready to be stored.
type FluencyResponseRecord struct {
	ContextID   string
	Response    FluencyResponse
	RespondedAt time.Time
}

// FluencyContextRecord is a context ready to be stored. The store refuses an
// agreeing context beyond FluencyContextsPerWord for the word, and one for an
// occurrence it already holds.
type FluencyContextRecord struct {
	ProgressID                      string
	Agreement                       FluencyAgreement
	Reason                          FluencyReason
	BookID, AnalysisRunID, CorpusID string
	SentenceOrdinal, TokenOrdinal   int64
	Sentence, TargetWord            string
	Gloss, SentenceTranslation      string
	SentenceTranslationTargets      []string
	Form                            FluencyForm
	// Cited are empty for a modern language.
	Cited StudyCitations
}

// ErrFluencyContextNotStored is returned when a context was not stored because
// the word already has its contexts or the occurrence was already assessed.
var ErrFluencyContextNotStored = errors.New("study: fluency context was not stored")

// FluencyContextsNeeded reports whether a mature word still lacks contexts:
// fewer than FluencyContextsPerWord agree. Words whose candidates ran out stay
// listed; the preparation job asks about each occurrence only once.
func FluencyContextsNeeded(progress []StudyProgress, contexts []FluencyContext) []StudyProgress {
	agreeing := map[string]int{}
	for _, context := range contexts {
		if context.Agreement == FluencyAgrees {
			agreeing[context.ProgressID]++
		}
	}
	var needed []StudyProgress
	for _, item := range progress {
		if IsStudyMature(item) && agreeing[item.ID] < FluencyContextsPerWord {
			needed = append(needed, item)
		}
	}
	return needed
}
