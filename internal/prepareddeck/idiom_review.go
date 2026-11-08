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

// FreezeIdiomReviewItems returns every accepted idiom item the Book's current
// deck preparation would carry for language, each with the exact request a
// prepared deck sends. It runs in a read-only transaction that is always
// rolled back.
func (f IdiomReviewFreezer) FreezeIdiomReviewItems(ctx context.Context, bookID, language string) (items []cardexport.IdiomWorkItem, err error) {
	language = canonicalization.NormalizeLanguage(language)
	if f.Store == nil || f.Presentation == nil || f.Codec == nil || strings.TrimSpace(bookID) == "" || language == "" {
		return nil, ErrInvalidInput
	}
	tx, err := f.Store.Pool().BeginTx(ctx, pgx.TxOptions{AccessMode: pgx.ReadOnly})
	if err != nil {
		return nil, fmt.Errorf("begin idiom review transaction: %w", err)
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	preparation, err := f.Store.CurrentDeckPreparationForBookTx(ctx, tx, bookID)
	if err != nil {
		return nil, err
	}
	assembler := f.Assembler
	if assembler == nil {
		assembler = NewInputAssembler(f.Store)
	}
	projections, deckName, err := assembler.AssemblePreparedDeckInputs(ctx, tx, preparation)
	if err != nil {
		return nil, fmt.Errorf("assemble prepared deck inputs: %w", err)
	}
	if sourceEvidenceDeck(projections) {
		return nil, errIdiomReviewSourceEvidenceDeck
	}
	assignExternalProvider(projections, f.Codec.ProviderName(), f.Codec.ContextualGlossProviderVersion(), "en")
	deck, _, err := f.Presentation.Freeze(ctx, preparation.OwnerID, deckName, projections,
		cardexport.WithIdiomCards([]string{language}, f.Codec.IdiomCardProviderVersion()),
		cardexport.WithoutIdiomCardBound())
	if err != nil {
		return nil, fmt.Errorf("freeze prepared deck presentation: %w", err)
	}
	return deck.IdiomWork(language), nil
}
