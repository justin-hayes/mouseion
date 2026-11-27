//go:build integration

package persistence

import (
	"context"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/analysisinsights"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/checked"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/selection"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type coverageParityToken struct {
	lemma, upos, dependency string
}

// TestMyBooksCoverageMatchesSharedSelectionProjection pins the Browse-count
// derived coverage to the shared selection projection on fixtures that stress
// every eligibility rule: lemma corrections, exclusions, separable-verb
// particles, non-letter lemmas, and padded or lower-case UPOS values.
func TestMyBooksCoverageMatchesSharedSelectionProjection(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "coverage-parity", false)
	require.NoError(t, err)

	offset := func(value int64) uint64 {
		t.Helper()
		converted, convertErr := checked.Uint64FromInt64(value)
		require.NoError(t, convertErr)
		return converted
	}
	const unitManifest = "coverage-parity"
	unitID := domain.EPUBUnitID(0, unitManifest)
	tokens := []coverageParityToken{
		{"haus", "NOUN", "root"}, {"haus", "NOUN", "root"}, {"haus", "NOUN", "root"},
		{"baum", "NOUN", "root"}, {"baum", "NOUN", "root"}, {"baum", "NOUN", "root"},
		{"auf", "ADV", "compound:prt"},
		{"123", "NOUN", "root"}, {"123", "NOUN", "root"},
		{" laufen ", "VERB", "root"}, {"laufen", "VERB", "root"},
		{"tisch", " noun ", ""}, {"tisch", "noun", ""},
		{"straße", "ADJ", "root"}, {"straße", "ADJ", "root"},
		{"der", "DET", "root"},
		{"stuhl", "NOUN", "root"}, {"stuhl", "NOUN", "root"},
	}
	for i := range tokens {
		if tokens[i].dependency == "" {
			tokens[i].dependency = "root"
		}
	}
	var text strings.Builder
	sentence := concordanceSentence{UnitID: unitID, Ordinal: 0}
	analysis := analyzer.Result{Language: "de", Sentences: []analyzer.Sentence{{}}}
	for i, token := range tokens {
		start := int64(i) * 10
		sentence.Tokens = append(sentence.Tokens, concordanceToken{Surface: token.lemma, Lemma: token.lemma, Upos: token.upos, Dependency: token.dependency, Start: start, End: start + 5})
		analysis.Sentences[0].Tokens = append(analysis.Sentences[0].Tokens, analyzer.Token{
			Surface: token.lemma, CanonicalLemma: token.lemma, UPOS: token.upos, Dependency: token.dependency,
			Location: analyzer.SourceLocation{SourceDocumentID: unitID, StartOffset: offset(start), EndOffset: offset(start + 5)},
		})
		text.WriteString(strings.Repeat("x", 10))
	}
	sentence.Text, sentence.Start, sentence.End = text.String(), 0, int64(text.Len())
	book, source := createConcordanceBook(t, ctx, store, owner.ID, "Coverage parity", "coverage-parity", true,
		[]domain.ExtractedUnit{concordanceUnit(0, unitManifest, sentence.Text, 0, offset(int64(text.Len())))})
	insertConcordanceAnalysis(t, ctx, store, source, true, []concordanceSentence{sentence})
	var runID, corpusID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text,corpus_id::text FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID).Scan(&runID, &corpusID))

	// Token 3 (baum) is corrected into Known haus; token 4 (baum) is excluded.
	// Token 9 (" laufen ") is corrected to a padded lemma.
	var decisions []selection.OccurrenceDecision
	decide := func(index int, lemma string, excluded bool) {
		var lemmaArg any
		if !excluded {
			lemmaArg = lemma
		}
		start := int64(index) * 10
		_, execErr := store.Pool().Exec(ctx, `INSERT INTO occurrence_lemma_corrections(owner_id,book_id,corpus_id,analysis_run_id,source_document_id,start_offset,end_offset,canonical_lemma,normalization_profile,normalization_version,excluded) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11)`,
			owner.ID, book.ID, corpusID, runID, unitID, start, start+5, lemmaArg, nilIf(excluded, "german-post-1996"), nilIf(excluded, "6"), excluded)
		require.NoError(t, execErr)
		decision := selection.OccurrenceDecision{Occurrence: selection.OccurrenceIdentity{SourceDocumentID: unitID, StartOffset: offset(start), EndOffset: offset(start + 5)}, Excluded: excluded}
		if !excluded {
			decision.Lemma = lemma
		}
		decisions = append(decisions, decision)
	}
	decide(3, "haus", false)
	decide(4, "", true)
	decide(9, " stuhl ", false)

	candidates, err := selection.Project(analysis, selection.DefaultConfig(corpusID), nil)
	require.NoError(t, err)
	var analyzable int64
	for _, candidate := range candidates {
		analyzable += int64(candidate.OccurrenceCount)
	}
	_, err = store.Pool().Exec(ctx, `UPDATE corpora SET analyzable_token_count=$3, distinct_lemma_count=$4 WHERE owner_id=$1 AND id=$2`, owner.ID, corpusID, analyzable, len(candidates))
	require.NoError(t, err)

	for _, mature := range []struct{ lemma, upos string }{{"haus", "NOUN"}, {"laufen", "VERB"}, {"tisch", "NOUN"}, {"straße", "ADJ"}, {"stuhl", "NOUN"}} {
		seedCoverageStudyWord(t, ctx, store, owner.ID, "de", mature.lemma, mature.upos, true)
	}
	// baum is in study but not Mature: it counts only in the lighter figure.
	seedCoverageStudyWord(t, ctx, store, owner.ID, "de", "baum", "NOUN", false)

	tx, err := store.Pool().Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, BuildVocabularyBrowseCountsTx(ctx, tx, owner.ID, book.ID, source.ID, runID, corpusID, "de"))
	require.NoError(t, tx.Commit(ctx))

	projected, err := selection.Project(analysis, selection.DefaultConfig(corpusID), decisions)
	require.NoError(t, err)
	matureIdentities := map[selection.Identity]bool{}
	for _, identity := range []selection.Identity{
		{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN"}, {Language: "de", CanonicalLemma: "laufen", UPOS: "VERB"},
		{Language: "de", CanonicalLemma: "tisch", UPOS: "NOUN"}, {Language: "de", CanonicalLemma: "stuhl", UPOS: "NOUN"},
		{Language: "de", CanonicalLemma: "straße", UPOS: "ADJ"},
	} {
		matureIdentities[identity] = true
	}
	var wantMature, wantInStudy int64
	for _, candidate := range projected {
		id := candidate.Identity
		if matureIdentities[id] {
			wantMature += int64(candidate.OccurrenceCount)
			wantInStudy += int64(candidate.OccurrenceCount)
		}
		if id.CanonicalLemma == "baum" && id.Language == "de" {
			wantInStudy += int64(candidate.OccurrenceCount)
		}
	}
	require.Positive(t, wantMature)
	require.Greater(t, wantInStudy, wantMature)

	browse, err := store.ListMyBooksBrowse(ctx, owner.ID, "", "de", "", false, 0, 25)
	require.NoError(t, err)
	require.Len(t, browse.Items, 1)
	assert.Equal(t, wantMature, browse.Items[0].CoverageMatureTokens, "Browse-count coverage must equal the shared selection projection")
	assert.Equal(t, wantInStudy, browse.Items[0].CoverageInStudyTokens)
	insights, err := analysisinsights.NewService(store).Coverage(ctx, owner.ID, corpusID)
	require.NoError(t, err)
	assert.Equal(t, wantMature, insights.MatureTokenCount, "Analysis Insights agrees with My Books")
	assert.Equal(t, wantInStudy, insights.InStudyTokenCount)
	assert.Equal(t, analyzable, browse.Items[0].CoverageTotalTokens)

	_, err = store.Pool().Exec(ctx, `DELETE FROM vocabulary_browse_count_readiness WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID)
	require.NoError(t, err)
	unready, err := store.ListMyBooksBrowse(ctx, owner.ID, "", "de", "", false, 0, 25)
	require.NoError(t, err)
	require.Len(t, unready.Items, 1)
	assert.Zero(t, unready.Items[0].CoverageMatureTokens, "unready counts never fall back to a token scan")
	assert.Zero(t, unready.Items[0].CoverageTotalTokens)
}

func nilIf(condition bool, value string) any {
	if condition {
		return nil
	}
	return value
}

// seedCoverageStudyWord puts a word in the learner's study: Mature (in review at
// the maturity interval) or still learning.
func seedCoverageStudyWord(t *testing.T, ctx context.Context, store *PostgresStore, owner, language, lemma, upos string, mature bool) {
	t.Helper()
	state, days := "learning", int64(0)
	if mature {
		state, days = "review", domain.FluencyMaturityDays
	}
	_, err := store.Pool().Exec(ctx, `
WITH m AS (
  INSERT INTO study_material (owner_id, language, canonical_lemma, upos, state, omission_reason)
  VALUES ($1, $2, $3, $4, 'unpreparable', 'sentence-quality') RETURNING id)
INSERT INTO study_progress (owner_id, language, canonical_lemma, upos, material_id, introduced_at, learned_meaning, state, due, stability, difficulty, scheduled_days, reps, last_review)
SELECT $1, $2, $3, $4, id, now(), 'seeded', $5, now(), 5, 5, $6, 1, now() FROM m`, owner, language, lemma, upos, state, days)
	require.NoError(t, err)
}
