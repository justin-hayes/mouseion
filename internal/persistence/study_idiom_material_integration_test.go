//go:build integration

package persistence

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestStudyIdiomMaterialKeepsItsMatchedTokens holds that an Idiom's ready
// material carries the sentence tokens its headword matched, that a word
// carries none, and that the table refuses a ready Idiom without any (ADR 0137
// "Idiom Study material", ticket #2753).
func TestStudyIdiomMaterialKeepsItsMatchedTokens(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "study-idiom-material", false)
	require.NoError(t, err)

	content := func(matched []int) domain.StudyMaterialReadyContent {
		return domain.StudyMaterialReadyContent{
			BookID: uuid.NewString(), AnalysisRunID: uuid.NewString(), CorpusID: uuid.NewString(), SentenceOrdinal: 4, TokenOrdinal: 3,
			Sentence: "Anna ist nicht mit dem Haus.", TargetWord: "mit dem Haus", Gloss: "at home", SentenceTranslation: "Anna is not at home.",
			MatchedTokenOrdinals: matched,
		}
	}
	idiomID, _, err := store.ReserveStudyMaterial(ctx, owner.ID, domain.IdiomItem("de", "mit dem haus"))
	require.NoError(t, err)
	require.Error(t, store.RecordStudyMaterialReady(ctx, owner.ID, idiomID, content(nil)), "a ready Idiom has matched tokens")
	require.NoError(t, store.RecordStudyMaterialReady(ctx, owner.ID, idiomID, content([]int{3, 4, 5})))
	stored, err := store.GetStudyMaterial(ctx, owner.ID, idiomID)
	require.NoError(t, err)
	assert.Equal(t, []int{3, 4, 5}, stored.MatchedTokenOrdinals)
	listed, err := store.ListCurrentStudyMaterial(ctx, owner.ID, "de")
	require.NoError(t, err)
	require.Len(t, listed, 1)
	assert.Equal(t, []int{3, 4, 5}, listed[0].MatchedTokenOrdinals)

	wordID, _, err := store.ReserveStudyMaterial(ctx, owner.ID, domain.WordItem("de", "haus", "NOUN", ""))
	require.NoError(t, err)
	require.Error(t, store.RecordStudyMaterialReady(ctx, owner.ID, wordID, content([]int{3})), "a word has no matched tokens")
	require.NoError(t, store.RecordStudyMaterialReady(ctx, owner.ID, wordID, content(nil)))
	word, err := store.GetStudyMaterial(ctx, owner.ID, wordID)
	require.NoError(t, err)
	assert.Empty(t, word.MatchedTokenOrdinals)
}
