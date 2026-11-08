package main

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"testing"

	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/stretchr/testify/require"
)

// rankedIdioms builds an in-memory freeze's idiom ranking: count idioms whose
// last nounOnly entries are noun-only, as the bound orders them.
func rankedIdioms(count, nounOnly int) []cardexport.IdiomRank {
	ranking := make([]cardexport.IdiomRank, 0, count)
	for i := range count {
		ranking = append(ranking, cardexport.IdiomRank{
			Language: "it", Headword: fmt.Sprintf("idiom-%02d", i),
			AnchorCanonicalLemma: "gamba", AnchorUPOS: "NOUN", AnchorOccurrences: count - i,
			NounOnly: i >= count-nounOnly,
		})
	}
	return ranking
}

func renderIdiomReportWithRanking(t *testing.T, ranking []cardexport.IdiomRank) string {
	t.Helper()
	client := stubIdiomClient(t, func(idiomProviderRequestPayload) idiomStubAnswer {
		return idiomStubAnswer{translation: "review", gloss: "review"}
	})
	path := filepath.Join(t.TempDir(), "idiom-review.md")
	options, err := parseReviewCommand([]string{"-review", "idiom", "-out", path, "-date", "2026-11-08", "-book", "book-1"})
	require.NoError(t, err)
	source := &fakeIdiomSampleSource{items: sampleItems(3), ranking: ranking}
	require.NoError(t, executeIdiomCommand(context.Background(), options, io.Discard,
		func() (reviewEnvironment, error) {
			return reviewEnvironment{Client: client, Provider: idiomTestProviderMetadata()}, nil
		},
		func(context.Context, *enrichment.TranslationCodec) (idiomSampleSource, error) { return source, nil }))
	rendered, err := os.ReadFile(path) //nolint:gosec // path is inside t.TempDir().
	require.NoError(t, err)
	return string(rendered)
}

func TestIdiomReportListsTheIdiomsAPreparedDeckKeeps(t *testing.T) {
	report := renderIdiomReportWithRanking(t, rankedIdioms(32, 3))

	t.Run("explains the section", func(t *testing.T) {
		require.Contains(t, report, "## Prepared-deck bound\n\n"+
			"The sample above ignores the per-deck bound. A prepared deck keeps the first 30 of the 32 ranked idioms; "+
			"noun-only idioms rank after every other idiom. This list is not a gate criterion.\n\n"+
			"| Rank | Headword | Anchor | Anchor occurrences | Noun-only |\n| --- | --- | --- | --- | --- |\n"+
			"| 1 | `idiom-00` | `gamba` (`NOUN`) | 32 | — |\n")
	})
	t.Run("marks kept noun-only idioms", func(t *testing.T) {
		require.Contains(t, report, "| 30 | `idiom-29` | `gamba` (`NOUN`) | 3 | yes |\n\n- Idiom overflow: 2\n")
	})
	t.Run("leaves overflowed idioms out", func(t *testing.T) {
		require.NotContains(t, report, "`idiom-30`")
	})
}

func TestIdiomReportShowsEveryIdiomWhenTheBoundIsNotReached(t *testing.T) {
	report := renderIdiomReportWithRanking(t, rankedIdioms(2, 0))

	require.Contains(t, report, "A prepared deck keeps all 2 ranked idioms;")
	require.Contains(t, report, "| 2 | `idiom-01` | `gamba` (`NOUN`) | 1 | — |\n\n- Idiom overflow: 0\n")
}

func TestIdiomReportSaysWhenTheBookMatchedNoIdioms(t *testing.T) {
	report := renderIdiomReportWithRanking(t, nil)

	require.Contains(t, report, "## Prepared-deck bound\n\nThe Book matched no idioms.\n")
}
