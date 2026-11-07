package prepareddeck

import (
	"context"
	"log"
	"strings"

	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// contextualGlossLemmaMismatchOutcome is one provider outcome that should raise
// an advisory occurrence flag.
type contextualGlossLemmaMismatchOutcome struct {
	Ordinal          int
	Target           cardexport.TargetOccurrence
	TargetFound      bool
	UnresolvedReason string
}

// contextualGlossFlagStore is the prepared-deck store surface needed to resolve
// the Book and record one advisory flag. The write happens after the outcome is
// terminal, so every error is logged and discarded rather than propagated.
type contextualGlossFlagStore interface {
	GetDeckPreparation(context.Context, string, string) (domain.DeckPreparation, error)
	GetPreparedDeckRun(context.Context, string, string, string) (domain.PreparedDeckRun, error)
	SaveLemmaReviewFlags(context.Context, []domain.LemmaReviewFlag) error
}

// saveContextualGlossLemmaMismatchFlags records one advisory flag per
// lemma-mismatch outcome. It runs after the outcome's terminal write, never
// changes the outcome, and skips an unresolvable or stale occurrence.
func saveContextualGlossLemmaMismatchFlags(ctx context.Context, store contextualGlossFlagStore, owner, preparationID, runID string, outcomes []contextualGlossLemmaMismatchOutcome) {
	if store == nil || len(outcomes) == 0 {
		return
	}
	preparation, err := store.GetDeckPreparation(ctx, owner, preparationID)
	if err != nil {
		log.Printf("prepared deck lemma review flag: load preparation: %v", err)
		return
	}
	run, err := store.GetPreparedDeckRun(ctx, owner, preparationID, runID)
	if err != nil {
		log.Printf("prepared deck lemma review flag: load run: %v", err)
		return
	}
	for _, outcome := range outcomes {
		if !outcome.TargetFound {
			log.Printf("prepared deck lemma review flag: target occurrence unavailable for ordinal %d", outcome.Ordinal)
			continue
		}
		flag := lemmaMismatchReviewFlag(preparation, run, outcome.Target, outcome.UnresolvedReason)
		if err := store.SaveLemmaReviewFlags(ctx, []domain.LemmaReviewFlag{flag}); err != nil {
			log.Printf("prepared deck lemma review flag: save ordinal %d: %v", outcome.Ordinal, err)
		}
	}
}

// lemmaMismatchReviewFlag builds the advisory flag. The reason is the fixed
// domain label; the provider's own text is bounded evidence, never a proposed
// lemma or proof.
func lemmaMismatchReviewFlag(preparation domain.DeckPreparation, run domain.PreparedDeckRun, target cardexport.TargetOccurrence, unresolvedReason string) domain.LemmaReviewFlag {
	return domain.LemmaReviewFlag{
		Occurrence: domain.LemmaReviewOccurrence{
			OwnerID: preparation.OwnerID, BookID: preparation.BookID, CorpusID: target.CorpusID,
			AnalysisRunID: preparation.AnalysisRunID, SourceDocumentID: target.SourceDocumentID,
			SentenceOrdinal: target.SentenceOrdinal, StartOffset: target.StartOffset, EndOffset: target.EndOffset,
		},
		Reason: domain.DeckPreparationLemmaMismatchLabel,
		Provenance: map[string]any{
			"source":            domain.LemmaReviewFlagProviderSource,
			"provider":          strings.TrimSpace(run.Provider),
			"model":             strings.TrimSpace(run.Model),
			"prompt_version":    frozenContextualGlossPromptContract(run),
			"unresolved_reason": domain.DeckPreparationLemmaMismatchDetail(unresolvedReason),
		},
	}
}

// frozenContextualGlossPromptContract returns the prompt-contract portion of
// the frozen provider version. The contextual-Gloss identity is
// "<model>/<prompt-contract>[-reasoning-<effort>]"; a historical or custom
// version is recorded unchanged.
func frozenContextualGlossPromptContract(run domain.PreparedDeckRun) string {
	version := strings.TrimSpace(run.ProviderVersion)
	model := strings.TrimSpace(run.Model)
	if model == "" {
		return version
	}
	promptVersion, ok := strings.CutPrefix(version, model+"/")
	if !ok {
		return version
	}
	if contract, _, ok := strings.Cut(promptVersion, "-reasoning-"); ok {
		return contract
	}
	return promptVersion
}
