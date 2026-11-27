package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStudyItemKindsAreReadAsStored(t *testing.T) {
	t.Run("an unknown kind is refused", func(t *testing.T) {
		_, ok := ParseStudyItemKind("phrase")
		require.False(t, ok)
	})
	t.Run("a word has no headword", func(t *testing.T) {
		_, ok := WordItem("de", "haus", "NOUN", "").Headword()
		require.False(t, ok)
	})
	t.Run("an Idiom's headword is its key", func(t *testing.T) {
		headword, ok := IdiomItem("de", "mit dem haus").Headword()
		require.True(t, ok)
		assert.Equal(t, "mit dem haus", headword)
	})
}

func TestStudyItemsAreOrderedByLemmaPartOfSpeechDiscriminatorAndKind(t *testing.T) {
	cases := []struct {
		name string
		a, b StudyItem
	}{
		{"by lemma", WordItem("de", "apfel", "NOUN", ""), WordItem("de", "baum", "NOUN", "")},
		{"by part of speech", WordItem("de", "haus", "ADJ", ""), WordItem("de", "haus", "NOUN", "")},
		{"by discriminator", WordItem("de", "haus", "NOUN", "a"), WordItem("de", "haus", "NOUN", "b")},
		{"by kind", WordItem("de", "mit dem haus", "", ""), IdiomItem("de", "mit dem haus")},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.True(t, studyIdentityBefore(c.a, c.b))
			require.False(t, studyIdentityBefore(c.b, c.a))
		})
	}
}
