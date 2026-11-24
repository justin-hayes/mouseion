package constructions

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func conditionalTree() []Occurrence {
	return DetectWith(build(w{"εἰ", "AuxC", 2, ""}, w{"ἔχω", "ADV", 0, indic}, w{"λέγω", "PRED", -1, indic}), ShippingGate)
}

func TestShippingGate(t *testing.T) {
	tree := build(w{"εἰ", "AuxC", 2, ""}, w{"ἔχω", "ADV", 0, indic}, w{"λέγω", "PRED", -1, indic})

	t.Run("conditional ships since the release check cleared it", func(t *testing.T) {
		_, ok := has(Detect(tree), Conditional)
		assert.True(t, ok)
	})
	t.Run("switching the gate off removes it", func(t *testing.T) {
		_, ok := has(DetectWith(tree, ShippingGate.WithConstruction(Conditional, false)), Conditional)
		assert.False(t, ok)
	})
	t.Run("the shipping gate is not changed by the copy", func(t *testing.T) {
		_ = ShippingGate.WithConstruction(Conditional, false)
		assert.NotEmpty(t, conditionalTree())
	})
	t.Run("ungated detection ignores the gate", func(t *testing.T) {
		_, ok := has(DetectUngated(tree), Conditional)
		assert.True(t, ok)
	})

	labels := []struct {
		name    string
		id      ID
		subtype SubtypeID
		want    SubtypeID
	}{
		{"natural result is labelled", Result, SubtypeNatural, SubtypeNatural},
		{"actual result is not", Result, SubtypeActual, ""},
		{"infinitive indirect statement is labelled", IndirectStatement, SubtypeInfinitive, SubtypeInfinitive},
		{"ὅτι/ὡς indirect statement is not", IndirectStatement, SubtypeHotiHos, ""},
		{"genitive articular infinitive is labelled", ArticularInfinitive, SubtypeGenitive, SubtypeGenitive},
		{"dative articular infinitive is labelled", ArticularInfinitive, SubtypeDative, SubtypeDative},
		{"nominative articular infinitive is not", ArticularInfinitive, SubtypeNominative, ""},
		{"accusative articular infinitive is not", ArticularInfinitive, SubtypeAccusative, ""},
	}
	trees := map[SubtypeID]func() []Occurrence{
		SubtypeNatural: func() []Occurrence {
			return Detect(build(w{"ὥστε", "AuxC", 2, ""}, w{"ἔχω", "ADV", 0, inf}, w{"λέγω", "PRED", -1, indic}))
		},
		SubtypeActual: func() []Occurrence {
			return Detect(build(w{"ὥστε", "AuxC", 2, ""}, w{"ἔχω", "ADV", 0, indic}, w{"λέγω", "PRED", -1, indic}))
		},
		SubtypeInfinitive: func() []Occurrence {
			return Detect(build(w{"λέγω", "PRED", -1, indic}, w{"ἔχω", "OBJ", 0, inf}))
		},
		SubtypeHotiHos: func() []Occurrence {
			return Detect(build(w{"λέγω", "PRED", -1, indic}, w{"ὅτι", "AuxC", 0, ""}, w{"ἔχω", "OBJ", 1, indic}))
		},
	}
	for _, c := range []string{"Gen", "Dat", "Nom", "Acc"} {
		sub := map[string]SubtypeID{"Gen": SubtypeGenitive, "Dat": SubtypeDative, "Nom": SubtypeNominative, "Acc": SubtypeAccusative}[c]
		feats := "Gender=Neut,Number=Sing,Case=" + c
		trees[sub] = func() []Occurrence {
			return Detect(build(w{"ὁ", "ATR", 1, feats}, w{"λέγω", "ADV", -1, inf}))
		}
	}
	for _, c := range labels {
		t.Run(c.name, func(t *testing.T) {
			got, ok := has(trees[c.subtype](), c.id)
			require.True(t, ok)
			assert.Equal(t, c.want, got.Subtype)
		})
	}
}

func TestEveryEmittableConstructionHasAReviewedNote(t *testing.T) {
	for _, id := range All {
		t.Run(string(id), func(t *testing.T) {
			note, ok := Notes[id]
			require.True(t, ok, "note")
			assert.NotEmpty(t, note.Name, "name")
			assert.NotEmpty(t, note.Definition, "definition")
			assert.Positive(t, note.Citation.First, "citation")
			assert.Equal(t, ReviewTicket, note.ReviewTicket, "review ticket")
		})
	}
	assert.Len(t, Notes, len(All), "no note without a detector")
}

func TestEveryLabelledSubtypeHasALabel(t *testing.T) {
	for key := range ShippingGate.labels {
		t.Run(string(key.construction)+"/"+string(key.subtype), func(t *testing.T) {
			label, ok := SubtypeLabel(key.construction, key.subtype)
			require.True(t, ok)
			assert.NotEmpty(t, label)
		})
	}
	_, ok := SubtypeLabel(Result, SubtypeActual)
	assert.False(t, ok, "an unlabelled subtype has no label")
}

func TestCitations(t *testing.T) {
	cases := []struct {
		name string
		c    Citation
		text string
		url  string
	}{
		{"single section", Citation{First: 2257}, "Smyth 1920 §2257",
			"https://www.perseus.tufts.edu/hopper/text?doc=Perseus:text:1999.04.0007:smythp=2257"},
		{"range", Citation{First: 2280, Last: 2368}, "Smyth 1920 §§2280–2368",
			"https://www.perseus.tufts.edu/hopper/text?doc=Perseus:text:1999.04.0007:smythp=2280"},
		{"suffixed section", Citation{First: 2590, Suffix: "a"}, "Smyth 1920 §2590 a",
			"https://www.perseus.tufts.edu/hopper/text?doc=Perseus:text:1999.04.0007:smythp=2590"},
		{"last equal to first", Citation{First: 2016, Last: 2016}, "Smyth 1920 §2016",
			"https://www.perseus.tufts.edu/hopper/text?doc=Perseus:text:1999.04.0007:smythp=2016"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			assert.Equal(t, c.text, CitationText(c.c))
			assert.Equal(t, c.url, CitationURL(c.c))
		})
	}
	assert.Equal(t, GrammarReference{
		ID: "smyth-1920", Language: "grc", Label: "Smyth 1920",
		URLTemplate: "https://www.perseus.tufts.edu/hopper/text?doc=Perseus:text:1999.04.0007:smythp={§}",
	}, SmythGrammar)
}

func TestOrderingIsByFirstExtentThenAnchorThenID(t *testing.T) {
	// Two occurrences with the same first extent ordinal sort by anchor.
	tree := build(
		w{"ὁ", "ATR", 1, "Gender=Neut,Number=Sing,Case=Gen"},
		w{"λέγω", "OBJ", 3, inf},
		w{"ἔχω", "SBJ", 1, "Case=Acc"},
		w{"φημί", "PRED", -1, indic},
	)
	got := DetectUngated(tree)
	require.NotEmpty(t, got)
	for i := 1; i < len(got); i++ {
		a, b := got[i-1], got[i]
		assert.LessOrEqual(t, a.Extent[0], b.Extent[0])
	}
}
