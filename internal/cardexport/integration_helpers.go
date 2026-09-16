//go:build integration

package cardexport

import (
	"context"

	"github.com/justin-hayes/mouseion/internal/enrichment"
)

// TestManifest is a small integration-fixture adapter for constructing and
// restoring historical projections without reintroducing the old production
// manifest API.
type TestManifest struct {
	manifest manifest
}

func NewTestManifest(owner, deckName string, entries []Entry) TestManifest {
	return TestManifest{manifest: newManifest(owner, deckName, entries)}
}

func (m TestManifest) Candidates() []enrichment.Candidate {
	return m.manifest.enrichmentCandidatesProjection()
}

func (m TestManifest) BindCacheKeys(keys []enrichment.CacheKey) (TestManifest, error) {
	bound, err := m.manifest.bindCacheKeys(keys)
	if err != nil {
		return TestManifest{}, err
	}
	return TestManifest{manifest: bound}, nil
}

func (m TestManifest) Snapshot() ManifestSnapshot {
	return m.manifest.Snapshot()
}

func RestoreTestManifest(snapshot ManifestSnapshot) (TestManifest, error) {
	manifest, err := manifestFromSnapshot(snapshot)
	if err != nil {
		return TestManifest{}, err
	}
	return TestManifest{manifest: manifest}, nil
}

func RenderTestManifest(ctx context.Context, m TestManifest, exact []ExactEnrichment) (Artifact, error) {
	results := make([]StoredResult, 0, len(exact))
	for _, item := range exact {
		result := item.Result
		results = append(results, StoredResult{
			CacheKey: item.CacheKey,
			Record: enrichment.CacheEntry{
				CacheKey:                  item.CacheKey,
				Translation:               result.Translation.Value,
				FallbackGloss:             result.FallbackGloss.Value,
				SentenceTranslation:       result.SentenceTranslation.Value,
				SentenceTranslationTarget: result.SentenceTranslationTarget.Value,
				SenseSelection:            append([]int(nil), result.SenseSelection.Value...),
			},
		})
	}
	artifact, _, err := NewPresentation(nil).Finalize(ctx, FrozenDeck{manifest: m.manifest}, results, RunFacts{})
	return artifact, err
}
