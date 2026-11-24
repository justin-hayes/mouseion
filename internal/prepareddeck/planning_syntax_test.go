package prepareddeck

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/stretchr/testify/assert"
)

func TestCandidateProjectionsCarryTheRunsSyntaxState(t *testing.T) {
	facts := []persistence.PreparedDeckCandidateFacts{{SyntaxState: "ud"}, {SyntaxState: "native_agdt"}}

	projections := candidateProjections(domain.DeckPreparation{}, "deck", facts, domain.SnapshotStructure{}, nil)

	assert.Equal(t, "ud", projections[0].SyntaxState)
	assert.Equal(t, "native_agdt", projections[1].SyntaxState)
}
