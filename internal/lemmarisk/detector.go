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
	if language != "de" || index == nil {
		return nil, false, nil
	}
	counts := make(map[string]int, len(occurrences))
	lemmaExists := make(map[string]bool, len(occurrences))
	lemmaChecked := make(map[string]bool, len(occurrences))
	for _, occurrence := range occurrences {
		if !occurrence.Excluded {
			counts[identity(occurrence.Lemma, occurrence.UPOS)]++
		}
	}
	possible := make([]occurrenceAlternatives, 0)
	potentialCounts := make(map[string]int, len(occurrences))
	potentialOccurrences := make(map[string]map[string]bool, len(occurrences))
	for _, occurrence := range occurrences {
		if strings.ToLower(occurrence.Language) != "de" || occurrence.Reviewed || occurrence.Excluded || strings.TrimSpace(occurrence.ID) == "" || strings.TrimSpace(occurrence.Lemma) == "" {
			continue
		}
		key := identity(occurrence.Lemma, occurrence.UPOS)
		exists, checked := lemmaExists[key], lemmaChecked[key]
		if !checked {
			var err error
			exists, err = index.LemmaExists(ctx, "de", occurrence.Lemma, occurrence.UPOS)
			if err != nil {
				return nil, false, err
			}
			lemmaExists[key], lemmaChecked[key] = exists, true
		}
		if exists {
			continue
		}
		alternatives, err := index.Alternatives(ctx, "de", occurrence.Surface, occurrence.UPOS, occurrence.Sentence)
		if err != nil {
			return nil, false, err
		}
		validAlternatives := make([]Alternative, 0, len(alternatives))
		seenAlternatives := make(map[string]bool)
		for _, alternative := range alternatives {
			lemma := strings.TrimSpace(alternative.Lemma)
			if !alternative.ContextRelevant || lemma == "" || lemma == occurrence.Lemma || alternative.UPOS != occurrence.UPOS || alternative.Source == "" || alternative.Version == "" || alternative.EvidenceID == "" {
				continue
			}
			candidateID := identity(lemma, alternative.UPOS)
			if seenAlternatives[candidateID] {
				continue
			}
			seenAlternatives[candidateID] = true
			validAlternatives = append(validAlternatives, alternative)
			if potentialOccurrences[candidateID] == nil {
				potentialOccurrences[candidateID] = make(map[string]bool)
			}
			if !potentialOccurrences[candidateID][occurrence.ID] {
				potentialOccurrences[candidateID][occurrence.ID] = true
				potentialCounts[candidateID]++
			}
		}
		if len(validAlternatives) > 0 {
			possible = append(possible, occurrenceAlternatives{Occurrence: occurrence, Alternatives: validAlternatives})
		}
	}
	flags := make([]Flag, 0)
	for _, candidate := range possible {
		for _, alternative := range candidate.Alternatives {
			candidateID := identity(alternative.Lemma, alternative.UPOS)
			if counts[candidateID]+potentialCounts[candidateID] < recurringFloor {
				continue
			}
			flags = append(flags, Flag{
				OccurrenceID: candidate.Occurrence.ID,
				Reason:       "The analyzed lemma is a local-index miss and sentence-level lexical evidence supports a competing lemma that could affect recurring vocabulary.",
				Alternative:  alternative,
			})
			break
		}
	}
	sort.Slice(flags, func(i, j int) bool { return flags[i].OccurrenceID < flags[j].OccurrenceID })
	return flags, true, nil
}

func identity(lemma, upos string) string { return strings.ToLower(lemma) + "\x00" + upos }
