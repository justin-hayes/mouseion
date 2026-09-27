//go:build integration

package testutil

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
)

// PresentationProvider describes the external identity used by a fixture's
// frozen work. An empty provider creates a local deck without external work.
type PresentationProvider struct {
	Name, Version, TargetLanguage string
}

// FreezePresentationDeck builds an integration fixture through the public
// presentation lifecycle instead of reaching into manifest internals.
func FreezePresentationDeck(ctx context.Context, owner, deckName string, entries []cardexport.Entry, provider PresentationProvider) (cardexport.FrozenDeck, error) {
	return FreezePresentationDeckWithLexical(ctx, owner, deckName, entries, provider, nil)
}

// FreezePresentationDeckWithLexical also exercises the local provider through
// the public presentation lifecycle.
func FreezePresentationDeckWithLexical(ctx context.Context, owner, deckName string, entries []cardexport.Entry, provider PresentationProvider, lexical enrichment.LexicalProvider) (cardexport.FrozenDeck, error) {
	projections := make([]cardexport.CandidateProjection, 0, len(entries))
	for _, entry := range entries {
		upos := strings.ToUpper(strings.TrimSpace(entry.UPOS))
		observed := entry.TargetWord
		if observed == "" {
			observed = entry.CanonicalLemma
		}
		references, err := json.Marshal([]struct {
			SentenceIndex int            `json:"sentence_index"`
			Text          string         `json:"text"`
			Location      map[string]int `json:"location"`
		}{{
			SentenceIndex: int(entry.SentenceOrdinal),
			Text:          entry.Sentence,
			Location:      map[string]int{"start_offset": int(entry.FirstEncounter)},
		}})
		if err != nil {
			return cardexport.FrozenDeck{}, err
		}
		observedForms, err := json.Marshal([]string{observed})
		if err != nil {
			return cardexport.FrozenDeck{}, err
		}
		candidate := domain.SelectionCandidate{
			OwnerID:            owner,
			CorpusID:           entry.CorpusID,
			Language:           entry.Language,
			CanonicalLemma:     entry.CanonicalLemma,
			UPOS:               upos,
			FirstEncounter:     entry.FirstEncounter,
			ObservedForms:      observedForms,
			SentenceReferences: references,
		}
		projection := cardexport.CandidateProjection{
			OwnerID:         owner,
			DeckName:        deckName,
			Candidate:       candidate,
			Entry:           entry,
			Provider:        provider.Name,
			ProviderVersion: provider.Version,
			TargetLanguage:  provider.TargetLanguage,
		}
		if allEntriesHaveTokens(entries) {
			projection.Sentences = map[int64]analyzer.Sentence{
				entry.SentenceOrdinal: {Text: entry.Sentence, Tokens: entry.SentenceTokens},
			}
		}
		projections = append(projections, projection)
	}
	deck, _, err := cardexport.NewPresentation(lexical).Freeze(ctx, owner, deckName, projections)
	return deck, err
}

func allEntriesHaveTokens(entries []cardexport.Entry) bool {
	for _, entry := range entries {
		if entry.SentenceTokens == nil {
			return false
		}
	}
	return true
}
