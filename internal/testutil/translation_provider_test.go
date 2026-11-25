package testutil_test

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFakeTranslationProviderScriptsResponsesAndCountsCalls(t *testing.T) {
	fake := testutil.NewFakeTranslationProvider(
		testutil.TranslationReply{Response: enrichment.TranslationResponse{Translation: "house", Gloss: "a building"}},
		testutil.TranslationReply{Response: enrichment.TranslationResponse{UnresolvedKind: enrichment.ContextualGlossUnresolvedAmbiguous}},
	)
	var _ enrichment.TranslationProvider = fake
	assert.True(t, fake.Enabled())

	first, err := fake.Translate(context.Background(), enrichment.TranslationRequest{CanonicalLemma: "haus"})
	require.NoError(t, err)
	assert.Equal(t, "house", first.Translation)
	second, err := fake.Translate(context.Background(), enrichment.TranslationRequest{})
	require.NoError(t, err)
	assert.Equal(t, enrichment.ContextualGlossUnresolvedAmbiguous, second.UnresolvedKind)
	// The last scripted reply repeats.
	third, err := fake.Translate(context.Background(), enrichment.TranslationRequest{})
	require.NoError(t, err)
	assert.Equal(t, enrichment.ContextualGlossUnresolvedAmbiguous, third.UnresolvedKind)

	assert.Equal(t, 3, fake.Calls())
	assert.Equal(t, "haus", fake.Requests()[0].CanonicalLemma)
}

func TestFakeTranslationProviderReturnsScriptedError(t *testing.T) {
	boom := assert.AnError
	fake := testutil.NewFakeTranslationProvider(testutil.TranslationReply{Err: boom})
	_, err := fake.Translate(context.Background(), enrichment.TranslationRequest{})
	assert.ErrorIs(t, err, boom)
}

func TestFakeTranslationProviderDisabled(t *testing.T) {
	fake := testutil.NewFakeTranslationProvider()
	fake.SetEnabled(false)
	assert.False(t, fake.Enabled())
}
