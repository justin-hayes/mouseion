package prepareddeck

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type recordingContextualGlossFlagStore struct {
	preparation domain.DeckPreparation
	prepErr     error
	run         domain.PreparedDeckRun
	runErr      error
	saved       []domain.LemmaReviewFlag
	saveErr     error
}

func (s *recordingContextualGlossFlagStore) GetDeckPreparation(context.Context, string, string) (domain.DeckPreparation, error) {
	return s.preparation, s.prepErr
}

func (s *recordingContextualGlossFlagStore) GetPreparedDeckRun(context.Context, string, string, string) (domain.PreparedDeckRun, error) {
	return s.run, s.runErr
}

func (s *recordingContextualGlossFlagStore) SaveLemmaReviewFlags(_ context.Context, flags []domain.LemmaReviewFlag) error {
	if s.saveErr != nil {
		return s.saveErr
	}
	s.saved = append(s.saved, flags...)
	return nil
}

func testLemmaMismatchTarget() cardexport.TargetOccurrence {
	return cardexport.TargetOccurrence{CorpusID: "corpus-1", SourceDocumentID: "unit-1", SentenceOrdinal: 7, StartOffset: 23, EndOffset: 28}
}

func TestLemmaMismatchReviewFlagUsesTheOccurrenceAndBoundedProviderEvidence(t *testing.T) {
	preparation := domain.DeckPreparation{OwnerID: "owner-1", BookID: "book-1", AnalysisRunID: "run-1"}
	run := domain.PreparedDeckRun{Provider: "openai-compatible", Model: "gpt-test", ProviderVersion: "gpt-test/translation-v16-citation-form-gloss"}
	flag := lemmaMismatchReviewFlag(preparation, run, testLemmaMismatchTarget(), "  The sentence uses fernsehen, not sehen.  ")

	t.Run("uses the fixed domain reason", func(t *testing.T) {
		require.Equal(t, domain.DeckPreparationLemmaMismatchLabel, flag.Reason)
	})
	t.Run("uses the exact occurrence identity", func(t *testing.T) {
		require.Equal(t, domain.LemmaReviewOccurrence{
			OwnerID: "owner-1", BookID: "book-1", CorpusID: "corpus-1", AnalysisRunID: "run-1",
			SourceDocumentID: "unit-1", SentenceOrdinal: 7, StartOffset: 23, EndOffset: 28,
		}, flag.Occurrence)
	})
	t.Run("records bounded provider provenance", func(t *testing.T) {
		require.Equal(t, domain.LemmaReviewFlagProviderSource, flag.Provenance["source"])
		require.Equal(t, "openai-compatible", flag.Provenance["provider"])
		require.Equal(t, "gpt-test", flag.Provenance["model"])
		require.Equal(t, "translation-v16-citation-form-gloss", flag.Provenance["prompt_version"])
		require.Equal(t, "The sentence uses fernsehen, not sehen.", flag.Provenance["unresolved_reason"])
	})
	t.Run("carries no proposed lemma", func(t *testing.T) {
		require.NotContains(t, flag.Provenance, "proposed_lemma")
		require.NotContains(t, flag.Provenance, "alternative_lemma")
	})
}

func TestLemmaMismatchReviewFlagBoundsProviderEvidence(t *testing.T) {
	preparation := domain.DeckPreparation{OwnerID: "owner-1", BookID: "book-1", AnalysisRunID: "run-1"}
	run := domain.PreparedDeckRun{Provider: "provider", Model: "model", ProviderVersion: "model/contract"}

	flag := lemmaMismatchReviewFlag(preparation, run, testLemmaMismatchTarget(), strings.Repeat("r", 400))
	reason, ok := flag.Provenance["unresolved_reason"].(string)
	require.True(t, ok)
	assert.Equal(t, strings.Repeat("r", 184), reason)
	assert.Equal(t, 184, utf8.RuneCountInString(reason))
}

func TestFrozenContextualGlossPromptContractReadsTheFrozenContract(t *testing.T) {
	tests := map[string]struct {
		run  domain.PreparedDeckRun
		want string
	}{
		"model and contract": {
			run:  domain.PreparedDeckRun{Model: "gpt-test", ProviderVersion: "gpt-test/translation-v16-citation-form-gloss"},
			want: "translation-v16-citation-form-gloss",
		},
		"reasoning suffix is not part of the contract": {
			run:  domain.PreparedDeckRun{Model: "gpt-test", ProviderVersion: "gpt-test/translation-v16-citation-form-gloss-reasoning-high"},
			want: "translation-v16-citation-form-gloss",
		},
		"historical version without the configured model": {
			run:  domain.PreparedDeckRun{Model: "gpt-test", ProviderVersion: "legacy-v1"},
			want: "legacy-v1",
		},
		"empty model keeps the full version": {
			run:  domain.PreparedDeckRun{ProviderVersion: "legacy-v1"},
			want: "legacy-v1",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, test.want, frozenContextualGlossPromptContract(test.run))
		})
	}
}

func TestSaveContextualGlossLemmaMismatchFlagsIsBestEffort(t *testing.T) {
	preparation := domain.DeckPreparation{BookID: "book-1", AnalysisRunID: "run-1"}
	run := domain.PreparedDeckRun{Provider: "provider", Model: "model", ProviderVersion: "model/contract"}
	ctx := context.Background()

	t.Run("persists one flag per target", func(t *testing.T) {
		store := &recordingContextualGlossFlagStore{preparation: preparation, run: run}
		saveContextualGlossLemmaMismatchFlags(ctx, store, "owner-1", "preparation-1", "run-1", []contextualGlossLemmaMismatchOutcome{
			{Ordinal: 0, Target: testLemmaMismatchTarget(), TargetFound: true, UnresolvedReason: "first"},
			{Ordinal: 1, Target: testLemmaMismatchTarget(), TargetFound: true, UnresolvedReason: "second"},
		})
		require.Len(t, store.saved, 2)
		assert.Equal(t, "first", store.saved[0].Provenance["unresolved_reason"])
		assert.Equal(t, "second", store.saved[1].Provenance["unresolved_reason"])
	})

	t.Run("skips an unresolvable target", func(t *testing.T) {
		store := &recordingContextualGlossFlagStore{preparation: preparation, run: run}
		saveContextualGlossLemmaMismatchFlags(ctx, store, "owner-1", "preparation-1", "run-1", []contextualGlossLemmaMismatchOutcome{
			{Ordinal: 0, TargetFound: false, UnresolvedReason: "reason"},
		})
		assert.Empty(t, store.saved)
	})

	t.Run("skips when the preparation cannot be loaded", func(t *testing.T) {
		store := &recordingContextualGlossFlagStore{preparation: preparation, run: run, prepErr: errors.New("database unavailable")}
		saveContextualGlossLemmaMismatchFlags(ctx, store, "owner-1", "preparation-1", "run-1", []contextualGlossLemmaMismatchOutcome{
			{Ordinal: 0, Target: testLemmaMismatchTarget(), TargetFound: true, UnresolvedReason: "reason"},
		})
		assert.Empty(t, store.saved)
	})

	t.Run("skips when the run cannot be loaded", func(t *testing.T) {
		store := &recordingContextualGlossFlagStore{preparation: preparation, run: run, runErr: errors.New("database unavailable")}
		saveContextualGlossLemmaMismatchFlags(ctx, store, "owner-1", "preparation-1", "run-1", []contextualGlossLemmaMismatchOutcome{
			{Ordinal: 0, Target: testLemmaMismatchTarget(), TargetFound: true, UnresolvedReason: "reason"},
		})
		assert.Empty(t, store.saved)
	})

	t.Run("a rejected or failed write never propagates", func(t *testing.T) {
		store := &recordingContextualGlossFlagStore{preparation: preparation, run: run, saveErr: domain.ErrNotFound}
		saveContextualGlossLemmaMismatchFlags(ctx, store, "owner-1", "preparation-1", "run-1", []contextualGlossLemmaMismatchOutcome{
			{Ordinal: 0, Target: testLemmaMismatchTarget(), TargetFound: true, UnresolvedReason: "reason"},
		})
		assert.Empty(t, store.saved)
	})
}
