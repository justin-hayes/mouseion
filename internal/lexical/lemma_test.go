package lexical

import "testing"

func TestIsLemma(t *testing.T) {
	for _, value := range []string{"Haus", "Straße", "l'acqua", "l’acqua", "città", "B2"} {
		if !IsLemma(value) {
			t.Errorf("IsLemma(%q) = false", value)
		}
	}
	for _, value := range []string{"", " ", "5", "123.4", "—", "'", "💡"} {
		if IsLemma(value) {
			t.Errorf("IsLemma(%q) = true", value)
		}
	}
}
