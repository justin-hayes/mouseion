package canonicalization

import "testing"

func TestLemma(t *testing.T) {
	if got := Lemma("  Haus "); got != "haus" {
		t.Fatalf("Lemma() = %q, want %q", got, "haus")
	}
}
