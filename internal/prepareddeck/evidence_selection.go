package prepareddeck

import "github.com/justin-hayes/mouseion/internal/enrichment"

func evidenceSelectionIndices(senses []enrichment.LexicalSense, evidenceIDs []string) []int {
	selection := make([]int, 0, len(evidenceIDs))
	for _, id := range evidenceIDs {
		for index, sense := range senses {
			if sense.EvidenceID == id {
				selection = append(selection, index)
				break
			}
		}
	}
	return selection
}
