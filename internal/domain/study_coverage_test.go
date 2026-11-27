package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestStudyCoverageIndexClassifiesWordsByFullIdentity(t *testing.T) {
	mature := StudyProgress{StudyItem: WordItem("de", "Bank", "NOUN", "furniture"), Memory: StudyMemory{State: StudyStateReview, ScheduledDays: FluencyMaturityDays}}
	learning := StudyProgress{StudyItem: WordItem("de", "Haus", "NOUN", ""), Memory: StudyMemory{State: StudyStateLearning}}
	withdrawn := StudyProgress{StudyItem: WordItem("de", "Dorf", "NOUN", ""), Withdrawn: true, Memory: StudyMemory{State: StudyStateReview, ScheduledDays: FluencyMaturityDays}}
	idiom := StudyProgress{StudyItem: IdiomItem("de", "Katze"), Memory: StudyMemory{State: StudyStateReview, ScheduledDays: FluencyMaturityDays}}
	index := NewStudyCoverageIndex([]StudyProgress{mature, learning, withdrawn, idiom})

	for _, tc := range []struct {
		name            string
		lemma           LemmaOccurrence
		inStudy, mature bool
	}{
		{"mature word", LemmaOccurrence{Language: "de", CanonicalLemma: "Bank", UPOS: "NOUN", LexicalDiscriminator: "furniture"}, true, true},
		{"other discriminator", LemmaOccurrence{Language: "de", CanonicalLemma: "Bank", UPOS: "NOUN", LexicalDiscriminator: "finance"}, false, false},
		{"learning word", LemmaOccurrence{Language: "de", CanonicalLemma: "Haus", UPOS: "NOUN"}, true, false},
		{"withdrawn word", LemmaOccurrence{Language: "de", CanonicalLemma: "Dorf", UPOS: "NOUN"}, false, false},
		{"idiom headword is not a word", LemmaOccurrence{Language: "de", CanonicalLemma: "Katze", UPOS: "NOUN"}, false, false},
		{"other language", LemmaOccurrence{Language: "it", CanonicalLemma: "Haus", UPOS: "NOUN"}, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inStudy, isMature := index.Classify(tc.lemma)
			assert.Equal(t, tc.inStudy, inStudy)
			assert.Equal(t, tc.mature, isMature)
		})
	}
}
