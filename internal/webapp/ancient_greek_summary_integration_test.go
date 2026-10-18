//go:build integration

package webapp

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// requestsMatching returns a snapshot of every recorded provider request that
// matches the predicate.
func (p *greekPassProvider) requestsMatching(match func(enrichment.TranslationRequest) bool) []enrichment.TranslationRequest {
	p.mu.Lock()
	defer p.mu.Unlock()
	var matched []enrichment.TranslationRequest
	for _, request := range p.requests {
		if match(request) {
			matched = append(matched, request)
		}
	}
	return matched
}

// summaryRequests returns every recorded context-free LSJ sense summary request.
func (f *greekPassFixture) summaryRequests() []enrichment.TranslationRequest {
	return f.provider.requestsMatching(func(request enrichment.TranslationRequest) bool { return request.RequireLSJSenseSummary })
}

// greekSummaryResponse answers a context-free LSJ sense summary request with a
// line citing the entry's first sense, and every pass request through the
// resolved-gloss responder.
func greekSummaryResponse(request enrichment.TranslationRequest) (enrichment.TranslationResponse, error) {
	if request.RequireLSJSenseSummary {
		if len(request.LSJSenses) == 0 {
			return enrichment.TranslationResponse{}, errors.New("summary request carried no senses")
		}
		return enrichment.TranslationResponse{SenseSummary: []enrichment.LSJSenseSummaryLine{{SenseIDs: []string{request.LSJSenses[0].ID}, Text: "word or speech"}}}, nil
	}
	return greekPassResolvedResponse(request)
}

// TestAncientGreekLSJSenseSummaryShownAboveTheEntry proves a recorded summary
// assessment makes the pass request one context-free summary for the pinned
// entry, carrying only the entry's fields, and shows the accepted summary on the
// card above the full entry it condenses.
func TestAncientGreekLSJSenseSummaryShownAboveTheEntry(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	fixture := newGreekPassFixtureWithSummary(t, ctx, "greek-summary-learner", true, true, greekSummaryResponse)
	preparation := fixture.startReadingAndDeck(t, ctx)

	require.Equal(t, 3, preparation.TotalCards)
	summaries := fixture.summaryRequests()
	require.Len(t, summaries, 1, "the context-free summary is requested once for the one pinned entry")
	request := summaries[0]
	assert.True(t, request.RequireLSJSenseSummary)
	assert.False(t, request.RequireEvidenceAssistedPass)
	assert.Equal(t, "λόγος", request.Headword)
	assert.NotEmpty(t, request.LSJSenses)
	for _, sense := range request.LSJSenses {
		assert.NotEmpty(t, sense.ID)
	}
	assert.Empty(t, request.ExampleSentence, "the summary payload carries no Book sentence")
	assert.Empty(t, request.PassageContext, "the summary payload carries no passage context")
	assert.Empty(t, request.CandidateSenses)
	assert.Empty(t, request.TargetWord)

	assert.True(t, bytes.Contains(preparation.Artifact, []byte("Summary of LSJ, provider-written")), "the summary is shown on the card")
	assert.True(t, bytes.Contains(preparation.Artifact, []byte("word or speech")), "the provider-written line is shown")
	assert.True(t, bytes.Contains(preparation.Artifact, []byte("account")), "the full LSJ entry is still frozen on the card")
}

// TestAncientGreekLSJSenseSummaryUnknownSenseIDIsDiscarded proves a summary that
// cites a sense ID that is not in the entry is discarded, so the card keeps its
// full entry and nothing is cached.
func TestAncientGreekLSJSenseSummaryUnknownSenseIDIsDiscarded(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	fixture := newGreekPassFixtureWithSummary(t, ctx, "greek-summary-unknown-learner", true, true, func(request enrichment.TranslationRequest) (enrichment.TranslationResponse, error) {
		if request.RequireLSJSenseSummary {
			return enrichment.TranslationResponse{SenseSummary: []enrichment.LSJSenseSummaryLine{{SenseIDs: []string{"not-a-sense"}, Text: "word or speech"}}}, nil
		}
		return greekPassResolvedResponse(request)
	})
	preparation := fixture.startReadingAndDeck(t, ctx)

	require.Len(t, fixture.summaryRequests(), 1)
	assert.Equal(t, 3, preparation.TotalCards, "a discarded summary still produces every card")
	assert.False(t, bytes.Contains(preparation.Artifact, []byte("Summary of LSJ, provider-written")), "an unknown sense ID discards the summary")
	assert.True(t, bytes.Contains(preparation.Artifact, []byte("account")), "the card keeps its full LSJ entry")
	assert.Equal(t, 0, countGreekSummaryRows(t, ctx, fixture.pool, fixture.codec), "a discarded summary is not cached")
}

// TestAncientGreekLSJSenseSummaryReusedAcrossDecksAndOccurrences proves the
// per-entry cache serves a second preparation without another provider request,
// even when the represented occurrence's pass answer is resolved again.
func TestAncientGreekLSJSenseSummaryReusedAcrossDecksAndOccurrences(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	fixture := newGreekPassFixtureWithSummary(t, ctx, "greek-summary-reuse-learner", true, true, greekSummaryResponse)
	preparation := fixture.startReadingAndDeck(t, ctx)
	require.Len(t, fixture.summaryRequests(), 1)
	assert.Equal(t, 1, countGreekSummaryRows(t, ctx, fixture.pool, fixture.codec))

	// A re-preparation re-resolves the pass answer for the same entry as a new
	// occurrence, but the context-free summary is reused from its per-entry cache.
	_, err := fixture.pool.Exec(ctx, `DELETE FROM enrichment_cache WHERE provider_version=$1`, fixture.codec.EvidenceAssistedPassProviderVersion())
	require.NoError(t, err)
	reading, err := fixture.store.GetCurrentReading(ctx, fixture.learner.ID, "grc")
	require.NoError(t, err)
	_, err = fixture.deck.Reprepare(ctx, fixture.learner.ID, preparation.ID, reading.SnapshotID)
	require.NoError(t, err)
	reprepared := waitForNewPreparedDeckRun(t, ctx, fixture.store, fixture.learner.ID, reading.SnapshotID, preparation.CurrentRunID)

	require.Equal(t, domain.DeckPreparationReady, reprepared.State, "preparation error=%q", reprepared.Error)
	assert.Len(t, fixture.summaryRequests(), 1, "the cached summary is reused rather than re-requested")
	assert.Equal(t, 1, countGreekSummaryRows(t, ctx, fixture.pool, fixture.codec))
	assert.True(t, bytes.Contains(reprepared.Artifact, []byte("Summary of LSJ, provider-written")), "the reused summary is shown on the new deck")
}

// waitForNewPreparedDeckRun waits until the preparation is ready in a generation
// newer than previousRunID.
func waitForNewPreparedDeckRun(t *testing.T, ctx context.Context, store *persistence.PostgresStore, ownerID, snapshotID, previousRunID string) domain.DeckPreparation {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	for {
		preparation, err := store.GetDeckPreparationForSnapshot(ctx, ownerID, snapshotID)
		if err == nil && preparation.CurrentRunID != previousRunID && (preparation.State == domain.DeckPreparationReady || preparation.State == domain.DeckPreparationFailed) {
			return preparation
		}
		if time.Now().After(deadline) {
			t.Fatalf("re-prepared deck did not finish: state=%+v err=%v", preparation, err)
		}
		time.Sleep(25 * time.Millisecond)
	}
}

func countGreekSummaryRows(t *testing.T, ctx context.Context, pool *pgxpool.Pool, codec *enrichment.TranslationCodec) int {
	t.Helper()
	var count int
	require.NoError(t, pool.QueryRow(ctx, `SELECT count(*) FROM lsj_sense_summaries WHERE prompt_version=$1`, codec.LSJSenseSummaryProviderVersion()).Scan(&count))
	return count
}
