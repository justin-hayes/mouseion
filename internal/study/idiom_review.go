package study

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/idiom"
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
	return reviewIdiom(ctx, s.store, s.idioms, owner, word, now)
}

func reviewIdiom(ctx context.Context, store idiomReviewStore, avoidance *IdiomAvoidance, owner string, word domain.StudyProgress, now time.Time) (bool, error) {
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
	case !hasCandidate && pending == 0:
		if word.Withdrawn {
			return false, nil
		}
		return true, ignoreMissing(store.SetStudyWordWithdrawn(ctx, owner, word.ID, true, now))
	case word.Withdrawn && hasCandidate:
		return returnIdiom(ctx, store, owner, word, now)
	case word.Withdrawn:
		// A Book's counts are not ready: it may hold candidates, so the Idiom
		// is not counted as having none, and it does not return on a guess.
		return false, nil
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
// withdrawn because its material was unpreparable stays withdrawn: the counts
// say nothing new about its sentences.
func returnIdiom(ctx context.Context, store idiomReviewStore, owner string, word domain.StudyProgress, now time.Time) (bool, error) {
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
		return false, nil
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
	if _, found, err := reader.entry(ctx, headword); err != nil || !found {
		// A refresh dropped the headword: its Corpus counts say so once they
		// are rebuilt, and the Idiom is withdrawn then. Until then there is
		// no sentence to compare it with.
		return true, err
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
