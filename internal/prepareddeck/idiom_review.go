package prepareddeck

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
)

// errIdiomReviewSourceEvidenceDeck reports a Book whose deck is built from
// source evidence alone, which never carries idiom cards.
var errIdiomReviewSourceEvidenceDeck = errors.New("prepareddeck: a source-evidence deck carries no idiom cards")

// IdiomReviewFreezer re-freezes a Book's current deck preparation in memory for
// the operator idiom review (#2375). It turns idiom cards on for the requested
// language and lifts the per-deck bound, and it does not check the idiom
// payload assessment: recording that assessment is the review's outcome, so
// requiring it here would make the review impossible. Nothing is written.
type IdiomReviewFreezer struct {
	Store        *persistence.PostgresStore
	Presentation *cardexport.Presentation
	Codec        *enrichment.TranslationCodec
	// Assembler supplies the preparation's projections. Nil uses the
	// prepared-deck input assembler over Store.
	Assembler inputAssembler
}

// IdiomReviewFreeze is what one in-memory review freeze found for a language.
type IdiomReviewFreeze struct {
	// Items are every accepted idiom item, each with the exact request a
	// prepared deck sends.
	Items []cardexport.IdiomWorkItem
	// Ranking is every matched idiom in the per-deck bound's order. The freeze
	// lifts the bound, so a prepared deck keeps the first
	// cardexport.IdiomCardBound of them (#2379).
	Ranking []cardexport.IdiomRank
}

// FreezeIdiomReviewItems returns every accepted idiom item the Book's current
// deck preparation would carry for language, and the bound's ranking of them.
// It runs in a read-only transaction that is always rolled back.
func (f IdiomReviewFreezer) FreezeIdiomReviewItems(ctx context.Context, bookID, language string) (result IdiomReviewFreeze, err error) {
	language = canonicalization.NormalizeLanguage(language)
	if f.Store == nil || f.Presentation == nil || f.Codec == nil || strings.TrimSpace(bookID) == "" || language == "" {
		return IdiomReviewFreeze{}, ErrInvalidInput
	}
	tx, err := f.Store.Pool().BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return IdiomReviewFreeze{}, fmt.Errorf("begin idiom review transaction: %w", err)
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	preparation, err := f.Store.CurrentDeckPreparationForBookTx(ctx, tx, bookID)
	if err != nil {
		return IdiomReviewFreeze{}, err
	}
	assembler := f.Assembler
	if assembler == nil {
		assembler = NewInputAssembler(f.Store)
	}
	projections, deckName, err := assembler.AssemblePreparedDeckInputs(ctx, tx, preparation)
	if err != nil {
		return IdiomReviewFreeze{}, fmt.Errorf("assemble prepared deck inputs: %w", err)
	}
	if sourceEvidenceDeck(projections) {
		return IdiomReviewFreeze{}, errIdiomReviewSourceEvidenceDeck
	}
	assignExternalProvider(projections, f.Codec.ProviderName(), f.Codec.ContextualGlossProviderVersion(), "en")
	deck, diagnostics, err := f.Presentation.Freeze(ctx, preparation.OwnerID, deckName, projections,
		cardexport.WithIdiomCards([]string{language}, f.Codec.IdiomCardProviderVersion()),
		cardexport.WithoutIdiomCardBound())
	if err != nil {
		return IdiomReviewFreeze{}, fmt.Errorf("freeze prepared deck presentation: %w", err)
	}
	ranking := make([]cardexport.IdiomRank, 0, len(diagnostics.IdiomRanking))
	for _, rank := range diagnostics.IdiomRanking {
		if canonicalization.NormalizeLanguage(rank.Language) == language {
			ranking = append(ranking, rank)
		}
	}
	return IdiomReviewFreeze{Items: deck.IdiomWork(language), Ranking: ranking}, nil
}
