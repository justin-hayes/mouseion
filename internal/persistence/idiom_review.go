package persistence

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/justin-hayes/mouseion/internal/analyzer"
)

// ConfirmedIdiomCard is one provider-confirmed idiom card from a Book's current
// prepared deck, as the operator sample review reads it. It contains only the
// card's own source and provider result, never learner state.
type ConfirmedIdiomCard struct {
	Language       string
	Headword       string
	MatchedWords   []string
	Sentence       string
	Gloss          string
	Translation    string
	AnchorLemma    string
	AnchorUPOS     string
	MatcherVersion string
	GapBound       int
}

// idiomSampleRenderPayload is the frozen manifest payload fields the sample
// review reads. The durable manifest stores the analyzer tokens and matched
// ordinals so a review can name the exact words the matcher found.
type idiomSampleRenderPayload struct {
	SentenceTokens            []analyzer.Token `json:"sentence_tokens"`
	IdiomMatchedTokenOrdinals []int            `json:"idiom_matched_token_ordinals"`
	IdiomAnchorCanonicalLemma string           `json:"idiom_anchor_canonical_lemma"`
	IdiomAnchorUPOS           string           `json:"idiom_anchor_upos"`
	IdiomMatcherVersion       string           `json:"idiom_matcher_version"`
	IdiomGapBound             int              `json:"idiom_gap_bound"`
	DictionaryProviderVersion string           `json:"dictionary_provider_version"`
}

// ListConfirmedIdiomCardsForBook returns the accepted idiom items of the Book's
// most recent ready prepared deck whose translation resolved to a Gloss. The
// cache join uses the frozen item identity, so an omitted or unrelated result
// is never sampled.
func (s *PostgresStore) ListConfirmedIdiomCardsForBook(ctx context.Context, bookID, language string) ([]ConfirmedIdiomCard, error) {
	rows, err := s.pool.Query(ctx, `
		WITH current AS (
			SELECT owner_id, id, current_run_id
			FROM deck_preparations
			WHERE book_id = $1 AND state = 'ready' AND retired_at IS NULL AND current_run_id IS NOT NULL
			ORDER BY updated_at DESC, id DESC
			LIMIT 1
		)
		SELECT mi.language, mi.headword, mi.source_sentence,
		       mi.render_payload, ec.translation, ec.fallback_gloss
		FROM current c
		JOIN deck_preparation_manifest_items mi
		  ON mi.owner_id = c.owner_id
		 AND mi.preparation_id = c.id
		 AND mi.run_id = c.current_run_id
		JOIN enrichment_cache ec
		  ON ec.language = mi.language
		 AND ec.target_language = mi.target_language
		 AND ec.canonical_lemma = mi.canonical_lemma
		 AND ec.upos = mi.upos
		 AND ec.item_kind = mi.item_kind
		 AND ec.provider = mi.provider
		 AND ec.provider_version = mi.provider_version
		 AND ec.sentence_hash = mi.sentence_hash
		 AND ec.dictionary_provider_version = COALESCE(mi.render_payload->>'dictionary_provider_version', '')
		 AND ec.meaning_evidence_hash = COALESCE(mi.meaning_evidence_hash, '')
		WHERE mi.item_kind = 'idiom'
		  AND mi.disposition = 'accepted'
		  AND mi.language = $2
		  AND btrim(ec.translation) <> ''
		  AND btrim(ec.sentence_translation) <> ''
		  AND btrim(ec.fallback_gloss) <> ''
		ORDER BY mi.ordinal`, bookID, language)
	if err != nil {
		return nil, fmt.Errorf("list confirmed idiom cards: %w", err)
	}
	defer rows.Close()
	cards := make([]ConfirmedIdiomCard, 0)
	for rows.Next() {
		var card ConfirmedIdiomCard
		var payloadBytes []byte
		if err = rows.Scan(&card.Language, &card.Headword, &card.Sentence, &payloadBytes, &card.Translation, &card.Gloss); err != nil {
			return nil, fmt.Errorf("scan confirmed idiom card: %w", err)
		}
		var payload idiomSampleRenderPayload
		if err = json.Unmarshal(payloadBytes, &payload); err != nil {
			return nil, fmt.Errorf("decode confirmed idiom card %q: %w", card.Headword, err)
		}
		card.MatchedWords, err = matchedIdiomWords(payload)
		if err != nil {
			return nil, fmt.Errorf("decode confirmed idiom card %q: %w", card.Headword, err)
		}
		card.AnchorLemma = payload.IdiomAnchorCanonicalLemma
		card.AnchorUPOS = payload.IdiomAnchorUPOS
		card.MatcherVersion = payload.IdiomMatcherVersion
		card.GapBound = payload.IdiomGapBound
		cards = append(cards, card)
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate confirmed idiom cards: %w", err)
	}
	return cards, nil
}

// matchedIdiomWords resolves every frozen matched token ordinal to its surface
// form. A missing or out-of-range ordinal is a corrupt manifest item rather
// than a reason to sample an incomplete card.
func matchedIdiomWords(payload idiomSampleRenderPayload) ([]string, error) {
	if len(payload.IdiomMatchedTokenOrdinals) == 0 {
		return nil, errors.New("idiom evidence has no matched token ordinals")
	}
	words := make([]string, 0, len(payload.IdiomMatchedTokenOrdinals))
	for _, ordinal := range payload.IdiomMatchedTokenOrdinals {
		if ordinal < 0 || ordinal >= len(payload.SentenceTokens) {
			return nil, errors.New("idiom matched token ordinal is outside the sentence")
		}
		words = append(words, payload.SentenceTokens[ordinal].Surface)
	}
	return words, nil
}
