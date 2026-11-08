package enrichment_test

import (
	"testing"

	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/stretchr/testify/assert"
)

func TestItemKindTreatsTheZeroValueAsLemma(t *testing.T) {
	assert.Equal(t, enrichment.ItemKindLemma, enrichment.ItemKind("").Normalized())
	assert.Equal(t, enrichment.ItemKindIdiom, enrichment.ItemKindIdiom.Normalized())
	assert.Equal(t, enrichment.ItemKindLemma, enrichment.ItemKindLemma.Normalized())
}

func TestItemKindValid(t *testing.T) {
	assert.True(t, enrichment.ItemKindLemma.Valid())
	assert.True(t, enrichment.ItemKindIdiom.Valid())
	assert.True(t, enrichment.ItemKind("").Valid(), "an unset kind is a lemma")
	assert.False(t, enrichment.ItemKind("phrase").Valid())
}

func TestItemKindFromStoredCompactsLemmaOnly(t *testing.T) {
	assert.Equal(t, enrichment.ItemKind(""), enrichment.ItemKindFromStored("lemma"))
	assert.Equal(t, enrichment.ItemKindIdiom, enrichment.ItemKindFromStored("idiom"))
	assert.Equal(t, enrichment.ItemKind("phrase"), enrichment.ItemKindFromStored("phrase"), "an unknown value is preserved for validation")
}
