package cardexport_test

import (
	"math"
	"testing"

	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/stretchr/testify/require"
)

// TestManifestSnapshotRejectsInvalidItems pins each manifest-item
// canonicalization rule through the public digest seam. Splitting the item rule
// into named steps must not change which malformed item is rejected, or the
// message that names the rule.
func TestManifestSnapshotRejectsInvalidItems(t *testing.T) {
	tests := []struct {
		name   string
		want   string
		mutate func(*cardexport.ManifestSnapshot)
	}{
		{
			name: "external or incomplete entry fields",
			want: "invalid or external manifest entry fields",
			mutate: func(snapshot *cardexport.ManifestSnapshot) {
				snapshot.Items[0].Entry.OwnerID = "owner-1"
			},
		},
		{
			name: "invalid disposition",
			want: "invalid manifest disposition",
			mutate: func(snapshot *cardexport.ManifestSnapshot) {
				snapshot.Items[0].Disposition = "dropped"
			},
		},
		{
			name: "disposition contradicts quality decision",
			want: "disposition contradicts quality decision",
			mutate: func(snapshot *cardexport.ManifestSnapshot) {
				snapshot.Items[0].Quality.Accepted = false
			},
		},
		{
			name: "quality score out of range",
			want: "invalid manifest quality result",
			mutate: func(snapshot *cardexport.ManifestSnapshot) {
				snapshot.Items[0].Quality.Score = 200
			},
		},
		{
			name: "too many quality reasons",
			want: "invalid manifest quality result",
			mutate: func(snapshot *cardexport.ManifestSnapshot) {
				snapshot.Items[0].Quality.Reasons = make([]string, 17)
			},
		},
		{
			name: "invalid GDEX score",
			want: "invalid manifest GDEX score",
			mutate: func(snapshot *cardexport.ManifestSnapshot) {
				snapshot.Items[0].Quality.GDEXScore = math.NaN()
			},
		},
		{
			name: "unknown quality reason",
			want: "unknown manifest quality reason",
			mutate: func(snapshot *cardexport.ManifestSnapshot) {
				snapshot.Items[0].Quality.Reasons = []string{"invented reason"}
			},
		},
		{
			name: "non-canonical target",
			want: "manifest target is not canonical",
			mutate: func(snapshot *cardexport.ManifestSnapshot) {
				snapshot.Items[0].Entry.TargetWord = "  Haus  "
			},
		},
		{
			name: "cache identity does not match entry",
			want: "cache identity does not match manifest entry",
			mutate: func(snapshot *cardexport.ManifestSnapshot) {
				snapshot.Items[0].CacheKey.Language = "fr"
			},
		},
		{
			name: "too many candidate senses",
			want: "too many manifest candidate senses",
			mutate: func(snapshot *cardexport.ManifestSnapshot) {
				senses := make([]enrichment.LexicalSense, enrichment.DefaultMaxCandidateSenses+1)
				snapshot.Items[0].Entry.CandidateSenses = senses
			},
		},
		{
			name: "invalid omitted evidence count",
			want: "invalid omitted evidence count",
			mutate: func(snapshot *cardexport.ManifestSnapshot) {
				snapshot.Items[0].Entry.OmittedEvidenceCount = -1
			},
		},
		{
			name: "speaker attribution without a name",
			want: "speaker attribution has no name",
			mutate: func(snapshot *cardexport.ManifestSnapshot) {
				snapshot.Items[0].Entry.Attribution = domain.SourceAttribution{Kind: domain.AttributionSpeaker}
			},
		},
		{
			name: "stage direction attribution with a speaker",
			want: "stage direction attribution carries a speaker",
			mutate: func(snapshot *cardexport.ManifestSnapshot) {
				snapshot.Items[0].Entry.Attribution = domain.SourceAttribution{Kind: domain.AttributionStageDirection, Speaker: "Erster"}
			},
		},
		{
			name: "unknown attribution kind",
			want: "unknown attribution kind",
			mutate: func(snapshot *cardexport.ManifestSnapshot) {
				snapshot.Items[0].Entry.Attribution = domain.SourceAttribution{Kind: "chorus"}
			},
		},
		{
			name: "speaker without an attribution kind",
			want: "attribution speaker has no kind",
			mutate: func(snapshot *cardexport.ManifestSnapshot) {
				snapshot.Items[0].Entry.Attribution = domain.SourceAttribution{Speaker: "Erster"}
			},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			snapshot := lifecycleRestoreSnapshot()
			test.mutate(&snapshot)
			_, err := snapshot.Digest()
			require.ErrorIs(t, err, cardexport.ErrInvalidInput)
			require.ErrorContains(t, err, test.want)
		})
	}
}
