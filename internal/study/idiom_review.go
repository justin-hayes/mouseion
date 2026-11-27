package study

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/idiom"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

// idiomReviewStore is what checking an Idiom against current evidence reads and
// writes (ADR 0137 §5). The PostgreSQL store satisfies it.
type idiomReviewStore interface {
	ListStudyIdiomCandidates(ctx context.Context, owner, language, indexVersion string) ([]domain.StudyCandidate, int64, error)
	GetStudyMaterial(ctx context.Context, owner, id string) (domain.StudyMaterial, error)
	ListCurrentStudyMaterial(ctx context.Context, owner, language string) ([]domain.StudyMaterial, error)
	ListStudyEffectiveTokens(ctx context.Context, owner, book, run, corpus string, ordinals []int64) (map[int64][]analyzer.Token, error)
	SupersedeStudyMaterial(ctx context.Context, owner, materialID string) error
	SetStudyWordWithdrawn(ctx context.Context, owner, progressID string, withdrawn bool, now time.Time) error
	ListStudyIdiomSentenceRefs(ctx context.Context, owner, language, headword string, words []string) ([]persistence.StudyIdiomSentenceRef, error)
}

// ReviewIdiom checks one Idiom in study against current evidence and applies
// what that requires, reporting whether its progress or material changed
// (ADR 0137 §5). Corrections never create reviews, and the Idiom's progress is
// never deleted.
//
//   - With no candidate left in the Corpus the Idiom is withdrawn. A dictionary
//     or reviewed-list refresh that drops the headword rebuilds the counts
//     without it, so it ends here too. While a Book's counts are not ready the
//     Book is not counted as having none: the Idiom is not withdrawn.
//   - A withdrawn Idiom returns, on new material, when a candidate reappears,
//     unless it was withdrawn because its material could not be prepared.
//   - Otherwise its introducing sentence must still match the headword on the
//     effective lemmas, after Lemma corrections, Occurrence exclusions and
//     Analysis updates; if it does not, the Idiom is re-introduced.
//
// Without an idiom index there is no evidence either way and nothing changes.
func (s *Service) ReviewIdiom(ctx context.Context, owner string, word domain.StudyProgress, now time.Time) (bool, error) {
	if s.idioms == nil || s.idioms.Index == nil {
		return false, nil
	}
	return reviewIdiom(ctx, s.store, s.idioms, s.idioms.active(word.Language), owner, word, now)
}

func reviewIdiom(ctx context.Context, store idiomReviewStore, avoidance *IdiomAvoidance, active bool, owner string, word domain.StudyProgress, now time.Time) (bool, error) {
	if !word.IsIdiom() {
		return false, nil
	}
	candidates, pending, err := store.ListStudyIdiomCandidates(ctx, owner, word.Language, avoidance.IndexVersion)
	if err != nil {
		return false, err
	}
	hasCandidate := false
	for _, candidate := range candidates {
		if candidate.StudyItem == word.StudyItem {
			hasCandidate = true
			break
		}
	}
	switch {
	case !hasCandidate && pending > 0:
		// A Book's counts are not ready: it may hold candidates, so it does not
		// count as having none. Nothing changes until the counts are rebuilt.
		return false, nil
	case !hasCandidate:
		if word.Withdrawn {
			return false, nil
		}
		return true, ignoreMissing(store.SetStudyWordWithdrawn(ctx, owner, word.ID, true, now))
	case !active:
		// With the language's switch off no Idiom material is prepared, so a
		// replacement would never arrive: the Idiom keeps its reviews as they are.
		return false, nil
	case word.Withdrawn:
		return returnIdiom(ctx, store, avoidance, owner, word, now)
	}
	material, err := store.GetStudyMaterial(ctx, owner, word.MaterialID)
	if errors.Is(err, domain.ErrNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	matches, err := introducingSentenceMatches(ctx, store, avoidance, owner, word, material)
	if err != nil || matches {
		return false, err
	}
	return true, ignoreMissing(store.SupersedeStudyMaterial(ctx, owner, word.MaterialID))
}

// returnIdiom brings a withdrawn Idiom back when evidence reappears, and
// replaces its material so it is re-introduced in a current sentence. One
// withdrawn because its material was unpreparable stays withdrawn until its
// sentences differ from the ones that attempt saw.
func returnIdiom(ctx context.Context, store idiomReviewStore, avoidance *IdiomAvoidance, owner string, word domain.StudyProgress, now time.Time) (bool, error) {
	all, err := store.ListCurrentStudyMaterial(ctx, owner, word.Language)
	if err != nil {
		return false, err
	}
	var current domain.StudyMaterial
	for _, item := range all {
		if item.StudyItem == word.StudyItem {
			current = item
			break
		}
	}
	if current.State == domain.StudyMaterialUnpreparable {
		// It returns only on evidence the failed attempt did not see.
		seen, err := idiomEvidence(ctx, store, avoidance, owner, word.StudyItem)
		if err != nil || current.EvidenceFingerprint == domain.StudyEvidenceFingerprint(seen) {
			return false, err
		}
	}
	if err := store.SetStudyWordWithdrawn(ctx, owner, word.ID, false, now); err != nil {
		return false, ignoreMissing(err)
	}
	if current.ID == "" {
		return true, nil
	}
	return true, ignoreMissing(store.SupersedeStudyMaterial(ctx, owner, current.ID))
}

// introducingSentenceMatches reports whether the Idiom's introducing sentence,
// read with the effective lemmas, still holds a candidate of its headword. A
// sentence that is no longer in a current analysis does not.
func introducingSentenceMatches(ctx context.Context, store idiomReviewStore, avoidance *IdiomAvoidance, owner string, word domain.StudyProgress, material domain.StudyMaterial) (bool, error) {
	source := material.Source()
	tokens, err := store.ListStudyEffectiveTokens(ctx, owner, source.BookID, source.AnalysisRunID, source.CorpusID, []int64{source.SentenceOrdinal})
	if err != nil {
		return false, err
	}
	sentence := tokens[source.SentenceOrdinal]
	if len(sentence) == 0 {
		return false, nil
	}
	headword, _ := word.Headword()
	reader := &idiomReader{avoidance: avoidance, index: idiom.NewCachedIndex(avoidance.Index), language: canonicalization.NormalizeLanguage(word.Language)}
	entry, found, err := reader.entry(ctx, headword)
	if err != nil || !found {
		// A refresh dropped the headword: its Corpus counts say so once they
		// are rebuilt, and the Idiom is withdrawn then. Until then there is
		// no sentence to compare it with.
		return true, err
	}
	// The sentence must still be in a current analysis of a Book in My Books.
	refs, err := store.ListStudyIdiomSentenceRefs(ctx, owner, reader.language, entry.Headword, entry.Words)
	if err != nil {
		return false, err
	}
	if !slices.ContainsFunc(refs, func(ref persistence.StudyIdiomSentenceRef) bool {
		return ref.BookID == source.BookID && ref.AnalysisRunID == source.AnalysisRunID && ref.SentenceOrdinal == source.SentenceOrdinal
	}) {
		return false, nil
	}
	candidates, err := idiom.CandidatesInSentence(ctx, reader.index, avoidance.Parameters, reader.language, sentence)
	if err != nil {
		return false, fmt.Errorf("study: idiom candidates: %w", err)
	}
	for _, candidate := range candidates {
		if strings.EqualFold(candidate.Entry.Headword, headword) {
			return true, nil
		}
	}
	return false, nil
}

// idiomEvidence names the candidate sentences the Idiom has now, as the
// evidence an unpreparable attempt saw and a later check compares with.
func idiomEvidence(ctx context.Context, store interface {
	ListStudyIdiomSentenceRefs(ctx context.Context, owner, language, headword string, words []string) ([]persistence.StudyIdiomSentenceRef, error)
}, avoidance *IdiomAvoidance, owner string, item domain.StudyItem) ([]domain.StudyOccurrenceRef, error) {
	headword, _ := item.Headword()
	reader := &idiomReader{avoidance: avoidance, index: idiom.NewCachedIndex(avoidance.Index), language: canonicalization.NormalizeLanguage(item.Language)}
	entry, found, err := reader.entry(ctx, headword)
	if err != nil || !found {
		return nil, err
	}
	refs, err := store.ListStudyIdiomSentenceRefs(ctx, owner, reader.language, entry.Headword, entry.Words)
	if err != nil {
		return nil, err
	}
	seen := make([]domain.StudyOccurrenceRef, len(refs))
	for i, ref := range refs {
		seen[i] = domain.StudyOccurrenceRef{BookID: ref.BookID, AnalysisRunID: ref.AnalysisRunID, CorpusID: ref.CorpusID, SentenceOrdinal: ref.SentenceOrdinal}
	}
	return seen, nil
}
