//go:build integration

package webapp

import (
	"archive/zip"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/justin-hayes/mouseion/internal/analysis"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/analyzer/analyzertest"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/checked"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/prepareddeck"
	"github.com/justin-hayes/mouseion/internal/selection"
	"github.com/justin-hayes/mouseion/internal/testutil"
	_ "modernc.org/sqlite"

	"github.com/riverqueue/river/rivertype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const faustExcerptDeck = "Faust: Eine Tragödie [erster Teil]"

// curatedToken is one analyzer token the supplied fixture declares for an
// excerpt sentence. Root marks the single self-headed finite verb the German
// sentence-quality gate requires.
type curatedToken struct {
	surface, lemma, upos, dependency string
	root                             bool
}

// curatedSentence locates a curated sentence inside its exact extracted unit
// and returns analyzer evidence whose code-point offsets are absolute in that
// unit, the shape the production RPC returns.
func curatedSentence(t *testing.T, unit domain.ExtractedUnit, text string, specs []curatedToken) analyzer.Sentence {
	t.Helper()
	byteStart := strings.Index(unit.Text, text)
	require.GreaterOrEqual(t, byteStart, 0, "curated sentence is absent from %q", unit.ManifestID)
	start, err := checked.Uint64FromInt(utf8.RuneCountInString(unit.Text[:byteStart]))
	require.NoError(t, err)
	length, err := checked.Uint64FromInt(utf8.RuneCountInString(text))
	require.NoError(t, err)
	end := start + length

	root := 0
	for index, spec := range specs {
		if spec.root {
			root = index
			break
		}
	}
	tokens := make([]analyzer.Token, 0, len(specs))
	cursor := 0
	for index, spec := range specs {
		byteIndex := strings.Index(text[cursor:], spec.surface)
		require.GreaterOrEqual(t, byteIndex, 0, "curated token %q is absent from %q", spec.surface, text)
		tokenByteStart := cursor + byteIndex
		tokenOffset, err := checked.Uint64FromInt(utf8.RuneCountInString(text[:tokenByteStart]))
		require.NoError(t, err)
		tokenLength, err := checked.Uint64FromInt(utf8.RuneCountInString(spec.surface))
		require.NoError(t, err)
		tokenStart := start + tokenOffset
		tokenEnd := tokenStart + tokenLength
		head, err := checked.Uint32FromInt(root)
		require.NoError(t, err)
		if spec.root {
			head, err = checked.Uint32FromInt(index)
			require.NoError(t, err)
		}
		morphology := map[string]string{}
		if spec.root {
			morphology["VerbForm"] = "Fin"
		}
		tokens = append(tokens, analyzer.Token{
			Surface: spec.surface, RawLemma: spec.lemma, CanonicalLemma: strings.ToLower(spec.lemma),
			UPOS: spec.upos, Dependency: spec.dependency, Head: head, Morphology: morphology,
			Location: analyzer.SourceLocation{SourceDocumentID: unit.ID, StartOffset: tokenStart, EndOffset: tokenEnd},
		})
		cursor = tokenByteStart + len(spec.surface)
	}
	return analyzer.Sentence{
		Text:     text,
		Location: analyzer.SourceLocation{SourceDocumentID: unit.ID, StartOffset: start, EndOffset: end},
		Tokens:   tokens,
	}
}

// faustExcerptAnalysis is the supplied analyzer for the committed Faust
// excerpts. It intentionally declares the measured cross-line Director sentence,
// the Thule stanzas, the correction reading, speaker labels and two sentences
// that share one source line; it never infers a boundary from markup.
func faustExcerptAnalysis(t *testing.T, extraction epub.Extraction) func(context.Context, analyzer.AnalyzeRequest) (analyzer.Result, error) {
	t.Helper()
	units := make(map[string]domain.ExtractedUnit, len(extraction.Book.ExtractedUnits.Units))
	manifests := make(map[string]string, len(extraction.Book.ExtractedUnits.Units))
	for _, unit := range extraction.Book.ExtractedUnits.Units {
		units[unit.ManifestID] = unit
		manifests[unit.ID] = unit.ManifestID
	}

	results := map[string][]analyzer.Sentence{
		"excerpt-0": {
			curatedSentence(t, units["excerpt-0"], curatorSentenceText, []curatedToken{
				{surface: "Ihr", lemma: "ihr", upos: "PRON", dependency: "nsubj"},
				{surface: "Unternehmung", lemma: "Unternehmung", upos: "NOUN", dependency: "obj"},
				{surface: "hofft", lemma: "hoffen", upos: "VERB", dependency: "root", root: true},
			}),
		},
		"excerpt-2": {
			curatedSentence(t, units["excerpt-2"], "Nachdem die Löcher alle gebohrt und verstopft sind, Mephistopheles mit seltsamen Geberden.", []curatedToken{
				{surface: "Löcher", lemma: "Loch", upos: "NOUN", dependency: "obj"},
				{surface: "sind", lemma: "sein", upos: "AUX", dependency: "root", root: true},
				{surface: "Mephistopheles", lemma: "Mephistopheles", upos: "PROPN", dependency: "nsubj"},
				{surface: "Geberden", lemma: "Geberde", upos: "NOUN", dependency: "obl"},
			}),
		},
		"excerpt-3": {
			curatedSentence(t, units["excerpt-3"], directionStandalone, []curatedToken{
				{surface: "Sie", lemma: "sie", upos: "PRON", dependency: "nsubj"},
				{surface: "singen", lemma: "singen", upos: "VERB", dependency: "root", root: true},
			}),
			curatedSentence(t, units["excerpt-3"], "Es war ein König in Thule\nGar treu bis an das Grab,\nDem sterbend seine Buhle\nEinen goldnen Becher gab.", []curatedToken{
				{surface: "war", lemma: "sein", upos: "AUX", dependency: "root", root: true},
				{surface: "König", lemma: "König", upos: "NOUN", dependency: "nsubj"},
				{surface: "Becher", lemma: "Becher", upos: "NOUN", dependency: "obj"},
			}),
			curatedSentence(t, units["excerpt-3"], "Es ging ihm nichts darüber,\nEr leert ihn jeden Schmaus;\nDie Augen gingen ihm über,\nSo oft er trank daraus.", []curatedToken{
				{surface: "Augen", lemma: "Auge", upos: "NOUN", dependency: "nsubj"},
				{surface: "gingen", lemma: "gehen", upos: "VERB", dependency: "root", root: true},
			}),
			curatedSentence(t, units["excerpt-3"], "Und als er kam zu sterben,\nZählt’ er seine Städt’ im Reich,\nGönnt’ alles seinem Erben,\nDen Becher nicht zugleich.", []curatedToken{
				{surface: "er", lemma: "er", upos: "PRON", dependency: "nsubj"},
				{surface: "kam", lemma: "kommen", upos: "VERB", dependency: "root", root: true},
				{surface: "Becher", lemma: "Becher", upos: "NOUN", dependency: "obj"},
			}),
			curatedSentence(t, units["excerpt-3"], "Er saß beym Königsmahle,\nDie Ritter um ihn her,\nAuf hohem Väter-Saale,\nDort auf dem Schloß am Meer.", []curatedToken{
				{surface: "Er", lemma: "er", upos: "PRON", dependency: "nsubj"},
				{surface: "saß", lemma: "sitzen", upos: "VERB", dependency: "root", root: true},
				{surface: "Meer", lemma: "Meer", upos: "NOUN", dependency: "obl"},
			}),
			curatedSentence(t, units["excerpt-3"], "Dort stand der alte Zecher,\nTrank letzte Lebensgluth,\nUnd warf den heiligen Becher\nHinunter in die Fluth.", []curatedToken{
				{surface: "Zecher", lemma: "Zecher", upos: "NOUN", dependency: "nsubj"},
				{surface: "warf", lemma: "werfen", upos: "VERB", dependency: "root", root: true},
				{surface: "Becher", lemma: "Becher", upos: "NOUN", dependency: "obj"},
			}),
			curatedSentence(t, units["excerpt-3"], "Er sah ihn stürzen, trinken\nUnd sinken tief ins Meer,\nDie Augen thäten ihm sinken,\nTrank nie einen Tropfen mehr.", []curatedToken{
				{surface: "Er", lemma: "er", upos: "PRON", dependency: "nsubj"},
				{surface: "sah", lemma: "sehen", upos: "VERB", dependency: "root", root: true},
				{surface: "Augen", lemma: "Auge", upos: "NOUN", dependency: "obj"},
			}),
		},
		"excerpt-4": {
			curatedSentence(t, units["excerpt-4"], "Mephistopheles zu Faust.", []curatedToken{
				{surface: "Mephistopheles", lemma: "Mephistopheles", upos: "PROPN", dependency: "root", root: true},
				{surface: "zu", lemma: "zu", upos: "ADP", dependency: "case"},
				{surface: "Faust", lemma: "Faust", upos: "PROPN", dependency: "conj"},
			}),
			curatedSentence(t, units["excerpt-4"], "Stoß zu!", []curatedToken{
				{surface: "Stoß", lemma: "Stoß", upos: "NOUN", dependency: "root", root: true},
				{surface: "zu", lemma: "zu", upos: "ADP", dependency: "case"},
			}),
			curatedSentence(t, units["excerpt-4"], "Valentin fällt.", []curatedToken{
				{surface: "Valentin", lemma: "Valentin", upos: "PROPN", dependency: "nsubj"},
				{surface: "fällt", lemma: "fallen", upos: "VERB", dependency: "root", root: true},
			}),
			curatedSentence(t, units["excerpt-4"], "O weh!", []curatedToken{
				{surface: "O", lemma: "o", upos: "INTJ", dependency: "root", root: true},
				{surface: "weh", lemma: "weh", upos: "INTJ", dependency: "conj"},
			}),
		},
		"excerpt-5": {
			curatedSentence(t, units["excerpt-5"], "Mephistopheles mit der Alten.", []curatedToken{
				{surface: "Mephistopheles", lemma: "Mephistopheles", upos: "PROPN", dependency: "root", root: true},
				{surface: "mit", lemma: "mit", upos: "ADP", dependency: "case"},
				{surface: "Alten", lemma: "Alte", upos: "NOUN", dependency: "obl"},
			}),
			curatedSentence(t, units["excerpt-5"], "Einst hatt’ ich einen wüsten Traum;\nDa sah’ ich einen gespaltnen Baum,\nDer hatt’ ein — — —\nSo — es war,", []curatedToken{
				{surface: "Traum", lemma: "Traum", upos: "NOUN", dependency: "obj"},
				{surface: "Baum", lemma: "Baum", upos: "NOUN", dependency: "obj"},
				{surface: "war", lemma: "sein", upos: "AUX", dependency: "root", root: true},
			}),
			curatedSentence(t, units["excerpt-5"], "gefiel mir’s doch.", []curatedToken{
				{surface: "gefiel", lemma: "gefallen", upos: "VERB", dependency: "root", root: true},
			}),
			curatedSentence(t, units["excerpt-5"], "Ich biete meinen besten Gruß\nDem Ritter mit dem Pferdefuß!\nHalt’ er einen — — bereit,\nWenn er — — — nicht scheut.", []curatedToken{
				{surface: "Ich", lemma: "ich", upos: "PRON", dependency: "nsubj"},
				{surface: "biete", lemma: "bieten", upos: "VERB", dependency: "root", root: true},
				{surface: "Gruß", lemma: "Gruß", upos: "NOUN", dependency: "obj"},
			}),
		},
	}
	return func(_ context.Context, request analyzer.AnalyzeRequest) (analyzer.Result, error) {
		sentences := results[manifests[request.Document.ID]]
		return analyzer.Result{
			SchemaVersion:        "1.0.0",
			Language:             request.Language,
			Analysis:             analyzer.AnalysisProvenance{AnalyzerName: "faust-excerpt-supplied", AnalyzerVersion: "1"},
			NormalizationProfile: analyzer.NormalizationProfile{Name: "casefold", Version: "1"},
			Sentences:            sentences,
		}, nil
	}
}

// curatorSentenceText keeps the cross-line Director sentence in one place so
// the provider fixture and the extraction fixture cannot drift.
const (
	curatorSentenceText = "Ihr beyden die ihr mir so oft,\nIn Noth und Trübsal, beygestanden,\nSagt was ihr wohl, in deutschen Landen,\nVon unsrer Unternehmung hofft?" //nolint:misspell // Goethe source text
	directionStandalone = "Sie fängt an zu singen, indem sie sich auszieht."
)

func TestFaustExcerptEndToEndRegression(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	databaseURL, pool := testutil.Postgres(t, ctx, persistence.Migrate)
	store, err := persistence.Open(ctx, databaseURL)
	require.NoError(t, err)
	testutil.Cleanup(t, "store", store.Close)
	require.NoError(t, analysis.MigrateRiver(ctx, pool))
	owner, err := store.CreateUser(ctx, "faust-excerpt-learner", false)
	require.NoError(t, err)

	data := testutil.ZipDirectory(t, filepath.Join("..", "epub", "testfixtures", "faust-excerpt"))
	extraction, err := epub.ExtractV2(data)
	require.NoError(t, err)
	imported, err := epub.NewService(store).Import(ctx, owner.ID, "de", data)
	require.NoError(t, err)

	client, err := analysis.NewClient(pool, &analyzertest.Fake{AnalyzeFunc: faustExcerptAnalysis(t, extraction)}, analyzertest.ReadyDepparseCapabilityProvider(), selection.NewService(store))
	require.NoError(t, err)
	require.NoError(t, client.Start(ctx))
	testutil.StopOnCleanup(t, "analysis client", client.Stop)
	service := analysis.NewService(pool, client)
	handle, err := service.SubmitAnalysis(ctx, owner.ID, imported.Source.ID)
	require.NoError(t, err)
	status, err := service.Wait(ctx, owner.ID, handle.ID)
	require.NoError(t, err)
	require.Equal(t, rivertype.JobStateCompleted, status.State)

	unit := func(manifest string) domain.ExtractedUnit {
		for _, candidate := range extraction.Book.ExtractedUnits.Units {
			if candidate.ManifestID == manifest {
				return candidate
			}
		}
		t.Fatalf("no extracted unit for manifest %q", manifest)
		return domain.ExtractedUnit{}
	}
	var runID, corpusID, bookID string
	require.NoError(t, pool.QueryRow(ctx, `
SELECT cai.analysis_run_id::text, cai.corpus_id::text, cai.book_id::text
FROM current_analysis_identity cai
WHERE cai.owner_id=$1 AND cai.source_material_id=$2`, owner.ID, imported.Source.ID).Scan(&runID, &corpusID, &bookID))

	ordinalAndUnitByText := func(text string) (int64, string) {
		var ordinal int64
		var unitID string
		require.NoError(t, pool.QueryRow(ctx, `
SELECT sentence_ordinal, unit_id FROM corpus_sentences
WHERE owner_id=$1 AND corpus_id=$2 AND sentence_text=$3
ORDER BY sentence_ordinal LIMIT 1`, owner.ID, corpusID, text).Scan(&ordinal, &unitID))
		return ordinal, unitID
	}
	tokenOrdinal := func(sentence int64, surface string) int64 {
		var ordinal int64
		require.NoError(t, pool.QueryRow(ctx, `
SELECT token_ordinal FROM corpus_tokens
WHERE owner_id=$1 AND corpus_id=$2 AND sentence_ordinal=$3 AND surface=$4
ORDER BY token_ordinal LIMIT 1`, owner.ID, corpusID, sentence, surface).Scan(&ordinal))
		return ordinal
	}

	directorText := curatorSentenceText
	t.Run("Director cross-line sentence keeps exact text and offsets", func(t *testing.T) {
		directorUnit := unit("excerpt-0")
		ordinal, unitID := ordinalAndUnitByText(directorText)
		assert.Equal(t, directorUnit.ID, unitID)
		var sentenceText string
		var sentenceStart, sentenceEnd int64
		require.NoError(t, pool.QueryRow(ctx, `
SELECT sentence_text, start_offset, end_offset FROM corpus_sentences
WHERE owner_id=$1 AND corpus_id=$2 AND sentence_ordinal=$3`,
			owner.ID, corpusID, ordinal).Scan(&sentenceText, &sentenceStart, &sentenceEnd))
		assert.Equal(t, directorText, sentenceText)
		assert.Equal(t, directorText, string([]rune(directorUnit.Text)[sentenceStart:sentenceEnd]))
		assert.Equal(t, 3, strings.Count(directorText, "\n"), "the complete Director sentence spans four source lines")
	})

	t.Run("Thule keeps six four-line groups and no sentence crosses a group", func(t *testing.T) {
		thule := unit("excerpt-3")
		groups := strings.Split(thule.Text, "\n\n")
		var stanzas []string
		for _, group := range groups {
			if len(strings.Split(group, "\n")) == 4 {
				stanzas = append(stanzas, group)
			}
		}
		require.Len(t, stanzas, 6, "the song's six groups survive extraction")
		for _, stanza := range stanzas {
			ordinal, unitID := ordinalAndUnitByText(stanza)
			assert.Equal(t, thule.ID, unitID)
			assert.Equal(t, stanza, corpusSentenceText(t, ctx, pool, owner.ID, corpusID, ordinal))
		}
	})

	t.Run("correction markup keeps visible text only", func(t *testing.T) {
		correction := unit("excerpt-5")
		assert.Contains(t, correction.Text, "Der hatt’ ein — — —\nSo — es war,")
		for _, hidden := range []string{"ungeheures Loch", "rechten Pfropf", "das große Loch", "Transcriber"} {
			assert.NotContains(t, correction.Text, hidden)
		}
		ordinal, _ := ordinalAndUnitByText("Einst hatt’ ich einen wüsten Traum;\nDa sah’ ich einen gespaltnen Baum,\nDer hatt’ ein — — —\nSo — es war,")
		stored := corpusSentenceText(t, ctx, pool, owner.ID, corpusID, ordinal)
		assert.Contains(t, stored, "— — —")
		for _, hidden := range []string{"ungeheures Loch", "groß", "rechten Pfropf", "das große Loch"} {
			assert.NotContains(t, stored, hidden)
		}
	})

	t.Run("analyzer-owned boundaries may share one source line", func(t *testing.T) {
		correction := unit("excerpt-5")
		first, _ := ordinalAndUnitByText("Einst hatt’ ich einen wüsten Traum;\nDa sah’ ich einen gespaltnen Baum,\nDer hatt’ ein — — —\nSo — es war,")
		second, _ := ordinalAndUnitByText("gefiel mir’s doch.")
		var firstStart, firstEnd, secondStart, secondEnd int64
		require.NoError(t, pool.QueryRow(ctx, `SELECT start_offset,end_offset FROM corpus_sentences WHERE owner_id=$1 AND corpus_id=$2 AND sentence_ordinal=$3`, owner.ID, corpusID, first).Scan(&firstStart, &firstEnd))
		require.NoError(t, pool.QueryRow(ctx, `SELECT start_offset,end_offset FROM corpus_sentences WHERE owner_id=$1 AND corpus_id=$2 AND sentence_ordinal=$3`, owner.ID, corpusID, second).Scan(&secondStart, &secondEnd))
		assert.Equal(t, lineIndex(correction.Text, int(firstEnd)), lineIndex(correction.Text, int(secondStart)), "the supplied analyzer's two sentences share one source line")
		assert.Equal(t, firstEnd+1, secondStart, "the boundary is the analyzer's, not a source-line split")
	})

	type candidateRow struct {
		count int
		refs  []byte
	}
	t.Run("selection excludes speaker labels and keeps directions and recurring targets", func(t *testing.T) {
		rows, err := pool.Query(ctx, `
SELECT canonical_lemma, occurrence_count, eligible_sentence_refs
FROM selection_candidates WHERE owner_id=$1 AND corpus_id=$2`, owner.ID, corpusID)
		require.NoError(t, err)
		byLemma := make(map[string]candidateRow)
		for rows.Next() {
			var lemma string
			var row candidateRow
			require.NoError(t, rows.Scan(&lemma, &row.count, &row.refs))
			byLemma[lemma] = row
		}
		require.NoError(t, rows.Err())
		require.Contains(t, byLemma, "becher")
		assert.Equal(t, 3, byLemma["becher"].count, "Thule supplies three Becher occurrences")
		require.Contains(t, byLemma, "unternehmung")
		assert.Equal(t, 1, byLemma["unternehmung"].count)
		var refs []selection.SentenceReference
		require.NoError(t, json.Unmarshal(byLemma["unternehmung"].refs, &refs))
		require.Len(t, refs, 1)
		assert.Equal(t, directorText, refs[0].Text)
		require.Contains(t, byLemma, "singen")
		assert.Equal(t, 1, byLemma["singen"].count)
		assert.NotEmpty(t, byLemma["singen"].refs, "a standalone direction is eligible study context")
		require.Contains(t, byLemma, "loch")
		assert.Equal(t, 1, byLemma["loch"].count)
		assert.JSONEq(t, "[]", string(byLemma["loch"].refs), "the mixed-role sentence supplies no study context")
		require.Contains(t, byLemma, "geberde")
		assert.Equal(t, 1, byLemma["geberde"].count)
		assert.JSONEq(t, "[]", string(byLemma["geberde"].refs), "the mixed-role sentence supplies no study context")
		for _, excluded := range []string{"valentin", "fallen", "mephistopheles", "faust"} {
			assert.NotContains(t, byLemma, excluded, "speaker-label tokens are structural non-vocabulary")
		}
		require.NotContains(t, byLemma, "weh", "a one-word exclamation yields no vocabulary identity")
	})

	t.Run("Concordance keeps speaker labels searchable and withholds Study", func(t *testing.T) {
		result, err := store.ListVocabularyConcordance(ctx, owner.ID, "de", domain.ConcordanceLookup{Language: "de", Term: "Valentin", Match: domain.ConcordanceMatchForm, Page: 1})
		require.NoError(t, err)
		require.NotEmpty(t, result.Occurrences)
		for _, occurrence := range result.Occurrences {
			assert.True(t, occurrence.SpeakerLabel, "the label remains raw searchable evidence")
			assert.True(t, occurrence.StudyUnavailable, "a label-overlapping sentence supplies no Study context")
		}
		becher, err := store.ListVocabularyConcordance(ctx, owner.ID, "de", domain.ConcordanceLookup{Language: "de", Term: "Becher", Match: domain.ConcordanceMatchLemma, Page: 1})
		require.NoError(t, err)
		require.NotEmpty(t, becher.Occurrences)
		for _, occurrence := range becher.Occurrences {
			assert.False(t, occurrence.SpeakerLabel)
			assert.False(t, occurrence.StudyUnavailable)
		}
	})

	t.Run("study context carries exact source lines and attribution", func(t *testing.T) {
		directorOrdinal, directorUnitID := ordinalAndUnitByText(directorText)
		directorStudy, err := store.GetVocabularySentenceStudy(ctx, owner.ID, bookID, runID, corpusID, directorUnitID, directorOrdinal, tokenOrdinal(directorOrdinal, "Unternehmung"), "Unternehmung")
		require.NoError(t, err)
		assert.False(t, directorStudy.SpeakerLabelOverlap)
		assert.Equal(t, domain.SourceAttribution{Kind: domain.AttributionSpeaker, Speaker: "Director"}, directorStudy.Attribution)
		assert.Equal(t, directorText, directorStudy.SentenceText)
		assert.Equal(t, unit("excerpt-0").Text, directorStudy.UnitText)
		assert.Equal(t, directorText, string([]rune(directorStudy.UnitText)[directorStudy.SentenceStartOffset:directorStudy.SentenceEndOffset]))

		mixedText := "Nachdem die Löcher alle gebohrt und verstopft sind, Mephistopheles mit seltsamen Geberden."
		mixedOrdinal, mixedUnitID := ordinalAndUnitByText(mixedText)
		mixedStudy, err := store.GetVocabularySentenceStudy(ctx, owner.ID, bookID, runID, corpusID, mixedUnitID, mixedOrdinal, tokenOrdinal(mixedOrdinal, "Löcher"), "Löcher")
		require.NoError(t, err)
		assert.True(t, mixedStudy.SpeakerLabelOverlap, "a mixed-role sentence supplies no study or card context")

		directionText := directionStandalone
		directionOrdinal, directionUnitID := ordinalAndUnitByText(directionText)
		directionStudy, err := store.GetVocabularySentenceStudy(ctx, owner.ID, bookID, runID, corpusID, directionUnitID, directionOrdinal, tokenOrdinal(directionOrdinal, "singen"), "singen")
		require.NoError(t, err)
		assert.False(t, directionStudy.SpeakerLabelOverlap)
		assert.Equal(t, domain.SourceAttribution{Kind: domain.AttributionStageDirection}, directionStudy.Attribution)
	})

	checkPreparationAndAnki(t, ctx, store, pool, owner.ID, imported, extraction, runID, corpusID, unit)
}

// faustDirectorProviderFixture mirrors the reviewed production-provider fixture
// so the end-to-end regression proves the same exact request and response.
type faustDirectorProviderFixture struct {
	CanonicalLemma      string                    `json:"canonical_lemma"`
	UPOS                string                    `json:"upos"`
	TargetWord          string                    `json:"target_word"`
	SourceSentence      string                    `json:"source_sentence"`
	CandidateSenses     []enrichment.LexicalSense `json:"candidate_senses"`
	Translation         string                    `json:"translation"`
	FallbackGloss       string                    `json:"fallback_gloss"`
	SentenceTranslation string                    `json:"sentence_translation"`
	ContiguousTargets   []string                  `json:"contiguous_targets"`
}

func loadFaustDirectorProviderFixture(t *testing.T) faustDirectorProviderFixture {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join("..", "enrichment", "testdata", "faust_director_provider.json"))
	require.NoError(t, err)
	var fixture faustDirectorProviderFixture
	require.NoError(t, json.Unmarshal(raw, &fixture))
	require.Equal(t, curatorSentenceText, fixture.SourceSentence)
	return fixture
}

// checkPreparationAndAnki walks the persisted evidence through the production
// preparation input assembler and presentation freeze, then inspects the APKG
// notes. It is package inspection, not a real Anki import or provider call.
func checkPreparationAndAnki(t *testing.T, ctx context.Context, store *persistence.PostgresStore, pool *pgxpool.Pool, owner string, imported epub.ImportResult, extraction epub.Extraction, runID, corpusID string, unit func(string) domain.ExtractedUnit) {
	t.Helper()
	preparation, err := store.CreateDeckPreparation(ctx, domain.DeckPreparation{
		OwnerID: owner, SourceMaterialID: imported.Source.ID, AnalysisRunID: runID,
		Filename: "faust-excerpt.apkg", DeckName: faustExcerptDeck, ContentHash: imported.Source.ContentHash,
	})
	require.NoError(t, err)

	tx, err := pool.Begin(ctx)
	require.NoError(t, err)
	testutil.Cleanup(t, "prepared deck facts transaction", func() error { return tx.Rollback(context.WithoutCancel(ctx)) })
	facts, err := store.LoadPreparedDeckInputFactsTx(ctx, tx, preparation)
	require.NoError(t, err)
	projections, deckName, err := prepareddeck.NewInputAssembler(store).AssemblePreparedDeckInputs(ctx, tx, preparation)
	require.NoError(t, err)

	assert.Equal(t, faustExcerptDeck, deckName)
	assert.True(t, facts.Structure.Recorded)
	assert.Equal(t, extraction.Structure.RoleSpans, facts.Structure.RoleSpans)
	assert.Equal(t, extraction.Structure.HeadingResets, facts.Structure.HeadingResets)
	assert.Len(t, facts.SourceUnits, len(extraction.Book.ExtractedUnits.Units))

	becher := projectionByLemma(projections, "becher")
	require.NotEmpty(t, becher.Candidate.SentenceReferences, "recurring Becher has frozen study references")
	assert.Empty(t, projectionByLemma(projections, "loch").Candidate.CanonicalLemma, "a label-only context is not prepared")

	fixture := loadFaustDirectorProviderFixture(t)
	directorCount, directorFormsJSON, directorRefs := loadSelectionCandidate(t, ctx, pool, owner, corpusID, "unternehmung")
	require.Equal(t, 1, directorCount)
	var directorForms []string
	require.NoError(t, json.Unmarshal(directorFormsJSON, &directorForms))
	require.Equal(t, []string{fixture.TargetWord}, directorForms)
	directorOrdinal := queryCorpusSentence(t, ctx, pool, owner, corpusID, curatorSentenceText)
	directorSentence := corpusSentence(t, ctx, store, owner, corpusID, directorOrdinal, unit("excerpt-0").ID)
	director := curatedProjection(t, owner, corpusID, deckName, domain.SelectionCandidate{
		OwnerID: owner, CorpusID: corpusID, Language: "de", CanonicalLemma: "unternehmung", UPOS: fixture.UPOS,
		OccurrenceCount: directorCount, ObservedForms: directorFormsJSON, SentenceReferences: directorRefs,
	}, directorSentence, directorOrdinal, fixture.TargetWord, fixture.CandidateSenses, facts)

	directionCount, directionForms, directionRefs := loadSelectionCandidate(t, ctx, pool, owner, corpusID, "singen")
	require.Equal(t, 1, directionCount)
	directionOrdinal := queryCorpusSentence(t, ctx, pool, owner, corpusID, directionStandalone)
	directionSentence := corpusSentence(t, ctx, store, owner, corpusID, directionOrdinal, unit("excerpt-3").ID)
	direction := curatedProjection(t, owner, corpusID, deckName, domain.SelectionCandidate{
		OwnerID: owner, CorpusID: corpusID, Language: "de", CanonicalLemma: "singen", UPOS: "VERB",
		OccurrenceCount: directionCount, ObservedForms: directionForms, SentenceReferences: directionRefs,
	}, directionSentence, directionOrdinal, "singen", nil, facts)

	stoßCount, stoßForms, stoßRefs := loadSelectionCandidate(t, ctx, pool, owner, corpusID, "stoß")
	require.Equal(t, 1, stoßCount)
	var stoßForm []string
	require.NoError(t, json.Unmarshal(stoßForms, &stoßForm))
	require.Len(t, stoßForm, 1)
	stoßOrdinal := queryCorpusSentence(t, ctx, pool, owner, corpusID, "Stoß zu!")
	stoßSentence := corpusSentence(t, ctx, store, owner, corpusID, stoßOrdinal, unit("excerpt-4").ID)
	stoß := curatedProjection(t, owner, corpusID, deckName, domain.SelectionCandidate{
		OwnerID: owner, CorpusID: corpusID, Language: "de", CanonicalLemma: "stoß", UPOS: "NOUN",
		OccurrenceCount: stoßCount, ObservedForms: stoßForms, SentenceReferences: stoßRefs,
	}, stoßSentence, stoßOrdinal, stoßForm[0], nil, facts)

	omissionText := "O weh!"
	omissionOrdinal := queryCorpusSentence(t, ctx, pool, owner, corpusID, omissionText)
	omissionSentence := corpusSentence(t, ctx, store, owner, corpusID, omissionOrdinal, unit("excerpt-4").ID)
	omissionCandidate := domain.SelectionCandidate{OwnerID: owner, CorpusID: corpusID, Language: "de", CanonicalLemma: "weh", UPOS: "INTJ", OccurrenceCount: 1}
	omission := curatedProjection(t, owner, corpusID, deckName, omissionCandidate, omissionSentence, omissionOrdinal, "weh", nil, facts)

	all := make([]cardexport.CandidateProjection, 0, len(projections)+4)
	all = append(all, projections...)
	all = append(all, director, direction, stoß, omission)
	for index := range all {
		all[index].Provider = "llm"
		all[index].ProviderVersion = "prompt-v2"
		all[index].TargetLanguage = "en"
	}
	deck, diagnostics, err := cardexport.NewPresentation(nil).Freeze(ctx, owner, deckName, all)
	require.NoError(t, err)
	require.Len(t, diagnostics.QualityOmissions, 2, "the deliberately short exclamations are omitted")
	assert.ElementsMatch(t, []string{"stoß", "weh"}, []string{diagnostics.QualityOmissions[0].CanonicalLemma, diagnostics.QualityOmissions[1].CanonicalLemma})

	work := deck.WorkProjection()
	require.Len(t, work, 3, "only the accepted recurring and curated targets call the provider")
	directorWork := workItemByLemma(t, work, "unternehmung")
	assert.Equal(t, fixture.SourceSentence, directorWork.Request.ExampleSentence)
	assert.Equal(t, fixture.TargetWord, directorWork.Request.TargetWord)
	assert.Equal(t, fixture.CanonicalLemma, directorWork.Request.CanonicalLemma)
	payload, err := json.Marshal(directorWork.Request) //nolint:musttag // the whole request is serialized to check for leaked context
	require.NoError(t, err)
	for _, forbidden := range []string{"Director", "Vorspiel", "Theaterdichter", "Stage direction"} {
		assert.NotContains(t, string(payload), forbidden, "attribution, title and orientation never enter the provider payload")
	}
	lemmas := workItemLemmas(work)
	assert.NotContains(t, lemmas, "stoß", "an omitted exclamation has no provider identity")
	assert.NotContains(t, lemmas, "weh", "an omitted exclamation has no provider identity")
	assert.NotContains(t, lemmas, "loch", "a mixed-role sentence supplies no card context")
	assert.NotContains(t, lemmas, "geberde", "a mixed-role sentence supplies no card context")

	results := make([]cardexport.StoredResult, 0, len(work))
	for _, item := range work {
		entry := enrichment.CacheEntry{CacheKey: item.CacheKey, FallbackGloss: "fixture gloss", SentenceTranslation: "A fixture English sentence."}
		switch item.Request.CanonicalLemma {
		case "unternehmung":
			entry.Translation = fixture.Translation
			entry.FallbackGloss = fixture.FallbackGloss
			entry.SentenceTranslation = fixture.SentenceTranslation
			entry.SentenceTranslationTargets = fixture.ContiguousTargets
		case "becher":
			entry.Translation = "goblet"
			entry.SentenceTranslation = "A goblet stood on the table."
			entry.SentenceTranslationTargets = []string{"goblet"}
		case "singen":
			entry.Translation = "sing"
			entry.SentenceTranslation = "She begins to sing as she undresses."
			entry.SentenceTranslationTargets = []string{"sing"}
		default:
			t.Fatalf("unexpected provider target %q", item.Request.CanonicalLemma)
		}
		results = append(results, cardexport.StoredResult{CacheKey: item.CacheKey, Record: entry})
	}
	artifact, _, err := cardexport.NewPresentation(nil).Finalize(ctx, deck, results, cardexport.RunFacts{
		Consent: true, Configured: true, ExecutionMode: "standard", TargetLanguage: "en", Provider: "llm", ProviderVersion: "prompt-v2",
	})
	require.NoError(t, err)
	require.Equal(t, 3, artifact.Count)

	directorNote := noteByLemma(t, artifact.Generated, "unternehmung")
	assert.Equal(t, "Director", directorNote.Attribution)
	assert.Equal(t, 4, strings.Count(directorNote.Text, `<span class="source-line">`), "the four source lines render as lines, not raw LF")
	assert.Contains(t, directorNote.Text, "<b>Unternehmung</b>")
	assert.NotContains(t, directorNote.Text, "\n")
	directionNote := noteByLemma(t, artifact.Generated, "singen")
	assert.Equal(t, "Stage direction", directionNote.Attribution)
	generatedText := noteByLemmaText(artifact.Generated)
	assert.NotContains(t, generatedText, "O weh!", "an omitted exclamation produces no note")
	assert.NotContains(t, generatedText, "Stoß zu!", "an omitted exclamation produces no note")
	assert.NotContains(t, generatedText, "Löcher", "a mixed-role sentence produces no card")

	fields := apkgNoteFields(t, artifact.APKG)
	require.Len(t, fields, 3)
	var directorFields []string
	var directionFields []string
	for _, row := range fields {
		switch {
		case strings.Contains(row[1], "Unternehmung"):
			directorFields = row
		case strings.Contains(row[1], "<b>singen</b>"):
			directionFields = row
		}
	}
	require.NotNil(t, directorFields, "the APKG has no Director note")
	require.NotNil(t, directionFields, "the APKG has no standalone-direction note")
	require.Len(t, directorFields, 18)
	assert.Equal(t, "Director", directorFields[17], "Attribution is appended after the existing fields")
	assert.Equal(t, 4, strings.Count(directorFields[1], `<span class="source-line">`))
	assert.Contains(t, directorFields[1], "<b>Unternehmung</b>")
	assert.Equal(t, "Stage direction", directionFields[17])
	assert.NotContains(t, strings.Join(directionFields, " "), "Löcher")
}

func projectionByLemma(projections []cardexport.CandidateProjection, lemma string) cardexport.CandidateProjection {
	for _, projection := range projections {
		if projection.Candidate.CanonicalLemma == lemma {
			return projection
		}
	}
	return cardexport.CandidateProjection{}
}

func workItemByLemma(t *testing.T, work []cardexport.WorkItem, lemma string) cardexport.WorkItem {
	t.Helper()
	for _, item := range work {
		if item.Request.CanonicalLemma == lemma {
			return item
		}
	}
	t.Fatalf("no provider work for %q", lemma)
	return cardexport.WorkItem{}
}

func workItemLemmas(work []cardexport.WorkItem) []string {
	lemmas := make([]string, 0, len(work))
	for _, item := range work {
		lemmas = append(lemmas, item.Request.CanonicalLemma)
	}
	return lemmas
}

func noteByLemma(t *testing.T, records []cardexport.GeneratedRecord, lemma string) cardexport.Note {
	t.Helper()
	for _, record := range records {
		if strings.EqualFold(record.Note.Lemma, lemma) || strings.EqualFold(record.Input.CanonicalLemma, lemma) {
			return record.Note
		}
	}
	t.Fatalf("no generated note for %q", lemma)
	return cardexport.Note{}
}

func noteByLemmaText(records []cardexport.GeneratedRecord) string {
	var text strings.Builder
	for _, record := range records {
		text.WriteString(record.Note.Text)
		text.WriteString("\n")
	}
	return text.String()
}

// curatedProjection builds the reviewed Director card (and the deliberate
// omission case) from the exact persisted corpus and snapshot structure.
func curatedProjection(t *testing.T, owner, corpusID, deckName string, candidate domain.SelectionCandidate, sentence analyzer.Sentence, ordinal int64, target string, senses []enrichment.LexicalSense, facts persistence.PreparedDeckInputFacts) cardexport.CandidateProjection {
	t.Helper()
	candidate.OwnerID, candidate.CorpusID = owner, corpusID
	if candidate.SentenceReferences == nil {
		refs, err := json.Marshal([]selection.SentenceReference{{
			SentenceIndex: int(ordinal), Text: sentence.Text, Location: sentence.Location,
		}})
		require.NoError(t, err)
		candidate.SentenceReferences = refs
	}
	return cardexport.CandidateProjection{
		OwnerID: owner, DeckName: deckName,
		Candidate: candidate,
		Entry: cardexport.Entry{
			OwnerID: owner, Language: "de", CanonicalLemma: candidate.CanonicalLemma, UPOS: candidate.UPOS,
			SourceDocument: faustExcerptDeck, Sentence: sentence.Text, SentenceOrdinal: ordinal,
			TargetWord: target, CandidateSenses: senses,
		},
		Sentences: map[int64]analyzer.Sentence{ordinal: sentence},
		Structure: facts.Structure, SourceUnits: facts.SourceUnits,
	}
}

// loadSelectionCandidate reads one persisted identity's reviewed selection
// facts so the curated cards use the pipeline's identity, not a fixture copy.
func loadSelectionCandidate(t *testing.T, ctx context.Context, pool *pgxpool.Pool, owner, corpusID, lemma string) (int, []byte, []byte) {
	t.Helper()
	var count int
	var forms, refs []byte
	require.NoError(t, pool.QueryRow(ctx, `
SELECT occurrence_count, observed_forms, eligible_sentence_refs
FROM selection_candidates WHERE owner_id=$1 AND corpus_id=$2 AND canonical_lemma=$3`,
		owner, corpusID, lemma).Scan(&count, &forms, &refs))
	return count, forms, refs
}

// corpusSentence loads one persisted corpus sentence with its exact unit
// identity attached, as the prepared-deck input facts do.
func corpusSentence(t *testing.T, ctx context.Context, store *persistence.PostgresStore, owner, corpusID string, ordinal int64, unitID string) analyzer.Sentence {
	t.Helper()
	sentences, err := store.ListCorpusSentences(ctx, owner, corpusID, []int64{ordinal})
	require.NoError(t, err)
	sentence, ok := sentences[ordinal]
	require.True(t, ok)
	sentence.Location.SourceDocumentID = unitID
	for index := range sentence.Tokens {
		sentence.Tokens[index].Location.SourceDocumentID = unitID
	}
	return sentence
}

func queryCorpusSentence(t *testing.T, ctx context.Context, pool *pgxpool.Pool, owner, corpusID, text string) int64 {
	t.Helper()
	var ordinal int64
	require.NoError(t, pool.QueryRow(ctx, `
SELECT sentence_ordinal FROM corpus_sentences
WHERE owner_id=$1 AND corpus_id=$2 AND sentence_text=$3
ORDER BY sentence_ordinal LIMIT 1`, owner, corpusID, text).Scan(&ordinal))
	return ordinal
}

func corpusSentenceText(t *testing.T, ctx context.Context, pool *pgxpool.Pool, owner, corpusID string, ordinal int64) string {
	t.Helper()
	var text string
	require.NoError(t, pool.QueryRow(ctx, `
SELECT sentence_text FROM corpus_sentences WHERE owner_id=$1 AND corpus_id=$2 AND sentence_ordinal=$3`,
		owner, corpusID, ordinal).Scan(&text))
	return text
}

func lineIndex(text string, offset int) int {
	// Sentence offsets are Unicode code points, not bytes.
	return strings.Count(string([]rune(text)[:offset]), "\n")
}

// apkgNoteFields reads the release APKG's note fields for package inspection.
// This is not a real Anki import.
func apkgNoteFields(t *testing.T, apkg []byte) [][]string {
	t.Helper()
	reader, err := zip.NewReader(bytes.NewReader(apkg), int64(len(apkg)))
	require.NoError(t, err)
	var collection []byte
	for _, file := range reader.File {
		if file.Name != "collection.anki2" {
			continue
		}
		opened, err := file.Open()
		require.NoError(t, err)
		collection, err = io.ReadAll(opened)
		require.NoError(t, err)
		require.NoError(t, opened.Close())
	}
	require.NotEmpty(t, collection, "APKG has no collection.anki2")
	path := filepath.Join(t.TempDir(), "collection.anki2")
	require.NoError(t, os.WriteFile(path, collection, 0o600))
	db, err := sql.Open("sqlite", path)
	require.NoError(t, err)
	testutil.Cleanup(t, "APKG SQLite database", db.Close)
	rows, err := db.QueryContext(t.Context(), `SELECT flds FROM notes ORDER BY rowid`)
	require.NoError(t, err)
	defer func() {
		if err := rows.Close(); err != nil {
			t.Errorf("close APKG note rows: %v", err)
		}
	}()
	var fields [][]string
	for rows.Next() {
		var raw string
		require.NoError(t, rows.Scan(&raw))
		fields = append(fields, strings.Split(raw, "\x1f"))
	}
	require.NoError(t, rows.Err())
	return fields
}
