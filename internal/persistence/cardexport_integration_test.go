//go:build integration

package persistence_test

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetCoverageEntryForBookEncodesFirstEncounterAsBigint(t *testing.T) {
	ctx := context.Background()
	databaseURL, _ := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	defer store.Close()

	owner, err := store.CreateUser(ctx, "coverage-entry-owner", false)
	require.NoError(t, err)
	artifact := domain.NormalizedArtifact{ContentHash: "coverage-entry-hash", Language: "de", SchemaVersion: "1", NormalizationProfile: "test", NormalizationVersion: "1", AnalyzerName: "test", AnalyzerVersion: "1"}
	err = store.PutArtifact(ctx, artifact, []domain.SharedLemma{
		{CanonicalLemma: "Haus", UPOS: "NOUN", Morphology: []byte(`{"Gender":"Neut","Number":"Sing"}`), Frequency: 2},
		{CanonicalLemma: "Haus", UPOS: "NOUN", Morphology: []byte(`{"Gender":"Neut","Number":"Plur"}`), Frequency: 1},
	})
	require.NoError(t, err)
	book, err := store.PutSourceMaterial(ctx, domain.SourceMaterial{OwnerID: owner.ID, Language: "de", SourceIdentifier: "coverage-entry-book", Title: "Coverage Entry Book", MediaType: "text/plain", ContentHash: artifact.ContentHash, Content: []byte("Haus"), FullText: "Haus"})
	require.NoError(t, err)
	corpus, err := store.PutCorpus(ctx, owner.ID, book.ID, artifact.ContentHash)
	require.NoError(t, err)
	candidate := domain.SelectionCandidate{OwnerID: owner.ID, CorpusID: corpus.ID, Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN", OccurrenceCount: 1, ObservedForms: []byte(`["Haus"]`), SentenceReferences: []byte(`[]`), Provenance: []byte(`{}`), FirstEncounter: 59}
	_, err = store.PutSelectionCandidate(ctx, candidate)
	require.NoError(t, err)

	candidates, err := store.ListSelectionCandidatesForCorpus(ctx, owner.ID, corpus.ID)
	require.NoError(t, err)
	require.Len(t, candidates, 1)
	assert.Equal(t, owner.ID, candidates[0].OwnerID, "scoped candidates")
	bookCandidates, err := store.ListSelectionCandidatesForBook(ctx, owner.ID, book.ID)
	require.NoError(t, err)
	require.Len(t, bookCandidates, 1)
	assert.Equal(t, candidate.CorpusID, bookCandidates[0].CorpusID, "book-scoped candidates")

	entry, err := store.GetCoverageEntryForBook(ctx, owner.ID, book.ID, candidate)
	require.NoError(t, err)
	assert.Equal(t, candidate.FirstEncounter, entry.FirstEncounter)
	corpusEntry, err := store.GetCoverageEntryForCorpus(ctx, owner.ID, corpus.ID, candidate)
	require.NoError(t, err)
	assert.Equal(t, candidate.FirstEncounter, corpusEntry.FirstEncounter)
	var morphologies []map[string]string
	err = json.Unmarshal([]byte(entry.Morphology), &morphologies)
	require.NoError(t, err)
	require.Len(t, morphologies, 2)
	assert.Equal(t, "Sing", morphologies[0]["Number"])
	assert.Equal(t, "Plur", morphologies[1]["Number"])

	const exactSentence = "Sie nennt dieses Haus seit vielen Jahren ihr Zuhause."
	const otherSentence = "Das Haus veröffentlicht heute ein neues Buch für Kinder."
	candidate.SentenceReferences, _ = json.Marshal([]map[string]any{{
		"text": exactSentence, "location": map[string]any{"start_offset": 59},
	}})
	candidate.ObservedForms = []byte(`["Haus"]`)
	example, err := store.PutExampleSentence(ctx, owner.ID, corpus.ID, "prepared-sentence", exactSentence, []byte(`{"start_offset":59}`))
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `UPDATE example_sentences SET language='de', canonical_lemma='Haus', upos='NOUN', selection_rank=1, selection_score=1, selection_reasons='[]', is_chosen=true WHERE id=$1`, example.ID)
	require.NoError(t, err)
	when := time.Date(2026, 8, 27, 12, 0, 0, 0, time.UTC)
	_, err = store.Pool().Exec(ctx, `INSERT INTO enrichment_cache(language,canonical_lemma,upos,provider,provider_version,sentence_hash,translation,fallback_gloss,sentence_translation,cached_at) VALUES
		('de','Haus','NOUN','test','1',$1,'home','dwelling','She has called this house her home for many years.',$3),
		('de','Haus','NOUN','test','1',$2,'publisher','publishing house','The publisher is releasing a new children''s book today.',$4)`, enrichment.SentenceHash(exactSentence), enrichment.SentenceHash(otherSentence), when, when.Add(time.Hour))
	require.NoError(t, err)
	entry, err = store.GetCoverageEntryForBook(ctx, owner.ID, book.ID, candidate)
	require.NoError(t, err)
	assert.Equal(t, exactSentence, entry.Sentence)
	assert.Equal(t, "home", entry.Translation)
	assert.Equal(t, "She has called this house her home for many years.", entry.SentenceTranslation)
	assert.Empty(t, entry.SentenceTranslationTarget)

	_, err = store.Pool().Exec(ctx, `INSERT INTO enrichment_cache(language,canonical_lemma,upos,provider,provider_version,sentence_hash,translation,fallback_gloss,sentence_translation,sentence_translation_target,cached_at) VALUES
		('de','Haus','NOUN','chosen','1',$1,'wrong version','','Wrong version sentence.','version',$3),
		('de','Haus','NOUN','other','9',$1,'wrong provider','','Wrong provider sentence.','provider',$4),
		('de','Haus','NOUN','chosen','2',$1,'correct','','This exact house translation is correct.','house',$3),
		('de','Haus','NOUN','chosen','2',$2,'wrong sentence','','Wrong source sentence.','sentence',$4)`, enrichment.SentenceHash(exactSentence), enrichment.SentenceHash(otherSentence), when.Add(-time.Hour), when.Add(2*time.Hour))
	require.NoError(t, err)
	preparedEntry, err := store.GetPreparedCoverageEntryForBook(ctx, owner.ID, book.ID, candidate)
	require.NoError(t, err)
	assert.Empty(t, preparedEntry.Translation, "prepared entry performed a broad cache lookup")
	assert.Empty(t, preparedEntry.SentenceTranslation, "prepared entry performed a broad cache lookup")
	manifest := cardexport.NewManifest(owner.ID, book.Title, []cardexport.Entry{preparedEntry})
	manifestCandidates := manifest.EnrichmentCandidates()
	require.Len(t, manifestCandidates, 1)
	provider := &cacheOnlyProvider{name: "chosen", version: "2"}
	enricher := enrichment.NewService(enrichment.Config{ExternalEnabled: true, UserOptIn: true, ContextMode: enrichment.SentenceContext}, nil, nil, nil, provider, store)
	key, ok := enricher.ExternalCacheKey(manifestCandidates[0])
	require.True(t, ok, "exact cache identity unavailable")
	result, err := enricher.EnrichExternal(ctx, manifestCandidates[0])
	require.NoError(t, err)
	bound, err := manifest.BindCacheKeys([]enrichment.CacheKey{key})
	require.NoError(t, err)
	artifactResult, err := cardexport.NewService(store).RenderManifest(ctx, bound, []cardexport.ExactEnrichment{{CacheKey: key, Result: result}})
	require.NoError(t, err)
	assert.Zero(t, provider.calls)
	assert.True(t, strings.Contains(artifactResult.TSV, "This exact <b>house</b> translation is correct."), "exact artifact missing translation")
	assert.False(t, strings.Contains(artifactResult.TSV, "Wrong"), "exact artifact leaked stale enrichment")
}

type cacheOnlyProvider struct {
	name, version string
	calls         int
}

func (p *cacheOnlyProvider) Name() string    { return p.name }
func (p *cacheOnlyProvider) Version() string { return p.version }
func (p *cacheOnlyProvider) Translate(context.Context, enrichment.TranslationRequest) (enrichment.TranslationResponse, error) {
	p.calls++
	return enrichment.TranslationResponse{}, nil
}
