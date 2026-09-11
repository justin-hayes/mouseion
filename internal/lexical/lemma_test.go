package lexical

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsLemma(t *testing.T) {
	for _, value := range []string{"Haus", "Straße", "l'acqua", "l’acqua", "città", "B2"} {
		assert.True(t, IsLemma(value), value)
	}
	for _, value := range []string{"", " ", "5", "123.4", "—", "'", "💡"} {
		assert.False(t, IsLemma(value), value)
	}
}
