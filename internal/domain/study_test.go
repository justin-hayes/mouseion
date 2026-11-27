package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func studyID(lemma string) StudyItem {
	return StudyItem{Language: "de", CanonicalLemma: lemma, UPOS: "NOUN"}
}

func TestOrderStudyStream(t *testing.T) {
	tests := []struct {
		name  string
		input []StudyCandidate
		want  []string
	}{
		{name: "empty", input: nil, want: []string{}},
		{
			name: "current reading first, then personal occurrences descending",
			input: []StudyCandidate{
				{StudyItem: studyID("alpha"), PersonalOccurrences: 90},
				{StudyItem: studyID("beta"), InCurrentReading: true, PersonalOccurrences: 3},
				{StudyItem: studyID("gamma"), InCurrentReading: true, PersonalOccurrences: 7},
				{StudyItem: studyID("delta"), PersonalOccurrences: 120},
			},
			want: []string{"gamma/NOUN/", "beta/NOUN/", "delta/NOUN/", "alpha/NOUN/"},
		},
		{
			name: "ties break by lemma, part of speech then discriminator",
			input: []StudyCandidate{
				{Language: "de", CanonicalLemma: "b", UPOS: "NOUN", PersonalOccurrences: 5},
				{Language: "de", CanonicalLemma: "a", UPOS: "VERB", PersonalOccurrences: 5},
				{Language: "de", CanonicalLemma: "a", UPOS: "ADJ", PersonalOccurrences: 5},
				{Language: "de", CanonicalLemma: "a", UPOS: "ADJ", LexicalDiscriminator: "x:2", PersonalOccurrences: 5},
			},
			want: []string{"a/ADJ/", "a/ADJ/x:2", "a/VERB/", "b/NOUN/"},
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			input := append([]StudyCandidate(nil), test.input...)
			ordered := OrderStudyStream(input)
			got := make([]string, 0, len(ordered))
			for _, candidate := range ordered {
				got = append(got, candidate.CanonicalLemma+"/"+candidate.UPOS+"/"+candidate.LexicalDiscriminator)
			}
			assert.Equal(t, test.want, got)
			assert.Equal(t, test.input, input, "the input slice is not reordered")
		})
	}
}

func TestStudyPreparationTargets(t *testing.T) {
	ordered := make([]StudyCandidate, 0, 40)
	for i := range 40 {
		ordered = append(ordered, StudyCandidate{StudyItem: studyID(string(rune('a'+i%26)) + string(rune('a'+i/26)))})
	}
	t.Run("takes the first window of the stream", func(t *testing.T) {
		targets := StudyPreparationTargets(ordered, nil)
		assert.Len(t, targets, StudyPreparationWindow)
		assert.Equal(t, ordered[0].StudyItem, targets[0].StudyItem)
		assert.Equal(t, ordered[29].StudyItem, targets[29].StudyItem)
	})
	t.Run("skips unpreparable words and reaches further", func(t *testing.T) {
		states := map[StudyItem]StudyMaterialState{
			ordered[0].StudyItem: StudyMaterialUnpreparable,
			ordered[1].StudyItem: StudyMaterialUnpreparable,
		}
		targets := StudyPreparationTargets(ordered, states)
		assert.Len(t, targets, StudyPreparationWindow)
		assert.Equal(t, ordered[2].StudyItem, targets[0].StudyItem)
		assert.Equal(t, ordered[31].StudyItem, targets[29].StudyItem)
	})
	t.Run("a short stream yields what it has", func(t *testing.T) {
		assert.Len(t, StudyPreparationTargets(ordered[:4], nil), 4)
	})
}

func TestStudyMaterialReadyContentCitationsAreCompleteOnlyWhenWhole(t *testing.T) {
	tests := []struct {
		name     string
		cited    StudyCitations
		complete bool
	}{
		{"modern material cites nothing", StudyCitations{}, true},
		{"senses across entries", StudyCitations{SenseIDs: []string{"n1.1", "n2.1"}, EntryIDs: []string{"n1", "n2"}, DictionaryPin: "pin"}, true},
		{"a sense two entries share is cited once per entry", StudyCitations{SenseIDs: []string{"s1", "s1"}, EntryIDs: []string{"n1", "n2"}, DictionaryPin: "pin"}, true},
		{"a cited sense needs the pin", StudyCitations{SenseIDs: []string{"n1.1"}, EntryIDs: []string{"n1"}}, false},
		{"a pin needs a cited sense", StudyCitations{DictionaryPin: "pin"}, false},
		{"entries need senses", StudyCitations{EntryIDs: []string{"n1"}, DictionaryPin: "pin"}, false},
		{"cited senses name their entries", StudyCitations{SenseIDs: []string{"n1.1"}, DictionaryPin: "pin"}, false},
		{"each sense names exactly one entry", StudyCitations{SenseIDs: []string{"n1.1", "n1.2"}, EntryIDs: []string{"n1"}, DictionaryPin: "pin"}, false},
		{"an entry is never blank", StudyCitations{SenseIDs: []string{"n1.1"}, EntryIDs: []string{""}, DictionaryPin: "pin"}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			content := StudyMaterialReadyContent{Sentence: "s", TargetWord: "t", Gloss: "g", SentenceTranslation: "x", Cited: tc.cited}
			require.Equal(t, tc.complete, content.Complete())
		})
	}
}

func TestStudyCitationsResolveSensesPerEntry(t *testing.T) {
	cited := StudyCitations{SenseIDs: []string{"s1", "s2", "s1"}, EntryIDs: []string{"n1", "n1", "n3"}, DictionaryPin: "pin"}
	require.True(t, cited.Has("n1", "s1"))
	require.True(t, cited.Has("n3", "s1"))
	require.False(t, cited.Has("n2", "s1"), "a homograph that shares the sense ID is not cited")
	require.Equal(t, []string{"n1", "n3"}, cited.EntrySet())
}

func TestStudyMaterialRetriesANoDictionaryEntryOmissionOnlyAgainstAnotherDictionary(t *testing.T) {
	omitted := StudyMaterial{State: StudyMaterialUnpreparable, OmissionReason: StudyOmissionNoDictionaryEntry, OmissionDictionaryPin: "pin-1"}
	cases := map[string]struct {
		material StudyMaterial
		pin      string
		want     bool
	}{
		"a changed pin retries":                 {omitted, "pin-2", true},
		"the same pin does not":                 {omitted, "pin-1", false},
		"a padded pin is the same pin":          {omitted, " pin-1 ", false},
		"no loaded dictionary never retries":    {omitted, "", false},
		"an omission before pins were recorded": {StudyMaterial{State: StudyMaterialUnpreparable, OmissionReason: StudyOmissionNoDictionaryEntry}, "pin-1", true},
		"a meaning omission is not retried":     {StudyMaterial{State: StudyMaterialUnpreparable, OmissionReason: StudyOmissionMeaning}, "pin-2", false},
		"a sentence-quality omission is not":    {StudyMaterial{State: StudyMaterialUnpreparable, OmissionReason: StudyOmissionSentenceQuality}, "pin-2", false},
		"ready material is not an omission":     {StudyMaterial{State: StudyMaterialReady}, "pin-2", false},
		"pending material is not an omission":   {StudyMaterial{State: StudyMaterialPending}, "pin-2", false},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) { assert.Equal(t, c.want, c.material.RetriesAgainstDictionary(c.pin)) })
	}
}

func TestOrderStudyStreamRanksWordsAndIdiomsTogether(t *testing.T) {
	idiom := func(headword string, current bool, count int64) StudyCandidate {
		return StudyCandidate{StudyItem: IdiomItem("de", headword), InCurrentReading: current, PersonalOccurrences: count}
	}
	word := func(lemma string, current bool, count int64) StudyCandidate {
		return StudyCandidate{StudyItem: studyID(lemma), InCurrentReading: current, PersonalOccurrences: count}
	}
	ordered := OrderStudyStream([]StudyCandidate{
		idiom("zz tie", false, 5),
		word("zz tie", false, 5),
		idiom("a tie", false, 5),
		word("high", false, 50),
		idiom("many", false, 80),
		idiom("reading idiom", true, 1),
		word("reading word", true, 2),
	})
	got := make([]string, 0, len(ordered))
	for _, candidate := range ordered {
		got = append(got, candidate.ItemKind.String()+":"+candidate.CanonicalLemma)
	}
	assert.Equal(t, []string{
		"word:reading word", "idiom:reading idiom", // the Current reading's items first, by count
		"idiom:many", "word:high", // then by count, whatever the kind
		"word:zz tie", "idiom:a tie", "idiom:zz tie", // a tie: kinds in a stable order, then keys
	}, got)
}

func TestIdiomStudyCandidatesCarryTheirCorpusCount(t *testing.T) {
	candidates := IdiomStudyCandidates("de", []IdiomCandidateCount{
		{Headword: "mit dem haus", Candidates: 4, Books: 2, InCurrentReading: true},
		{Headword: "gone", Candidates: 0},
	})
	require.Len(t, candidates, 1, "a headword with no candidates is not one")
	assert.Equal(t, StudyCandidate{StudyItem: IdiomItem("de", "mit dem haus"), InCurrentReading: true, PersonalOccurrences: 4}, candidates[0])
}

func TestStudyPreparationTargetsCountIdiomsInTheWindow(t *testing.T) {
	var ordered []StudyCandidate
	for i := range StudyPreparationWindow + 5 {
		ordered = append(ordered, StudyCandidate{StudyItem: IdiomItem("de", "headword "+string(rune('a'+i%26))+string(rune('a'+i/26)))})
	}
	skipped := ordered[0].StudyItem
	targets := StudyPreparationTargets(ordered, map[StudyItem]StudyMaterialState{skipped: StudyMaterialUnpreparable})
	require.Len(t, targets, StudyPreparationWindow)
	assert.NotEqual(t, skipped, targets[0].StudyItem, "an unpreparable Idiom is skipped and the next is prepared")
	assert.Equal(t, ordered[1].StudyItem, targets[0].StudyItem)
}
