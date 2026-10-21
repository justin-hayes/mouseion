//go:build integration

package analysis

import (
	"context"
	"encoding/json"
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
	testutil.Cleanup(t, "analysis store", store.Close)
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
	testutil.StopOnCleanup(t, "analysis client", client.Stop)

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

// paragraphAnalyzer returns one sentence per blank-line-separated paragraph,
// with source-relative offsets. It lets the Distributor trim be observed at
// sentence granularity instead of as one sentence per unit.
func paragraphAnalyzer(analyzed *[]string) *analyzertest.Fake {
	return &analyzertest.Fake{AnalyzeFunc: func(_ context.Context, req analyzer.AnalyzeRequest) (analyzer.Result, error) {
		*analyzed = append(*analyzed, req.Document.Text)
		runes := []rune(req.Document.Text)
		sentences := make([]analyzer.Sentence, 0)
		add := func(start, end int) {
			if end <= start {
				return
			}
			sentences = append(sentences, analyzer.Sentence{
				Text:     string(runes[start:end]),
				Location: analyzer.SourceLocation{SourceDocumentID: req.Document.ID, StartOffset: uint64(start), EndOffset: uint64(end)}, //nolint:gosec // rune indexes are non-negative
				Tokens: []analyzer.Token{{
					Surface: "Wort", RawLemma: "Wort", CanonicalLemma: "wort", UPOS: "NOUN", Dependency: "root", Head: 0,
					Location: analyzer.SourceLocation{SourceDocumentID: req.Document.ID, StartOffset: uint64(start), EndOffset: uint64(end)}, //nolint:gosec // rune indexes are non-negative
				}},
			})
		}
		start := 0
		for i := 0; i < len(runes); i++ {
			if runes[i] == '\n' && i+1 < len(runes) && runes[i+1] == '\n' {
				add(start, i)
				i++
				start = i + 1
			}
		}
		add(start, len(runes))
		return analyzer.Result{
			SchemaVersion: "1.0.0", Language: req.Language, Sentences: sentences,
			Analysis:             analyzer.AnalysisProvenance{AnalyzerName: "fixture", AnalyzerVersion: "1"},
			NormalizationProfile: analyzer.NormalizationProfile{Name: "casefold", Version: "1"},
		}, nil
	}}
}

func TestImportedGutenbergFixtureAnalysisExcludesDistributorText(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	url, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, url)
	require.NoError(t, err)
	testutil.Cleanup(t, "analysis store", store.Close)
	require.NoError(t, MigrateRiver(ctx, pool))
	owner, err := store.CreateUser(ctx, "gutenberg-e2e", false)
	require.NoError(t, err)

	imported, err := epub.NewService(store).Import(ctx, owner.ID, "de", testutil.ZipDirectory(t, "../epub/testfixtures/epub3-gutenberg"))
	require.NoError(t, err)
	assert.Contains(t, imported.Book.FullText, "Project Gutenberg licence text.")

	var analyzed []string
	client, err := NewClient(store.Pool(), paragraphAnalyzer(&analyzed), analyzertest.ReadyDepparseCapabilityProvider(), selection.NewService(store))
	require.NoError(t, err)
	require.NoError(t, client.Start(ctx))
	testutil.StopOnCleanup(t, "analysis client", client.Stop)
	service := NewService(store.Pool(), client)

	handle, err := service.SubmitAnalysis(ctx, owner.ID, imported.Source.ID)
	require.NoError(t, err)
	status, err := service.Wait(ctx, owner.ID, handle.ID)
	require.NoError(t, err)
	require.Equal(t, rivertype.JobStateCompleted, status.State, status.Error)
	assert.Len(t, analyzed, 3, "the header and END units are analyzed and trimmed, while later units are excluded")
	assert.NotContains(t, analyzed, "back", "a post-licence illustration unit is excluded whole")

	rows, err := pool.Query(ctx, `SELECT sentence_text,start_offset,end_offset FROM corpus_sentences WHERE owner_id=$1 AND corpus_id=$2 ORDER BY sentence_ordinal`, owner.ID, status.CorpusID)
	require.NoError(t, err)
	defer rows.Close()
	type sentenceRow struct {
		text       string
		start, end int64
	}
	var sentences []sentenceRow
	for rows.Next() {
		var row sentenceRow
		require.NoError(t, rows.Scan(&row.text, &row.start, &row.end))
		sentences = append(sentences, row)
	}
	require.NoError(t, rows.Err())
	texts := make([]string, len(sentences))
	for i, sentence := range sentences {
		texts[i] = sentence.text
	}
	assert.Equal(t, []string{"Erstes Buchkapitel.", "Zweites Buchkapitel.", "Drittes Buchkapitel."}, texts)
	joined := strings.Join(texts, " ")
	for _, unwanted := range []string{"Project Gutenberg", "licence", "back"} {
		assert.NotContains(t, joined, unwanted)
	}

	headerPrefix := "The Project Gutenberg eBook of Beispiel.\n\n" +
		"This eBook is for the use of anyone anywhere.\n\n" +
		"*** START OF THE PROJECT GUTENBERG EBOOK BEISPIEL ***\n\n"
	firstStart := int64(len([]rune(headerPrefix)))
	assert.Equal(t, firstStart, sentences[0].start, "the retained sentence keeps its header-unit offset")
	assert.Equal(t, firstStart+int64(len([]rune("Erstes Buchkapitel."))), sentences[0].end)

	var configIdentity string
	require.NoError(t, pool.QueryRow(ctx, `SELECT config_identity FROM analysis_runs WHERE owner_id=$1 AND id=$2`, owner.ID, handle.RunID).Scan(&configIdentity))
	assert.Equal(t, distributorConfigIdentity, configIdentity)

	var details []byte
	require.NoError(t, pool.QueryRow(ctx, `SELECT details FROM processing_history WHERE owner_id=$1 AND corpus_id=$2 AND operation='analysis' AND status='complete'`, owner.ID, status.CorpusID).Scan(&details))
	var provenance map[string]any
	require.NoError(t, json.Unmarshal(details, &provenance))
	assert.Equal(t, true, provenance["distributor_recognised"])
	assert.Equal(t, epub.DistributorConventionGutenberg, provenance["distributor_convention"])
	assert.Len(t, provenance["distributor_trimmed_ranges"], 2)
	excluded, ok := provenance["distributor_excluded_unit_ids"].([]any)
	require.True(t, ok)
	require.Len(t, excluded, 1)
	var illustrationUnitID string
	require.NoError(t, pool.QueryRow(ctx, `SELECT unit_id FROM source_material_units WHERE owner_id=$1 AND source_material_id=$2 AND manifest_id='illustration'`, owner.ID, imported.Source.ID).Scan(&illustrationUnitID))
	assert.Equal(t, illustrationUnitID, excluded[0])
}
