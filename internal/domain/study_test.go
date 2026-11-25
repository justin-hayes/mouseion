package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func studyID(lemma string) StudyIdentity {
	return StudyIdentity{Language: "de", CanonicalLemma: lemma, UPOS: "NOUN"}
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
				{StudyIdentity: studyID("alpha"), PersonalOccurrences: 90},
				{StudyIdentity: studyID("beta"), InCurrentReading: true, PersonalOccurrences: 3},
				{StudyIdentity: studyID("gamma"), InCurrentReading: true, PersonalOccurrences: 7},
				{StudyIdentity: studyID("delta"), PersonalOccurrences: 120},
			},
			want: []string{"gamma/NOUN/", "beta/NOUN/", "delta/NOUN/", "alpha/NOUN/"},
		},
		{
			name: "ties break by lemma, part of speech then discriminator",
			input: []StudyCandidate{
				{StudyIdentity: StudyIdentity{Language: "de", CanonicalLemma: "b", UPOS: "NOUN"}, PersonalOccurrences: 5},
				{StudyIdentity: StudyIdentity{Language: "de", CanonicalLemma: "a", UPOS: "VERB"}, PersonalOccurrences: 5},
				{StudyIdentity: StudyIdentity{Language: "de", CanonicalLemma: "a", UPOS: "ADJ"}, PersonalOccurrences: 5},
				{StudyIdentity: StudyIdentity{Language: "de", CanonicalLemma: "a", UPOS: "ADJ", LexicalDiscriminator: "x:2"}, PersonalOccurrences: 5},
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
		ordered = append(ordered, StudyCandidate{StudyIdentity: studyID(string(rune('a'+i%26)) + string(rune('a'+i/26)))})
	}
	t.Run("takes the first window of the stream", func(t *testing.T) {
		targets := StudyPreparationTargets(ordered, nil)
		assert.Len(t, targets, StudyPreparationWindow)
		assert.Equal(t, ordered[0].StudyIdentity, targets[0].StudyIdentity)
		assert.Equal(t, ordered[29].StudyIdentity, targets[29].StudyIdentity)
	})
	t.Run("skips unpreparable words and reaches further", func(t *testing.T) {
		states := map[StudyIdentity]StudyMaterialState{
			ordered[0].StudyIdentity: StudyMaterialUnpreparable,
			ordered[1].StudyIdentity: StudyMaterialUnpreparable,
		}
		targets := StudyPreparationTargets(ordered, states)
		assert.Len(t, targets, StudyPreparationWindow)
		assert.Equal(t, ordered[2].StudyIdentity, targets[0].StudyIdentity)
		assert.Equal(t, ordered[31].StudyIdentity, targets[29].StudyIdentity)
	})
	t.Run("a short stream yields what it has", func(t *testing.T) {
		assert.Len(t, StudyPreparationTargets(ordered[:4], nil), 4)
	})
}
