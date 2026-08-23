// Package webworkflow adapts persisted pipeline results to the interactive web UI.
package webworkflow

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/ranking"
	"github.com/justin-hayes/mouseion/internal/review"
	"github.com/justin-hayes/mouseion/internal/selection"
	"github.com/justin-hayes/mouseion/internal/sentences"
	"github.com/justin-hayes/mouseion/internal/vocabulary"
)

// Review delegates every mutation to the core service and only assembles its
// already-persisted inputs for Present.
type Review struct {
	pool *pgxpool.Pool
	core *review.Service
}

func NewReview(pool *pgxpool.Pool, core *review.Service) *Review {
	return &Review{pool: pool, core: core}
}

func (r *Review) Present(ctx context.Context, owner string) ([]review.Item, error) {
	rows, err := r.pool.Query(ctx, `SELECT sc.language,sc.canonical_lemma,sc.upos,sc.occurrence_count,sc.observed_forms,sc.eligible_sentence_refs,sc.provenance,COALESCE(sc.ranking_global_pct,0),COALESCE(sc.ranking_corpus_pct,0),COALESCE(sc.ranking_priority,false),COALESCE(sc.ranking_cross_text,0),COALESCE(sc.ranking_score,0),COALESCE(ec.translation,''),COALESCE(ec.gloss,'') FROM selection_candidates sc LEFT JOIN LATERAL (SELECT translation,gloss FROM enrichment_cache WHERE language=sc.language AND canonical_lemma=sc.canonical_lemma AND upos=upper(sc.upos) ORDER BY cached_at DESC LIMIT 1) ec ON true WHERE sc.owner_id=$1 ORDER BY sc.ranking_score DESC NULLS LAST,sc.language,sc.canonical_lemma,sc.upos`, owner)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var inputs []review.Input
	for rows.Next() {
		var candidate selection.Candidate
		var observed, refs, provenance []byte
		var components domain.RankingComponents
		var translation, gloss string
		if err = rows.Scan(&candidate.Identity.Language, &candidate.Identity.CanonicalLemma, &candidate.Identity.UPOS, &candidate.OccurrenceCount, &observed, &refs, &provenance, &components.GlobalPercentile, &components.CorpusPercentile, &components.Priority, &components.CrossText, &components.Score, &translation, &gloss); err != nil {
			return nil, err
		}
		if err = json.Unmarshal(observed, &candidate.ObservedForms); err != nil {
			return nil, fmt.Errorf("decode observed forms: %w", err)
		}
		if err = json.Unmarshal(refs, &candidate.SentenceReferences); err != nil {
			return nil, fmt.Errorf("decode sentence references: %w", err)
		}
		if err = json.Unmarshal(provenance, &candidate.Provenance); err != nil {
			return nil, fmt.Errorf("decode selection provenance: %w", err)
		}
		examples, err := r.loadSentences(ctx, owner, candidate.Identity)
		if err != nil {
			return nil, err
		}
		enriched := enrichment.Result{Candidate: enrichment.Candidate{Identity: enrichment.Identity(candidate.Identity)}}
		if translation != "" {
			enriched.Translation = enrichment.Field[string]{Value: translation, Available: true}
		}
		if gloss != "" {
			enriched.Gloss = enrichment.Field[string]{Value: gloss, Available: true}
		}
		inputs = append(inputs, review.Input{Candidate: ranking.RankedCandidate{Candidate: candidate, Components: components}, Enrichment: enriched, Sentences: examples})
	}
	if err = rows.Err(); err != nil {
		return nil, err
	}
	return r.core.Present(ctx, owner, inputs)
}

func (r *Review) loadSentences(ctx context.Context, owner string, id selection.Identity) (sentences.Result, error) {
	rows, err := r.pool.Query(ctx, `WITH latest AS (SELECT corpus_id FROM example_sentences WHERE owner_id=$1 AND language=$2 AND canonical_lemma=$3 AND upos=$4 GROUP BY corpus_id ORDER BY max(created_at) DESC,corpus_id DESC LIMIT 1) SELECT sentence_text,source_location,selection_score,selection_reasons,is_chosen FROM example_sentences WHERE owner_id=$1 AND language=$2 AND canonical_lemma=$3 AND upos=$4 AND corpus_id=(SELECT corpus_id FROM latest) ORDER BY is_chosen DESC,selection_rank,id`, owner, id.Language, id.CanonicalLemma, id.UPOS)
	if err != nil {
		return sentences.Result{}, err
	}
	defer rows.Close()
	var result sentences.Result
	for rows.Next() {
		var item sentences.ScoredSentence
		var location, reasons []byte
		var chosen bool
		if err = rows.Scan(&item.Text, &location, &item.Score, &reasons, &chosen); err != nil {
			return result, err
		}
		if err = json.Unmarshal(location, &item.Location.Location); err != nil {
			return result, err
		}
		if err = json.Unmarshal(reasons, &item.Reasons); err != nil {
			return result, err
		}
		if chosen && result.Chosen == nil {
			copy := item
			result.Chosen = &copy
		} else {
			result.Alternatives = append(result.Alternatives, item)
		}
	}
	return result, rows.Err()
}

func (r *Review) Accept(ctx context.Context, owner string, id vocabulary.Identity) (domain.VocabularyState, error) {
	return r.core.Accept(ctx, owner, id)
}
func (r *Review) Ignore(ctx context.Context, owner string, id vocabulary.Identity) (domain.VocabularyState, error) {
	return r.core.Ignore(ctx, owner, id)
}
func (r *Review) MarkKnown(ctx context.Context, owner string, id vocabulary.Identity) (domain.VocabularyState, error) {
	return r.core.MarkKnown(ctx, owner, id)
}
func (r *Review) Reset(ctx context.Context, owner string, id vocabulary.Identity) (domain.VocabularyState, error) {
	return r.core.Reset(ctx, owner, id)
}
func (r *Review) EditExample(ctx context.Context, owner string, id vocabulary.Identity, text string) (domain.CuratedSentence, error) {
	return r.core.EditExample(ctx, owner, id, text)
}
func (r *Review) ChooseAlternate(ctx context.Context, owner string, id vocabulary.Identity, index int) (domain.CuratedSentence, error) {
	return r.core.ChooseAlternate(ctx, owner, id, index)
}
