// Package webworkflow adapts persisted pipeline results to the interactive web UI.
package webworkflow

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

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

func (r *Review) Present(ctx context.Context, owner string, query review.Query) (review.Page, error) {
	query.BookID, query.Lemma, query.UPOS, query.Decision = strings.TrimSpace(query.BookID), strings.TrimSpace(query.Lemma), strings.TrimSpace(query.UPOS), strings.TrimSpace(query.Decision)
	if query.BookID == "" {
		return review.Page{}, review.ErrInvalidInput
	}
	if query.Page < 1 {
		query.Page = 1
	}
	if query.PageSize < 1 || query.PageSize > 100 {
		query.PageSize = 25
	}
	where := ` WHERE sc.owner_id=$1 AND co.source_material_id=$2`
	args := []any{owner, query.BookID}
	if query.Lemma != "" {
		args = append(args, "%"+query.Lemma+"%")
		where += fmt.Sprintf(" AND sc.canonical_lemma ILIKE $%d", len(args))
	}
	if query.UPOS != "" {
		args = append(args, strings.ToUpper(query.UPOS))
		where += fmt.Sprintf(" AND sc.upos=$%d", len(args))
	}
	if query.Decision != "" {
		if query.Decision != "candidate" && query.Decision != "accepted" && query.Decision != "known" && query.Decision != "ignored" && query.Decision != "generated" {
			return review.Page{}, review.ErrInvalidInput
		}
		args = append(args, query.Decision)
		where += fmt.Sprintf(" AND vs.state=$%d", len(args))
	}
	order := "first_encounter ASC NULLS LAST,sc.canonical_lemma,sc.upos"
	switch query.Sort {
	case "", "encounter":
	case "book":
		order = "sc.occurrence_count DESC,sc.canonical_lemma,sc.upos"
	case "global":
		order = "sc.ranking_global_pct DESC NULLS LAST,sc.canonical_lemma,sc.upos"
	case "lemma":
		order = "sc.canonical_lemma,sc.upos"
	default:
		return review.Page{}, review.ErrInvalidInput
	}
	var total int
	countSQL := `SELECT count(*) FROM selection_candidates sc JOIN corpora co ON co.owner_id=sc.owner_id AND co.id::text=sc.corpus_id JOIN vocabulary_states vs ON vs.owner_id=sc.owner_id AND vs.language=sc.language AND vs.canonical_lemma=sc.canonical_lemma AND vs.upos=sc.upos` + where
	if err := r.pool.QueryRow(ctx, countSQL, args...).Scan(&total); err != nil {
		return review.Page{}, err
	}
	lastPage := (total + query.PageSize - 1) / query.PageSize
	if lastPage < 1 {
		lastPage = 1
	}
	if query.Page > lastPage {
		query.Page = lastPage
	}
	args = append(args, query.PageSize, (query.Page-1)*query.PageSize)
	rows, err := r.pool.Query(ctx, `SELECT sc.language,sc.canonical_lemma,sc.upos,sc.corpus_id,sc.occurrence_count,sc.observed_forms,sc.eligible_sentence_refs,sc.provenance,COALESCE(sc.ranking_global_pct,0),COALESCE(sc.ranking_corpus_pct,0),COALESCE(sc.ranking_priority,false),COALESCE(sc.ranking_cross_text,0),COALESCE(sc.ranking_score,0),COALESCE(ec.translation,''),COALESCE(ec.gloss,''),first_seen.first_encounter FROM selection_candidates sc JOIN corpora co ON co.owner_id=sc.owner_id AND co.id::text=sc.corpus_id JOIN vocabulary_states vs ON vs.owner_id=sc.owner_id AND vs.language=sc.language AND vs.canonical_lemma=sc.canonical_lemma AND vs.upos=sc.upos LEFT JOIN LATERAL (SELECT MIN(COALESCE(ref->'location'->>'start_offset',ref->'Location'->>'StartOffset')::bigint) first_encounter FROM jsonb_array_elements(sc.eligible_sentence_refs) ref) first_seen ON true LEFT JOIN LATERAL (SELECT translation,gloss FROM enrichment_cache WHERE language=sc.language AND canonical_lemma=sc.canonical_lemma AND upos=upper(sc.upos) ORDER BY cached_at DESC LIMIT 1) ec ON true`+where+` ORDER BY `+order+fmt.Sprintf(" LIMIT $%d OFFSET $%d", len(args)-1, len(args)), args...)
	if err != nil {
		return review.Page{}, err
	}
	defer rows.Close()
	var inputs []review.Input
	var encounters []int64
	for rows.Next() {
		var candidate selection.Candidate
		var observed, refs, provenance []byte
		var components domain.RankingComponents
		var translation, gloss string
		var corpusID string
		var firstEncounter *int64
		if err = rows.Scan(&candidate.Identity.Language, &candidate.Identity.CanonicalLemma, &candidate.Identity.UPOS, &corpusID, &candidate.OccurrenceCount, &observed, &refs, &provenance, &components.GlobalPercentile, &components.CorpusPercentile, &components.Priority, &components.CrossText, &components.Score, &translation, &gloss, &firstEncounter); err != nil {
			return review.Page{}, err
		}
		if err = json.Unmarshal(observed, &candidate.ObservedForms); err != nil {
			return review.Page{}, fmt.Errorf("decode observed forms: %w", err)
		}
		if err = json.Unmarshal(refs, &candidate.SentenceReferences); err != nil {
			return review.Page{}, fmt.Errorf("decode sentence references: %w", err)
		}
		if err = json.Unmarshal(provenance, &candidate.Provenance); err != nil {
			return review.Page{}, fmt.Errorf("decode selection provenance: %w", err)
		}
		examples, err := r.loadSentences(ctx, owner, corpusID, candidate.Identity)
		if err != nil {
			return review.Page{}, err
		}
		enriched := enrichment.Result{Candidate: enrichment.Candidate{Identity: enrichment.Identity(candidate.Identity)}}
		if translation != "" {
			enriched.Translation = enrichment.Field[string]{Value: translation, Available: true}
		}
		if gloss != "" {
			enriched.Gloss = enrichment.Field[string]{Value: gloss, Available: true}
		}
		input := review.Input{Candidate: ranking.RankedCandidate{Candidate: candidate, Components: components}, Enrichment: enriched, Sentences: examples}
		inputs = append(inputs, input)
		if firstEncounter == nil {
			encounters = append(encounters, 0)
		} else {
			encounters = append(encounters, *firstEncounter)
		}
	}
	if err = rows.Err(); err != nil {
		return review.Page{}, err
	}
	items, err := r.core.Present(ctx, owner, inputs)
	if err != nil {
		return review.Page{}, err
	}
	for i := range items {
		items[i].FirstEncounter = encounters[i]
	}
	return review.Page{Items: items, Page: query.Page, PageSize: query.PageSize, Total: total}, nil
}

func (r *Review) loadSentences(ctx context.Context, owner, corpusID string, id selection.Identity) (sentences.Result, error) {
	rows, err := r.pool.Query(ctx, `SELECT sentence_text,source_location,selection_score,selection_reasons,is_chosen FROM example_sentences WHERE owner_id=$1 AND corpus_id=$2 AND language=$3 AND canonical_lemma=$4 AND upos=$5 ORDER BY is_chosen DESC,selection_rank,id`, owner, corpusID, id.Language, id.CanonicalLemma, id.UPOS)
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
func (r *Review) AcceptForBook(ctx context.Context, owner, bookID string, id vocabulary.Identity) (domain.VocabularyState, error) {
	return r.core.AcceptForBook(ctx, owner, bookID, id)
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
func (r *Review) EditExampleForBook(ctx context.Context, owner, bookID string, id vocabulary.Identity, text string) (domain.CuratedSentence, error) {
	return r.core.EditExampleForBook(ctx, owner, bookID, id, text)
}
func (r *Review) ChooseAlternate(ctx context.Context, owner string, id vocabulary.Identity, index int) (domain.CuratedSentence, error) {
	return r.core.ChooseAlternate(ctx, owner, id, index)
}
func (r *Review) ChooseAlternateForBook(ctx context.Context, owner, bookID string, id vocabulary.Identity, index int) (domain.CuratedSentence, error) {
	return r.core.ChooseAlternateForBook(ctx, owner, bookID, id, index)
}
