package cardexport_test

import (
	"math"
	"testing"

	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/stretchr/testify/require"
)

// TestManifestSnapshotRejectsInvalidItems pins each manifest-item
// canonicalization rule through the public digest seam. Splitting the item rule
// into named steps must not change which malformed item is rejected.
func TestManifestSnapshotRejectsInvalidItems(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*cardexport.ManifestSnapshot)
	}{
		{
			name: "external or incomplete entry fields",
			mutate: func(snapshot *cardexport.ManifestSnapshot) {
				snapshot.Items[0].Entry.OwnerID = "owner-1"
			},
		},
		{
			name: "invalid disposition",
			mutate: func(snapshot *cardexport.ManifestSnapshot) {
				snapshot.Items[0].Disposition = "dropped"
			},
		},
		{
			name: "disposition contradicts quality decision",
			mutate: func(snapshot *cardexport.ManifestSnapshot) {
				snapshot.Items[0].Quality.Accepted = false
			},
		},
		{
			name: "quality score out of range",
			mutate: func(snapshot *cardexport.ManifestSnapshot) {
				snapshot.Items[0].Quality.Score = 200
			},
		},
		{
			name: "too many quality reasons",
			mutate: func(snapshot *cardexport.ManifestSnapshot) {
				snapshot.Items[0].Quality.Reasons = make([]string, 17)
			},
		},
		{
			name: "invalid GDEX score",
			mutate: func(snapshot *cardexport.ManifestSnapshot) {
				snapshot.Items[0].Quality.GDEXScore = math.NaN()
			},
		},
		{
			name: "unknown quality reason",
			mutate: func(snapshot *cardexport.ManifestSnapshot) {
				snapshot.Items[0].Quality.Reasons = []string{"invented reason"}
			},
		},
		{
			name: "non-canonical target",
			mutate: func(snapshot *cardexport.ManifestSnapshot) {
				snapshot.Items[0].Entry.TargetWord = "  Haus  "
			},
		},
		{
			name: "cache identity does not match entry",
			mutate: func(snapshot *cardexport.ManifestSnapshot) {
				snapshot.Items[0].CacheKey.Language = "fr"
			},
		},
		{
			name: "too many candidate senses",
			mutate: func(snapshot *cardexport.ManifestSnapshot) {
				senses := make([]enrichment.LexicalSense, enrichment.DefaultMaxCandidateSenses+1)
				snapshot.Items[0].Entry.CandidateSenses = senses
			},
		},
		{
			name: "invalid omitted evidence count",
			mutate: func(snapshot *cardexport.ManifestSnapshot) {
				snapshot.Items[0].Entry.OmittedEvidenceCount = -1
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := lifecycleRestoreSnapshot()
			test.mutate(&snapshot)
			_, err := snapshot.Digest()
			require.ErrorIs(t, err, cardexport.ErrInvalidInput)
		})
	}
}
