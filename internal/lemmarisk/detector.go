// Package lemmarisk conservatively identifies exact analyzed occurrences that
// merit learner review. It never decides or rewrites a vocabulary identity.
package lemmarisk

import (
	"context"
	"sort"
	"strings"
)

const recurringFloor = 3

type Occurrence struct {
	ID, Language, Surface, Lemma, UPOS, Sentence string
	Reviewed                                     bool
	Excluded                                     bool
}

// Alternative retains the local-reference provenance and the detector's
// sentence-specific evidence assessment. ContextRelevant is deliberately
// supplied by the evidence adapter, not inferred from dictionary absence.
type Alternative struct {
	Lemma, UPOS, Source, Version, EvidenceID string
	ContextRelevant                          bool
}

type AlternativeIndex interface {
	LemmaExists(context.Context, string, string, string) (bool, error)
	Alternatives(context.Context, string, string, string, string) ([]Alternative, error)
}

// LemmaAlternativeIndex is an optional extension for adapters whose evidence
// rule needs the analyzed lemma. The detector prefers it when the index
// implements it; an adapter that does not keeps the surface-and-UPOS contract.
type LemmaAlternativeIndex interface {
	AlternativesForLemma(context.Context, string, string, string, string, string) ([]Alternative, error)
}

// AvailabilityIndex is an optional extension for an adapter whose language
// evidence can be absent even when the index opens. Detect consults it before
// treating a Book with nothing to inspect as a clean assessment.
type AvailabilityIndex interface {
	AssessmentAvailable(context.Context, string) (bool, error)
}

type Flag struct {
	OccurrenceID string
	Reason       string
	Alternative  Alternative
}

type occurrenceAlternatives struct {
	Occurrence   Occurrence
	Alternatives []Alternative
}

// Detect returns assessed=false when automatic detection is disabled or local
// evidence is unavailable. A missing exact lemma is never sufficient to flag.
func Detect(ctx context.Context, language string, occurrences []Occurrence, index AlternativeIndex) ([]Flag, bool, error) {
	language = strings.ToLower(strings.TrimSpace(language))
	if !SupportsLanguage(language) || index == nil {
		return nil, false, nil
	}
	if availability, ok := index.(AvailabilityIndex); ok {
		available, err := availability.AssessmentAvailable(ctx, language)
		if err != nil {
			return nil, false, err
		}
		if !available {
			return nil, false, nil
		}
	}
	counts := countLemmaOccurrences(occurrences)
	state := newDetectionState(len(occurrences))
	possible := make([]occurrenceAlternatives, 0)
	for _, occurrence := range occurrences {
		alternatives, err := state.collectValidAlternatives(ctx, occurrence, index)
		if err != nil {
			return nil, false, err
		}
		if len(alternatives) > 0 {
			possible = append(possible, occurrenceAlternatives{Occurrence: occurrence, Alternatives: alternatives})
		}
	}
	flags := flagRecurringCandidates(possible, counts, state.potentialCounts, flagReason(language))
	sort.Slice(flags, func(i, j int) bool { return flags[i].OccurrenceID < flags[j].OccurrenceID })
	return flags, true, nil
}

// countLemmaOccurrences counts how many analyzed occurrences already carry each
// (lemma, UPOS) identity, ignoring exclusions.
func countLemmaOccurrences(occurrences []Occurrence) map[string]int {
	counts := make(map[string]int, len(occurrences))
	for _, occurrence := range occurrences {
		if !occurrence.Excluded {
			counts[identity(occurrence.Lemma, occurrence.UPOS)]++
		}
	}
	return counts
}

// detectionState caches lemma existence and accumulates the competing identities
// that qualifying occurrences could contribute.
type detectionState struct {
	lemmaExists          map[string]bool
	lemmaChecked         map[string]bool
	potentialCounts      map[string]int
	potentialOccurrences map[string]map[string]bool
}

func newDetectionState(size int) *detectionState {
	return &detectionState{
		lemmaExists:          make(map[string]bool, size),
		lemmaChecked:         make(map[string]bool, size),
		potentialCounts:      make(map[string]int, size),
		potentialOccurrences: make(map[string]map[string]bool, size),
	}
}

// collectValidAlternatives returns the context-supported alternatives for one
// occurrence the local index does not already carry, recording each new
// candidate identity as a potential occurrence of the proposed lemma.
func (s *detectionState) collectValidAlternatives(ctx context.Context, occurrence Occurrence, index AlternativeIndex) ([]Alternative, error) {
	language := strings.ToLower(strings.TrimSpace(occurrence.Language))
	if !SupportsLanguage(language) || occurrence.Reviewed || occurrence.Excluded || strings.TrimSpace(occurrence.ID) == "" || strings.TrimSpace(occurrence.Lemma) == "" {
		return nil, nil
	}
	key := identity(occurrence.Lemma, occurrence.UPOS)
	exists, checked := s.lemmaExists[key], s.lemmaChecked[key]
	if !checked {
		var err error
		exists, err = index.LemmaExists(ctx, language, occurrence.Lemma, occurrence.UPOS)
		if err != nil {
			return nil, err
		}
		s.lemmaExists[key], s.lemmaChecked[key] = exists, true
	}
	if exists {
		return nil, nil
	}
	var alternatives []Alternative
	var err error
	if aware, ok := index.(LemmaAlternativeIndex); ok {
		alternatives, err = aware.AlternativesForLemma(ctx, language, occurrence.Surface, occurrence.Lemma, occurrence.UPOS, occurrence.Sentence)
	} else {
		alternatives, err = index.Alternatives(ctx, language, occurrence.Surface, occurrence.UPOS, occurrence.Sentence)
	}
	if err != nil {
		return nil, err
	}
	valid := make([]Alternative, 0, len(alternatives))
	seen := make(map[string]bool)
	for _, alternative := range alternatives {
		lemma := strings.TrimSpace(alternative.Lemma)
		if !alternative.ContextRelevant || lemma == "" || lemma == occurrence.Lemma || alternative.UPOS != occurrence.UPOS || alternative.Source == "" || alternative.Version == "" || alternative.EvidenceID == "" {
			continue
		}
		candidateID := identity(lemma, alternative.UPOS)
		if seen[candidateID] {
			continue
		}
		seen[candidateID] = true
		valid = append(valid, alternative)
		s.recordPotential(candidateID, occurrence.ID)
	}
	return valid, nil
}

// recordPotential counts one occurrence per candidate identity, so recurring
// alternatives merge with the occurrences already carrying that identity.
func (s *detectionState) recordPotential(candidateID, occurrenceID string) {
	if s.potentialOccurrences[candidateID] == nil {
		s.potentialOccurrences[candidateID] = make(map[string]bool)
	}
	if !s.potentialOccurrences[candidateID][occurrenceID] {
		s.potentialOccurrences[candidateID][occurrenceID] = true
		s.potentialCounts[candidateID]++
	}
}

// flagRecurringCandidates flags one occurrence per alternative whose combined
// existing and potential occurrences reach the recurring floor.
func flagRecurringCandidates(possible []occurrenceAlternatives, counts, potentialCounts map[string]int, reason string) []Flag {
	flags := make([]Flag, 0)
	for _, candidate := range possible {
		for _, alternative := range candidate.Alternatives {
			candidateID := identity(alternative.Lemma, alternative.UPOS)
			if counts[candidateID]+potentialCounts[candidateID] < recurringFloor {
				continue
			}
			flags = append(flags, Flag{
				OccurrenceID: candidate.Occurrence.ID,
				Reason:       reason,
				Alternative:  alternative,
			})
			break
		}
	}
	return flags
}

// flagReason names the evidence rule that produced a flag. German keeps its
// sentence-overlap wording; French evidence is the index link itself.
func flagReason(language string) string {
	if language == "fr" {
		return "The analyzed lemma is a local-index miss and the dictionary links this surface to a competing lemma with the same part of speech that could affect recurring vocabulary."
	}
	return "The analyzed lemma is a local-index miss and sentence-level lexical evidence supports a competing lemma that could affect recurring vocabulary."
}

func identity(lemma, upos string) string { return strings.ToLower(lemma) + "\x00" + upos }

// SupportsLanguage reports whether automatic lemma-risk detection runs for a
// language. German and French have dictionary-backed evidence rules; every
// other language remains manual-only.
func SupportsLanguage(language string) bool {
	return language == "de" || language == "fr"
}
