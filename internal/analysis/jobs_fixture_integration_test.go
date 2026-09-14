//go:build integration

package analysis

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/analyzer/analyzertest"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/selection"
	"github.com/justin-hayes/mouseion/internal/testutil"
	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestImportedMainTextFixtureAnalysisExcludesAncillaryText(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	url, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	defer store.Close()
	require.NoError(t, MigrateRiver(ctx, pool))
	owner, err := store.CreateUser(ctx, "main-text-e2e", false)
	require.NoError(t, err)

	imported, err := epub.NewService(store).Import(ctx, owner.ID, "de", testutil.ZipDirectory(t, "../epub/testfixtures/epub3-main-text"))
	require.NoError(t, err)
	assert.Contains(t, imported.Book.FullText, "Dies ist Zusatztext.")

	var analyzed []string
	fake := &analyzertest.Fake{AnalyzeFunc: func(_ context.Context, req analyzer.AnalyzeRequest) (analyzer.Result, error) {
		analyzed = append(analyzed, req.Document.Text)
		length := uint64(len([]rune(req.Document.Text)))
		return analyzer.Result{
			SchemaVersion:        "1.0.0",
			Language:             req.Language,
			Analysis:             analyzer.AnalysisProvenance{AnalyzerName: "fixture", AnalyzerVersion: "1"},
			NormalizationProfile: analyzer.NormalizationProfile{Name: "casefold", Version: "1"},
			Sentences: []analyzer.Sentence{{
				Text:     req.Document.Text,
				Location: analyzer.SourceLocation{SourceDocumentID: req.Document.ID, EndOffset: length},
				Tokens: []analyzer.Token{{
					Surface: "inhalt", RawLemma: "inhalt", CanonicalLemma: "inhalt", UPOS: "NOUN", Dependency: "root", Head: 0,
					Location: analyzer.SourceLocation{SourceDocumentID: req.Document.ID, EndOffset: length},
				}},
			}},
		}, nil
	}}
	client, err := NewClient(store.Pool(), fake, analyzertest.ReadyDepparseCapabilityProvider(), selection.NewService(store))
	require.NoError(t, err)
	require.NoError(t, client.Start(ctx))
	defer client.Stop(context.Background())

	handle, err := NewService(store.Pool(), client).SubmitAnalysis(ctx, owner.ID, imported.Source.ID)
	require.NoError(t, err)
	status, err := NewService(store.Pool(), client).Wait(ctx, owner.ID, handle.ID)
	require.NoError(t, err)
	assert.Equal(t, rivertype.JobStateCompleted, status.State)
	assert.Equal(t, []string{"Kapitel eins\n\nDies ist Haupttext eins.", "Kapitel zwei\n\nDies ist Haupttext zwei."}, analyzed)

	rows, err := pool.Query(ctx, `SELECT sentence_text FROM corpus_sentences WHERE owner_id=$1 AND corpus_id=$2 ORDER BY sentence_ordinal`, owner.ID, status.CorpusID)
	require.NoError(t, err)
	defer rows.Close()
	var corpusText []string
	for rows.Next() {
		var text string
		require.NoError(t, rows.Scan(&text))
		corpusText = append(corpusText, text)
	}
	require.NoError(t, rows.Err())
	assert.Equal(t, analyzed, corpusText)
	assert.True(t, strings.HasPrefix(strings.Join(corpusText, " "), "Kapitel eins"))
	assert.NotContains(t, strings.Join(corpusText, " "), "Zusatztext")
}
