package cardexport

import (
	"bytes"
	"context"
	"log"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPresentationFreezeLogsGlossCoverage(t *testing.T) {
	var output bytes.Buffer
	previousWriter, previousFlags := log.Writer(), log.Flags()
	log.SetOutput(&output)
	log.SetFlags(0)
	defer func() {
		log.SetOutput(previousWriter)
		log.SetFlags(previousFlags)
	}()

	_, _, err := NewPresentation(nil).Freeze(context.Background(), []CandidateProjection{
		{OwnerID: "owner", DeckName: "Book", Candidate: domain.SelectionCandidate{OwnerID: "owner", Language: "de", CanonicalLemma: "haus", UPOS: "NOUN"}, Entry: Entry{Language: "de", CanonicalLemma: "haus", UPOS: "NOUN", Sentence: "Das Haus steht dort.", TargetWord: "Haus", Gloss: "house"}},
		{OwnerID: "owner", DeckName: "Book", Candidate: domain.SelectionCandidate{OwnerID: "owner", Language: "de", CanonicalLemma: "fragment", UPOS: "NOUN"}, Entry: Entry{Language: "de", CanonicalLemma: "fragment", UPOS: "NOUN", Sentence: "Das Fragment steht dort.", TargetWord: "Fragment"}},
	})
	require.NoError(t, err)
	assert.Contains(t, output.String(), `gloss_coverage {"event":"gloss_coverage","groups":[{"language":"de","pos":"NOUN","selected":2,"with_gloss":1,"without_gloss":1}]}`)
}
