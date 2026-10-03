package webapp

import (
	"encoding/json"
	"unicode/utf16"

	"github.com/justin-hayes/mouseion/internal/domain"
)

type concordanceIslandData struct {
	Occurrences []concordanceIslandOccurrence `json:"occurrences"`
}

type concordanceIslandOccurrence struct {
	ID           string `json:"id"`
	BookTitle    string `json:"bookTitle"`
	Left         string `json:"left"`
	Surface      string `json:"surface"`
	Right        string `json:"right"`
	Sentence     string `json:"sentence"`
	TargetStart  int    `json:"targetStart"`
	TargetEnd    int    `json:"targetEnd"`
	StudyURL     string `json:"studyUrl"`
}

func concordanceIslandJSON(result domain.ConcordanceResult, lookup domain.ConcordanceLookup) string {
	data := concordanceIslandData{Occurrences: make([]concordanceIslandOccurrence, 0, len(result.Occurrences))}
	for _, occurrence := range result.Occurrences {
		before, target, after := concordanceSentenceParts(occurrence)
		start, end := -1, -1
		if target != "" {
			start = len(utf16.Encode([]rune(before)))
			end = start + len(utf16.Encode([]rune(target)))
		}
		data.Occurrences = append(data.Occurrences, concordanceIslandOccurrence{
			ID: concordanceOccurrenceID(occurrence), BookTitle: occurrence.BookTitle,
			Left: concordanceBefore(occurrence), Surface: occurrence.Surface, Right: concordanceAfter(occurrence),
			Sentence: before + target + after, TargetStart: start, TargetEnd: end,
			StudyURL: vocabularySentenceStudyURL(occurrence, lookup),
		})
	}
	encoded, err := json.Marshal(data)
	if err != nil {
		// All fields are strings and integers, so this is only reachable if the
		// representation changes. An empty payload leaves the native list intact.
		return "{}"
	}
	return string(encoded)
}
