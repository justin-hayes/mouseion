package cardexport

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/checked"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestAttributionFieldIsAppendedAfterExistingFields pins the released APKG
// field order: the frozen attribution follows every existing field, and the
// front renders it separately above the source text.
func TestAttributionFieldIsAppendedAfterExistingFields(t *testing.T) {
	assert.Equal(t, "Attribution", fieldNames[len(fieldNames)-1])
	assert.Len(t, fieldNames, 18)
	assert.Len(t, noteFields(Note{}), len(fieldNames))
	assert.Equal(t, "Attribution", fieldNames[17])

	model := modelMetadata(1, 2)
	fields, ok := model["flds"].([]map[string]any)
	require.True(t, ok)
	names := make([]string, len(fields))
	for i, field := range fields {
		name, ok := field["name"].(string)
		require.True(t, ok)
		names[i] = name
	}
	assert.Equal(t, fieldNames, names)

	templates, ok := model["tmpls"].([]any)
	require.True(t, ok)
	template, ok := templates[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, `{{#Attribution}}<div class="attribution">{{Attribution}}</div>{{/Attribution}}{{Text}}`, template["qfmt"])
}

// TestSourceLineTextEscapesSegmentsAndKeepsEmphasisWithinLines proves that a
// multi-line front emits explicit per-line markup, escapes each source segment
// before Mouseion's markup, preserves a literal slash, and keeps a
// separable-particle emphasis on its own source line.
func TestSourceLineTextEscapesSegmentsAndKeepsEmphasisWithinLines(t *testing.T) {
	sentence := "Ich <sehe> & / den Weg\nund <b>stehe</b> heute früh auf."
	tokens := []analyzer.Token{
		{Surface: "Ich", Location: analyzer.SourceLocation{StartOffset: 0, EndOffset: 3}},
		{Surface: "sehe", Location: analyzer.SourceLocation{StartOffset: 4, EndOffset: 8}},
		{Surface: "auf", Dependency: "compound:prt", Head: 1, Location: analyzer.SourceLocation{StartOffset: 55, EndOffset: 58}},
	}
	entry := Entry{
		Language: "de", CanonicalLemma: "aufstehen", UPOS: "VERB",
		Sentence: sentence, TargetWord: "sehe", SentenceTokens: tokens,
		Attribution: domain.SourceAttribution{Kind: domain.AttributionSpeaker, Speaker: "Erster"},
	}
	input := renderInputFromEntry(entry)
	input.sourceLinePresentation = true
	note, err := makeNote("owner", input)
	require.NoError(t, err)

	assert.Equal(t, `<span class="source-line">Ich &lt;<b>sehe</b>&gt; &amp; / den Weg</span><span class="source-line">und &lt;b&gt;stehe&lt;/b&gt; heute früh <b>auf</b>.</span>`, note.Text)
	assert.Equal(t, "Erster", note.Attribution)
	assert.NotContains(t, note.Text, "\n", "raw LF is not the line contract")
	assert.Equal(t, "Attribution", fieldNames[17])
	assert.Equal(t, note.Attribution, noteFields(note)[17])
}

// TestAttributionRendersTheSpeakerNameOrStageDirection separates the two typed
// attribution kinds and escapes the speaker as text.
func TestAttributionRendersTheSpeakerNameOrStageDirection(t *testing.T) {
	for _, test := range []struct {
		name        string
		attribution domain.SourceAttribution
		want        string
	}{
		{name: "speaker name", attribution: domain.SourceAttribution{Kind: domain.AttributionSpeaker, Speaker: "Erster & <Zweiter>"}, want: "Erster &amp; &lt;Zweiter&gt;"},
		{name: "stage direction", attribution: domain.SourceAttribution{Kind: domain.AttributionStageDirection}, want: "Stage direction"},
		{name: "no attribution", attribution: domain.SourceAttribution{}, want: ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			note, err := makeNote("owner", renderInputFromEntry(Entry{
				Language: "de", CanonicalLemma: "haus", UPOS: "NOUN",
				Sentence: "Das Haus steht heute dort.", TargetWord: "Haus",
				Attribution: test.attribution,
			}))
			require.NoError(t, err)
			assert.Equal(t, test.want, note.Attribution)
		})
	}
}

// TestLegacyManifestRendersProseWithoutAttribution keeps the historical text
// path: an old manifest with an authorial break is not retrofitted with
// source-line markup or attribution.
func TestLegacyManifestRendersProseWithoutAttribution(t *testing.T) {
	sentence := "Das Haus steht heute dort,\nund der Garten ist ruhig."
	snapshot := ManifestSnapshot{
		SchemaVersion: ManifestSchemaVersionV12, Owner: "owner-1", DeckName: "Book", Filename: DownloadFilename("Book"),
		Items: []ManifestItem{{
			Ordinal: 0, Disposition: ManifestAccepted,
			Entry:   Entry{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", Sentence: sentence, TargetWord: "Haus"},
			Quality: SentenceQuality{Accepted: true, Score: 90, Reasons: []string{"target present"}},
		}},
	}
	deck, err := NewPresentation(nil).Restore(snapshot)
	require.NoError(t, err)
	artifact, _, err := NewPresentation(nil).Finalize(t.Context(), deck, nil, RunFacts{})
	require.NoError(t, err)
	require.Len(t, artifact.Generated, 1)
	note := artifact.Generated[0].Note
	assert.Equal(t, "Das <b>Haus</b> steht heute dort,\nund der Garten ist ruhig.", note.Text)
	assert.NotContains(t, note.Text, "source-line")
	assert.Empty(t, note.Attribution)
}

// TestFrozenAttributionIsDerivedFromTheExactSnapshot proves preparation derives
// the optional typed attribution from the exact snapshot's roles and unit text,
// and never adds it to the provider request.
func TestFrozenAttributionIsDerivedFromTheExactSnapshot(t *testing.T) {
	unitID := "epub-unit-v1:0:scene"
	sentence := "Der Dichter\nspricht heute sehr laut."
	label := "Erster."
	unitText := label + "\n" + sentence
	sentenceStart, err := checked.Uint64FromInt(utf8.RuneCountInString(label) + 1)
	require.NoError(t, err)
	tokens := attributionTokens(t, sentence, sentenceStart)
	structure := domain.SnapshotStructure{
		Recorded: true, ConventionSet: domain.StructureConventionSet,
		RoleSpans: []domain.UnitRoleSpan{
			{UnitID: unitID, Role: domain.SourceStructureRoleSpeaker, StartOffset: 0, EndOffset: mustRuneCount(t, label)},
			{UnitID: unitID, Role: domain.SourceStructureRoleDirection, StartOffset: sentenceStart, EndOffset: sentenceStart + mustRuneCount(t, sentence)},
		},
	}
	sourceUnits := map[string]domain.ExtractedUnit{unitID: {ID: unitID, Text: unitText}}

	tests := []struct {
		name        string
		structure   domain.SnapshotStructure
		attribution domain.SourceAttribution
	}{
		{name: "nearest preceding speaker", structure: domain.SnapshotStructure{Recorded: true, ConventionSet: domain.StructureConventionSet, RoleSpans: structure.RoleSpans[:1]}, attribution: domain.SourceAttribution{Kind: domain.AttributionSpeaker, Speaker: "Erster"}},
		{name: "stage direction", structure: structure, attribution: domain.SourceAttribution{Kind: domain.AttributionStageDirection}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			projection := attributionProjection(t, unitID, sentence, sentenceStart, tokens, test.structure, sourceUnits)
			deck, diagnostics, err := NewPresentation(nil).Freeze(t.Context(), "owner-1", "Faust. Eine Tragödie", []CandidateProjection{projection})
			require.NoError(t, err)
			require.Empty(t, diagnostics.QualityOmissions)
			require.Equal(t, 1, deck.Summary().Accepted)
			assert.Equal(t, test.attribution, deck.StorageProjection().Items[0].Entry.Attribution)

			work := deck.WorkProjection()
			require.Len(t, work, 1)
			payload, err := json.Marshal(work[0].Request) //nolint:musttag // the whole request is serialized to check that attribution stays out
			require.NoError(t, err)
			assert.NotContains(t, string(payload), "Erster")
			assert.NotContains(t, string(payload), "Stage direction")
		})
	}
}

// TestAttributionDoesNotChangeNoteOrModelIdentity keeps same-specification
// re-import stable: presentation metadata is not part of the note identity, and
// the note type identity stays the shared released model.
func TestAttributionDoesNotChangeNoteOrModelIdentity(t *testing.T) {
	entry := Entry{
		Language: "de", CanonicalLemma: "haus", UPOS: "NOUN",
		Sentence: "Das Haus steht heute dort.", TargetWord: "Haus",
		Attribution: domain.SourceAttribution{Kind: domain.AttributionSpeaker, Speaker: "Erster"},
	}
	speaker, err := makeNote("owner", renderInputFromEntry(entry))
	require.NoError(t, err)
	entry.Attribution = domain.SourceAttribution{Kind: domain.AttributionStageDirection}
	direction, err := makeNote("owner", renderInputFromEntry(entry))
	require.NoError(t, err)

	assert.Equal(t, speaker.Key, direction.Key, "the same specification keeps the note GUID key")
	assert.Equal(t, speaker.Identity, direction.Identity, "the same specification keeps the sort identity")
	modelID := stableID("model|" + noteTypeName)
	assert.Equal(t, modelID, modelMetadata(modelID, 1)["id"], "the released note type keeps its model identity")
}

func TestApplySourceAttributionIgnoresMissingOrMismatchedEvidence(t *testing.T) {
	sentence := "Der Dichter spricht heute sehr laut."
	unit := domain.ExtractedUnit{ID: "unit", Text: "Erster.\n" + sentence}
	start := mustRuneCount(t, "Erster.\n")
	structure := domain.SnapshotStructure{Recorded: true, ConventionSet: domain.StructureConventionSet, RoleSpans: []domain.UnitRoleSpan{
		{UnitID: unit.ID, Role: domain.SourceStructureRoleSpeaker, StartOffset: 0, EndOffset: mustRuneCount(t, "Erster.")},
	}}
	base := CandidateProjection{Structure: structure, SourceUnits: map[string]domain.ExtractedUnit{unit.ID: unit}, Sentences: map[int64]analyzer.Sentence{
		0: {Text: sentence, Location: analyzer.SourceLocation{SourceDocumentID: unit.ID, StartOffset: start, EndOffset: start + mustRuneCount(t, sentence)}},
	}}

	t.Run("nil entry", func(t *testing.T) {
		require.NoError(t, applySourceAttribution(nil, CandidateProjection{}))
	})
	t.Run("unrecorded structure clears a caller value", func(t *testing.T) {
		entry := Entry{Sentence: sentence, SentenceOrdinal: 0, Attribution: domain.SourceAttribution{Kind: domain.AttributionSpeaker, Speaker: "Stale"}}
		require.NoError(t, applySourceAttribution(&entry, CandidateProjection{}))
		assert.Equal(t, domain.SourceAttribution{}, entry.Attribution)
	})
	t.Run("missing persisted sentence", func(t *testing.T) {
		entry := Entry{Sentence: sentence, SentenceOrdinal: 7}
		require.NoError(t, applySourceAttribution(&entry, base))
		assert.Equal(t, domain.SourceAttribution{}, entry.Attribution)
	})
	t.Run("sentence text mismatch", func(t *testing.T) {
		entry := Entry{Sentence: "Anderer Satz.", SentenceOrdinal: 0}
		require.NoError(t, applySourceAttribution(&entry, base))
		assert.Equal(t, domain.SourceAttribution{}, entry.Attribution)
	})
	t.Run("unit outside the exact snapshot", func(t *testing.T) {
		projection := base
		projection.SourceUnits = nil
		entry := Entry{Sentence: sentence, SentenceOrdinal: 0}
		require.NoError(t, applySourceAttribution(&entry, projection))
		assert.Equal(t, domain.SourceAttribution{}, entry.Attribution)
	})
	t.Run("unusable persisted range falls back to the sentence length", func(t *testing.T) {
		projection := base
		projection.Sentences = map[int64]analyzer.Sentence{0: {Text: sentence, Location: analyzer.SourceLocation{SourceDocumentID: unit.ID, StartOffset: start}}}
		entry := Entry{Sentence: sentence, SentenceOrdinal: 0}
		require.NoError(t, applySourceAttribution(&entry, projection))
		assert.Equal(t, domain.SourceAttribution{Kind: domain.AttributionSpeaker, Speaker: "Erster"}, entry.Attribution)
	})
	t.Run("empty displayed speaker name is not attribution", func(t *testing.T) {
		emptyUnit := domain.ExtractedUnit{ID: "unit", Text: ".\n" + sentence}
		projection := base
		projection.Structure = domain.SnapshotStructure{Recorded: true, ConventionSet: domain.StructureConventionSet, RoleSpans: []domain.UnitRoleSpan{
			{UnitID: emptyUnit.ID, Role: domain.SourceStructureRoleSpeaker, StartOffset: 0, EndOffset: 1},
		}}
		projection.SourceUnits = map[string]domain.ExtractedUnit{emptyUnit.ID: emptyUnit}
		projection.Sentences = map[int64]analyzer.Sentence{0: {Text: sentence, Location: analyzer.SourceLocation{SourceDocumentID: emptyUnit.ID, StartOffset: 2, EndOffset: 2 + mustRuneCount(t, sentence)}}}
		entry := Entry{Sentence: sentence, SentenceOrdinal: 0}
		require.NoError(t, applySourceAttribution(&entry, projection))
		assert.Equal(t, domain.SourceAttribution{}, entry.Attribution)
	})
}

func attributionProjection(t *testing.T, unitID, sentence string, start uint64, tokens []analyzer.Token, structure domain.SnapshotStructure, units map[string]domain.ExtractedUnit) CandidateProjection {
	t.Helper()
	refs, err := json.Marshal([]struct {
		SentenceIndex int              `json:"sentence_index"`
		Text          string           `json:"text"`
		Location      map[string]int64 `json:"location"`
	}{{SentenceIndex: 0, Text: sentence, Location: map[string]int64{"start_offset": mustInt64(t, start)}}})
	if err != nil {
		panic("attribution references are not JSON encodable: " + err.Error())
	}
	return CandidateProjection{
		OwnerID: "owner-1", DeckName: "Faust. Eine Tragödie",
		Candidate: domain.SelectionCandidate{
			OwnerID: "owner-1", CorpusID: "corpus-faust", Language: "de",
			CanonicalLemma: "dichter", UPOS: "NOUN", ObservedForms: []byte(`["Dichter"]`), SentenceReferences: refs,
		},
		Entry: Entry{OwnerID: "owner-1", Language: "de", CanonicalLemma: "dichter", UPOS: "NOUN", SourceDocument: "Faust. Eine Tragödie"},
		Sentences: map[int64]analyzer.Sentence{0: {
			Text: sentence, Tokens: tokens,
			Location: analyzer.SourceLocation{SourceDocumentID: unitID, StartOffset: start, EndOffset: start + mustRuneCount(t, sentence)},
		}},
		Provider: "llm", ProviderVersion: "prompt-v2", TargetLanguage: "en",
		Structure: structure, SourceUnits: units,
	}
}

// attributionTokens builds the dependency evidence the German card gate needs
// for the two-line attribution fixture.
func attributionTokens(t *testing.T, sentence string, sentenceStart uint64) []analyzer.Token {
	t.Helper()
	runes := []rune(sentence)
	offsets := map[string]struct{ start, end uint64 }{}
	run := 0
	for part := range strings.FieldsSeq(sentence) {
		index := strings.Index(string(runes[run:]), part)
		if index < 0 {
			continue
		}
		start := run + index
		offsets[part] = struct{ start, end uint64 }{sentenceStart + mustUint64(t, start), sentenceStart + mustUint64(t, start+len([]rune(part)))}
		run = start + len([]rune(part))
	}
	token := func(surface, dependency, upos string, head uint32) analyzer.Token {
		location := offsets[surface]
		return analyzer.Token{Surface: surface, Dependency: dependency, UPOS: upos, Head: head, Location: analyzer.SourceLocation{StartOffset: location.start, EndOffset: location.end}}
	}
	return []analyzer.Token{
		token("Der", "det", "DET", 1),
		token("Dichter", "nsubj", "NOUN", 2),
		func() analyzer.Token {
			t := token("spricht", "root", "VERB", 2)
			t.Morphology = map[string]string{"VerbForm": "Fin"}
			return t
		}(),
		token("heute", "advmod", "ADV", 2),
		token("sehr", "advmod", "ADV", 5),
		token("laut", "advmod", "ADJ", 2),
	}
}

// evidenceSpyLexicalProvider fails a presentation-only rebuild if it consults
// local dictionary evidence.
type evidenceSpyLexicalProvider struct{ calls int }

func (p *evidenceSpyLexicalProvider) Name() string    { return "spy" }
func (p *evidenceSpyLexicalProvider) Version() string { return "spy-v1" }
func (p *evidenceSpyLexicalProvider) Lookup(context.Context, enrichment.LexicalLookupRequest) (enrichment.LexicalEntry, bool, error) {
	p.calls++
	return enrichment.LexicalEntry{}, false, nil
}

// TestPresentationOnlyRebuildKeepsFrozenAttributionWithoutEvidenceCalls proves a
// rebuild consumes the frozen specification: selection, translation and
// omission stay unchanged, attribution is not re-derived, and neither local
// dictionary nor provider work runs.
func TestPresentationOnlyRebuildKeepsFrozenAttributionWithoutEvidenceCalls(t *testing.T) {
	sentence := "Der Dichter\nspricht heute sehr laut."
	key := &enrichment.CacheKey{
		Language: "de", TargetLanguage: "en", CanonicalLemma: "dichter", UPOS: "NOUN",
		Provider: "llm", ProviderVersion: "prompt-v1", SentenceHash: enrichment.SentenceHash(sentence),
	}
	snapshot := ManifestSnapshot{
		SchemaVersion: ManifestSchemaVersion, Owner: "owner-1", DeckName: "Faust. Eine Tragödie", Filename: DownloadFilename("Faust. Eine Tragödie"),
		Items: []ManifestItem{
			{
				Ordinal: 0, Disposition: ManifestAccepted,
				Entry: Entry{
					Language: "de", CanonicalLemma: "dichter", UPOS: "NOUN", Sentence: sentence, TargetWord: "Dichter",
					Gloss: "poet", SourceDocument: "Faust. Eine Tragödie",
					Attribution: domain.SourceAttribution{Kind: domain.AttributionSpeaker, Speaker: "Erster"},
				},
				Quality: SentenceQuality{Accepted: true, Score: 90, Reasons: []string{"target present"}}, CacheKey: key,
			},
			{
				Ordinal: 1, Disposition: ManifestQualityOmitted,
				Entry:   Entry{Language: "de", CanonicalLemma: "ach", UPOS: "INTJ", Sentence: "Ach!", TargetWord: "Ach"},
				Quality: SentenceQuality{Accepted: false, Score: 0, Reasons: []string{"too short or fragmented"}},
			},
		},
	}
	provider := &evidenceSpyLexicalProvider{}
	presentation := NewPresentation(provider)
	deck, err := presentation.Restore(snapshot)
	require.NoError(t, err)
	before := deck.Summary()
	artifact, diagnostics, err := presentation.Finalize(t.Context(), deck, []StoredResult{{
		CacheKey: *key,
		Record: enrichment.CacheEntry{CacheKey: *key,
			Translation: "poet", SentenceTranslation: "The poet speaks very loudly today."},
	}}, RunFacts{Consent: true, Configured: true, ExecutionMode: "batch", TargetLanguage: "en", Provider: "llm", ProviderVersion: "prompt-v1"})
	require.NoError(t, err)

	assert.Zero(t, provider.calls, "a presentation-only rebuild queried dictionary evidence")
	assert.Equal(t, 2, before.Selected)
	assert.Equal(t, 1, before.Accepted)
	assert.Equal(t, 1, before.Omitted)
	assert.Len(t, diagnostics.QualityOmissions, 1, "the frozen quality omission is unchanged")
	assert.Equal(t, Completeness{TotalCards: 1, CardsWithEnglish: 1, CardsWithEnglishSentence: 1, QualityOmitted: 1}, artifact.Completeness)
	require.Len(t, artifact.Generated, 1)
	assert.Equal(t, []string{"Der <b>Dichter</b>", "spricht heute sehr laut."}, sourceLineContents(artifact.Generated[0].Note.Text))
	assert.Equal(t, "Erster", artifact.Generated[0].Note.Attribution)
	assert.Equal(t, "poet", artifact.Generated[0].Note.Gloss)
	assert.Equal(t, "The poet speaks very loudly today.", artifact.Generated[0].Note.EnglishSentence)
}

func sourceLineContents(text string) []string {
	const prefix = `<span class="source-line">`
	var lines []string
	for _, part := range strings.Split(text, prefix)[1:] {
		line, _, _ := strings.Cut(part, `</span>`)
		lines = append(lines, line)
	}
	return lines
}

// mustRuneCount converts a non-negative rune count for a test fixture.
func mustRuneCount(t *testing.T, value string) uint64 {
	t.Helper()
	count, err := checked.Uint64FromInt(utf8.RuneCountInString(value))
	require.NoError(t, err)
	return count
}

// mustInt64 converts a bounded rune offset for a test fixture.
func mustInt64(t *testing.T, value uint64) int64 {
	t.Helper()
	converted, err := checked.Int64FromUint64(value)
	require.NoError(t, err)
	return converted
}

// mustUint64 converts a non-negative rune index for a test fixture.
func mustUint64(t *testing.T, value int) uint64 {
	t.Helper()
	converted, err := checked.Uint64FromInt(value)
	require.NoError(t, err)
	return converted
}
