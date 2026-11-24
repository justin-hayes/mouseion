package constructions

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/justin-hayes/mouseion/internal/syntax"
)

type detectCase struct {
	name    string
	id      ID
	tree    syntax.Tree
	hit     bool
	subtype SubtypeID
	anchor  int
	extent  []int
}

func runCases(t *testing.T, cases []detectCase) {
	t.Helper()
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, ok := has(DetectUngated(c.tree), c.id)
			require.Equal(t, c.hit, ok, "hit")
			if !c.hit {
				return
			}
			assert.Equal(t, c.subtype, got.Subtype, "subtype")
			assert.Equal(t, c.anchor, got.Anchor, "anchor")
			if c.extent != nil {
				assert.Equal(t, c.extent, got.Extent, "extent")
			}
		})
	}
}

func TestConditional(t *testing.T) {
	main := w{"λέγω", "PRED", -1, indic}
	runCases(t, []detectCase{
		{name: "εἰ with a verb clause", id: Conditional, hit: true, anchor: 0, extent: []int{0, 1},
			tree: build(w{"εἰ", "AuxC", 2, ""}, w{"ἔχω", "ADV", 0, indic}, main)},
		{name: "ἐάν", id: Conditional, hit: true, anchor: 0,
			tree: build(w{"ἐάν", "AuxC", 2, ""}, w{"ἔχω", "ADV", 0, subj}, main)},
		{name: "εἴπερ", id: Conditional, hit: true, anchor: 0,
			tree: build(w{"εἴπερ", "AuxC", 2, ""}, w{"ἔχω", "ADV", 0, indic}, main)},
		{name: "ἄν", id: Conditional, hit: true, anchor: 0,
			tree: build(w{"ἄν", "AuxC", 2, ""}, w{"ἔχω", "ADV", 0, subj}, main)},
		{name: "clause through COORD", id: Conditional, hit: true, anchor: 0,
			tree: build(w{"εἰ", "AuxC", 3, ""}, w{"καί", "COORD", 0, ""}, w{"ἔχω", "ADV", 1, indic}, main)},
		{name: "unprinted ADV clause", id: Conditional, hit: true, anchor: 0,
			tree: build(w{"εἰ", "AuxC", 2, ""}, w{"", "ADV", 0, ""}, main)},
		{name: "no whether exclusion", id: Conditional, hit: true, anchor: 1,
			tree: build(w{"οἶδα", "PRED", -1, indic}, w{"εἰ", "AuxC", 0, ""}, w{"ἔχω", "ADV", 1, indic})},
		{name: "after ὡς", id: Conditional, hit: false,
			tree: build(w{"ὡς", "AuxC", 3, ""}, w{"εἰ", "AuxC", 3, ""}, w{"ἔχω", "ADV", 1, indic}, main)},
		{name: "head is ὥσπερ", id: Conditional, hit: false,
			tree: build(w{"ὥσπερ", "AuxC", 3, ""}, w{"εἰ", "AuxC", 0, ""}, w{"ἔχω", "ADV", 1, indic}, main)},
		{name: "no clause child", id: Conditional, hit: false,
			tree: build(w{"εἰ", "AuxC", 1, ""}, main)},
		{name: "clause child is not a verb", id: Conditional, hit: false,
			tree: build(w{"εἰ", "AuxC", 2, ""}, w{"ἄνθρωπος", "ADV", 0, ""}, main)},
		{name: "wrong relation", id: Conditional, hit: false,
			tree: build(w{"εἰ", "ADV", 2, ""}, w{"ἔχω", "ADV", 0, indic}, main)},
	})
}

func TestArticularInfinitive(t *testing.T) {
	art := func(feats string) w { return w{"ὁ", "ATR", 1, feats} }
	runCases(t, []detectCase{
		{name: "genitive article", id: ArticularInfinitive, hit: true, subtype: SubtypeGenitive, anchor: 1, extent: []int{0, 1},
			tree: build(art("Gender=Neut,Number=Sing,Case=Gen"), w{"λέγω", "ADV", -1, inf})},
		{name: "dative article", id: ArticularInfinitive, hit: true, subtype: SubtypeDative, anchor: 1,
			tree: build(art("Gender=Neut,Number=Sing,Case=Dat"), w{"λέγω", "ADV", -1, inf})},
		{name: "nominative article", id: ArticularInfinitive, hit: true, subtype: SubtypeNominative, anchor: 1,
			tree: build(art("Gender=Neut,Number=Sing,Case=Nom"), w{"λέγω", "SBJ", -1, inf})},
		{name: "accusative article", id: ArticularInfinitive, hit: true, subtype: SubtypeAccusative, anchor: 1,
			tree: build(art("Gender=Neut,Number=Sing,Case=Acc"), w{"λέγω", "OBJ", -1, inf})},
		{name: "article under a preposition", id: ArticularInfinitive, hit: true, subtype: SubtypeGenitive, anchor: 2,
			tree: build(w{"ἀντί", "AuxP", -1, ""}, w{"ὁ", "ATR", 2, "Gender=Neut,Number=Sing,Case=Gen"}, w{"λέγω", "ADV", 0, inf})},
		{name: "masculine article", id: ArticularInfinitive, hit: false,
			tree: build(art("Gender=Masc,Number=Sing,Case=Gen"), w{"λέγω", "ADV", -1, inf})},
		{name: "plural article", id: ArticularInfinitive, hit: false,
			tree: build(art("Gender=Neut,Number=Plur,Case=Gen"), w{"λέγω", "ADV", -1, inf})},
		{name: "finite verb", id: ArticularInfinitive, hit: false,
			tree: build(art("Gender=Neut,Number=Sing,Case=Gen"), w{"λέγω", "ADV", -1, indic})},
		{name: "unprinted article", id: ArticularInfinitive, hit: false,
			tree: build(w{"", "ATR", 1, ""}, w{"λέγω", "ADV", -1, inf})},
		{name: "no article", id: ArticularInfinitive, hit: false,
			tree: build(w{"λέγω", "ADV", -1, inf})},
	})
}

func TestResult(t *testing.T) {
	runCases(t, []detectCase{
		{name: "ὥστε with an infinitive", id: Result, hit: true, subtype: SubtypeNatural, anchor: 0, extent: []int{0, 1},
			tree: build(w{"ὥστε", "AuxC", 2, ""}, w{"ἔχω", "ADV", 0, inf}, w{"λέγω", "PRED", -1, indic})},
		{name: "ὥστε with a finite verb", id: Result, hit: true, subtype: SubtypeActual, anchor: 0,
			tree: build(w{"ὥστε", "AuxC", 2, ""}, w{"ἔχω", "ADV", 0, indic}, w{"λέγω", "PRED", -1, indic})},
		{name: "unprinted verb has no subtype", id: Result, hit: true, subtype: "", anchor: 0,
			tree: build(w{"ὥστε", "AuxC", 2, ""}, w{"", "ADV", 0, ""}, w{"λέγω", "PRED", -1, indic})},
		{name: "clause through COORD", id: Result, hit: true, subtype: SubtypeNatural, anchor: 0,
			tree: build(w{"ὥστε", "AuxC", 3, ""}, w{"καί", "COORD", 0, ""}, w{"ἔχω", "ADV", 1, inf}, w{"λέγω", "PRED", -1, indic})},
		{name: "participle child", id: Result, hit: false,
			tree: build(w{"ὥστε", "AuxC", 2, ""}, w{"ἔχω", "ADV", 0, part}, w{"λέγω", "PRED", -1, indic})},
		{name: "no clause child", id: Result, hit: false,
			tree: build(w{"ὥστε", "AuxC", 1, ""}, w{"λέγω", "PRED", -1, indic})},
		{name: "not AuxC", id: Result, hit: false,
			tree: build(w{"ὥστε", "ADV", 2, ""}, w{"ἔχω", "ADV", 0, inf}, w{"λέγω", "PRED", -1, indic})},
	})
}

func TestPurpose(t *testing.T) {
	main := w{"λέγω", "PRED", -1, indic}
	runCases(t, []detectCase{
		{name: "ἵνα with a subjunctive", id: Purpose, hit: true, anchor: 0, extent: []int{0, 1},
			tree: build(w{"ἵνα", "AuxC", 2, ""}, w{"ἔχω", "ADV", 0, subj}, main)},
		{name: "ἵνα with an optative", id: Purpose, hit: true, anchor: 0,
			tree: build(w{"ἵνα", "AuxC", 2, ""}, w{"ἔχω", "ADV", 0, opt}, main)},
		{name: "ἵνα through COORD", id: Purpose, hit: true, anchor: 0,
			tree: build(w{"ἵνα", "AuxC", 3, ""}, w{"καί", "COORD", 0, ""}, w{"ἔχω", "ADV", 1, subj}, main)},
		{name: "ὅπως is deferred", id: Purpose, hit: false,
			tree: build(w{"ὅπως", "AuxC", 2, ""}, w{"ἔχω", "ADV", 0, subj}, main)},
		{name: "indicative", id: Purpose, hit: false,
			tree: build(w{"ἵνα", "AuxC", 2, ""}, w{"ἔχω", "ADV", 0, indic}, main)},
		{name: "unprinted clause", id: Purpose, hit: false,
			tree: build(w{"ἵνα", "AuxC", 2, ""}, w{"", "ADV", 0, ""}, main)},
	})
}

func TestGenitiveAbsolute(t *testing.T) {
	gen := "VerbForm=Part,Case=Gen"
	runCases(t, []detectCase{
		{name: "participle with genitive subject", id: GenitiveAbsolute, hit: true, anchor: 1, extent: []int{0, 1},
			tree: build(w{"ἥλιος", "SBJ", 1, "Case=Gen"}, w{"δύω", "ADV", 2, gen}, w{"λέγω", "PRED", -1, indic})},
		{name: "subject through COORD", id: GenitiveAbsolute, hit: true, anchor: 0,
			tree: build(w{"δύω", "ADV", 1, gen}, w{"καί", "COORD", 3, ""}, w{"ἥλιος", "SBJ", 1, "Case=Gen"}, w{"λέγω", "PRED", -1, indic})},
		{name: "perception head", id: GenitiveAbsolute, hit: false,
			tree: build(w{"ἥλιος", "SBJ", 1, "Case=Gen"}, w{"δύω", "ADV", 2, gen}, w{"ἀκούω", "PRED", -1, indic})},
		{name: "perception head through COORD", id: GenitiveAbsolute, hit: false,
			tree: build(w{"ἥλιος", "SBJ", 1, "Case=Gen"}, w{"δύω", "ADV", 2, gen}, w{"καί", "COORD", 3, ""}, w{"πυνθάνομαι", "PRED", -1, indic})},
		{name: "no genitive subject", id: GenitiveAbsolute, hit: false,
			tree: build(w{"ἥλιος", "SBJ", 1, "Case=Nom"}, w{"δύω", "ADV", 2, gen}, w{"λέγω", "PRED", -1, indic})},
		{name: "unprinted subject", id: GenitiveAbsolute, hit: false,
			tree: build(w{"", "SBJ", 1, ""}, w{"δύω", "ADV", 2, gen}, w{"λέγω", "PRED", -1, indic})},
		{name: "participle not ADV", id: GenitiveAbsolute, hit: false,
			tree: build(w{"ἥλιος", "SBJ", 1, "Case=Gen"}, w{"δύω", "ATR", 2, gen}, w{"λέγω", "PRED", -1, indic})},
		{name: "participle not genitive", id: GenitiveAbsolute, hit: false,
			tree: build(w{"ἥλιος", "SBJ", 1, "Case=Gen"}, w{"δύω", "ADV", 2, "VerbForm=Part,Case=Nom"}, w{"λέγω", "PRED", -1, indic})},
		{name: "unprinted participle", id: GenitiveAbsolute, hit: false,
			tree: build(w{"ἥλιος", "SBJ", 1, "Case=Gen"}, w{"", "ADV", 2, ""}, w{"λέγω", "PRED", -1, indic})},
	})
}

func TestIndirectStatement(t *testing.T) {
	say := w{"λέγω", "PRED", -1, indic}
	runCases(t, []detectCase{
		{name: "infinitive OBJ with accusative subject", id: IndirectStatement, hit: true, subtype: SubtypeInfinitive, anchor: 2, extent: []int{1, 2},
			tree: build(say, w{"ἄνθρωπος", "SBJ", 2, "Case=Acc"}, w{"ἔχω", "OBJ", 0, inf})},
		{name: "infinitive with no subject", id: IndirectStatement, hit: true, subtype: SubtypeInfinitive, anchor: 1,
			tree: build(say, w{"ἔχω", "OBJ", 0, inf})},
		{name: "infinitive SBJ", id: IndirectStatement, hit: true, subtype: SubtypeInfinitive, anchor: 1,
			tree: build(say, w{"ἔχω", "SBJ", 0, inf})},
		{name: "head through COORD", id: IndirectStatement, hit: true, subtype: SubtypeInfinitive, anchor: 2,
			tree: build(say, w{"καί", "COORD", 0, ""}, w{"ἔχω", "OBJ", 1, inf})},
		{name: "future-only verb with a future infinitive", id: IndirectStatement, hit: true, subtype: SubtypeInfinitive, anchor: 1,
			tree: build(w{"ὑπισχνέομαι", "PRED", -1, indic}, w{"ἔχω", "OBJ", 0, "VerbForm=Inf,Tense=Fut"})},
		{name: "future-only verb with a present infinitive", id: IndirectStatement, hit: false,
			tree: build(w{"ὄμνυμι", "PRED", -1, indic}, w{"ἔχω", "OBJ", 0, "VerbForm=Inf,Tense=Pres"})},
		{name: "future-only verb with a future perfect infinitive", id: IndirectStatement, hit: false,
			tree: build(w{"ὄμνυμι", "PRED", -1, indic}, w{"ἔχω", "OBJ", 0, "VerbForm=Inf,Tense=Fut,Aspect=Perf"})},
		{name: "δοκέω is not on the infinitive list", id: IndirectStatement, hit: false,
			tree: build(w{"δοκέω", "PRED", -1, indic}, w{"ἔχω", "OBJ", 0, inf})},
		{name: "verb of will", id: IndirectStatement, hit: false,
			tree: build(w{"κελεύω", "PRED", -1, indic}, w{"ἔχω", "OBJ", 0, inf})},
		{name: "nominative subject", id: IndirectStatement, hit: false,
			tree: build(say, w{"ἄνθρωπος", "SBJ", 2, "Case=Nom"}, w{"ἔχω", "OBJ", 0, inf})},
		{name: "unprinted subject", id: IndirectStatement, hit: false,
			tree: build(say, w{"", "SBJ", 2, ""}, w{"ἔχω", "OBJ", 0, inf})},
		{name: "infinitive not OBJ or SBJ", id: IndirectStatement, hit: false,
			tree: build(say, w{"ἔχω", "ADV", 0, inf})},
		{name: "unprinted governing verb", id: IndirectStatement, hit: false,
			tree: build(w{"", "PRED", -1, ""}, w{"ἔχω", "OBJ", 0, inf})},
		{name: "ὅτι with an indicative", id: IndirectStatement, hit: true, subtype: SubtypeHotiHos, anchor: 1, extent: []int{1, 2},
			tree: build(say, w{"ὅτι", "AuxC", 0, ""}, w{"ἔχω", "OBJ", 1, indic})},
		{name: "ὡς with an optative", id: IndirectStatement, hit: true, subtype: SubtypeHotiHos, anchor: 1,
			tree: build(say, w{"ὡς", "AuxC", 0, ""}, w{"ἔχω", "SBJ", 1, opt})},
		{name: "ὅτι under a knowing verb", id: IndirectStatement, hit: true, subtype: SubtypeHotiHos, anchor: 1,
			tree: build(w{"οἶδα", "PRED", -1, indic}, w{"ὅτι", "AuxC", 0, ""}, w{"ἔχω", "OBJ", 1, indic})},
		{name: "ὅτι under δοκέω", id: IndirectStatement, hit: true, subtype: SubtypeHotiHos, anchor: 1,
			tree: build(w{"δοκέω", "PRED", -1, indic}, w{"ὅτι", "AuxC", 0, ""}, w{"ἔχω", "OBJ", 1, indic})},
		{name: "ὅτι under an unlisted verb", id: IndirectStatement, hit: false,
			tree: build(w{"γιγνώσκω", "PRED", -1, indic}, w{"ὅτι", "AuxC", 0, ""}, w{"ἔχω", "OBJ", 1, indic})},
		{name: "ὅτι with a participle", id: IndirectStatement, hit: false,
			tree: build(say, w{"ὅτι", "AuxC", 0, ""}, w{"ἔχω", "OBJ", 1, part})},
		{name: "ὅτι with an unprinted verb", id: IndirectStatement, hit: false,
			tree: build(say, w{"ὅτι", "AuxC", 0, ""}, w{"", "OBJ", 1, ""})},
		{name: "ὅτι with an adverbial clause", id: IndirectStatement, hit: false,
			tree: build(say, w{"ὅτι", "AuxC", 0, ""}, w{"ἔχω", "ADV", 1, indic})},
	})
}

func TestAnchorsAreUniqueAndOutputIsSorted(t *testing.T) {
	// ὅτι governs an OBJ verb that is also an articular infinitive and an
	// infinitive-bearing object: several constructions, one tree.
	tree := build(
		w{"λέγω", "PRED", -1, indic},
		w{"ὅτι", "AuxC", 0, ""},
		w{"ἔχω", "OBJ", 1, indic},
		w{"ἵνα", "AuxC", 0, ""},
		w{"ἔχω", "ADV", 3, subj},
	)
	got := DetectUngated(tree)
	require.Len(t, got, 2)
	assert.Equal(t, IndirectStatement, got[0].Construction)
	assert.Equal(t, Purpose, got[1].Construction)
}

func TestUnprintedCoordinatorFailsClosed(t *testing.T) {
	say := w{"λέγω", "PRED", -1, indic}
	runCases(t, []detectCase{
		{name: "purpose clause only through an unprinted COORD", id: Purpose, hit: false,
			tree: build(w{"ἵνα", "AuxC", 3, ""}, w{"", "COORD", 0, ""}, w{"ἔχω", "ADV", 1, subj}, say)},
		{name: "purpose clause through a printed COORD", id: Purpose, hit: true, anchor: 0,
			tree: build(w{"ἵνα", "AuxC", 3, ""}, w{"καί", "COORD", 0, ""}, w{"ἔχω", "ADV", 1, subj}, say)},
		{name: "infinitive governor only through an unprinted COORD", id: IndirectStatement, hit: false,
			tree: build(say, w{"", "COORD", 0, ""}, w{"ἔχω", "OBJ", 1, inf})},
		{name: "infinitive subject only through an unprinted COORD", id: IndirectStatement, hit: false,
			tree: build(say, w{"", "COORD", 2, ""}, w{"ἔχω", "OBJ", 0, inf}, w{"ἄνθρωπος", "SBJ", 1, "Case=Nom"})},
		{name: "ὅτι clause only through an unprinted COORD", id: IndirectStatement, hit: false,
			tree: build(say, w{"ὅτι", "AuxC", 0, ""}, w{"", "COORD", 1, ""}, w{"ἔχω", "OBJ", 2, indic})},
		{name: "ὅτι governor only through an unprinted COORD", id: IndirectStatement, hit: false,
			tree: build(say, w{"", "COORD", 0, ""}, w{"ὅτι", "AuxC", 1, ""}, w{"ἔχω", "OBJ", 2, indic})},
		{name: "conditional clause only through an unprinted COORD", id: Conditional, hit: false,
			tree: build(w{"εἰ", "AuxC", 3, ""}, w{"", "COORD", 0, ""}, w{"ἔχω", "ADV", 1, indic}, say)},
		{name: "result clause only through an unprinted COORD", id: Result, hit: false,
			tree: build(w{"ὥστε", "AuxC", 3, ""}, w{"", "COORD", 0, ""}, w{"ἔχω", "ADV", 1, inf}, say)},
		{name: "genitive subject only through an unprinted COORD", id: GenitiveAbsolute, hit: false,
			tree: build(w{"ἥλιος", "SBJ", 1, "Case=Gen"}, w{"", "COORD", 3, ""}, w{"δύω", "ADV", 1, "VerbForm=Part,Case=Gen"}, say)},
		{name: "participle head only through an unprinted COORD", id: GenitiveAbsolute, hit: false,
			tree: build(w{"ἥλιος", "SBJ", 1, "Case=Gen"}, w{"δύω", "ADV", 2, "VerbForm=Part,Case=Gen"}, w{"", "COORD", 3, ""}, say)},
	})
}

func TestResultKeepsAPrintedVerbOfUnknownMood(t *testing.T) {
	runCases(t, []detectCase{
		{name: "printed ADV verb with no mood", id: Result, hit: true, subtype: "", anchor: 0, extent: []int{0, 1},
			tree: build(w{"ὥστε", "AuxC", 2, ""}, w{"ἔχω", "ADV", 0, "UPOS=VERB"}, w{"λέγω", "PRED", -1, indic})},
		{name: "printed ADV verb with an unmapped mood", id: Result, hit: true, subtype: "", anchor: 0,
			tree: build(w{"ὥστε", "AuxC", 2, ""}, w{"ἔχω", "ADV", 0, "UPOS=VERB,VerbForm=Fin"}, w{"λέγω", "PRED", -1, indic})},
	})
}

func TestUnprintedAdverbialClauseCountsThroughAnUnprintedCoordinator(t *testing.T) {
	say := w{"λέγω", "PRED", -1, indic}
	runCases(t, []detectCase{
		{name: "conditional", id: Conditional, hit: true, anchor: 0, extent: []int{0},
			tree: build(w{"εἰ", "AuxC", 3, ""}, w{"", "COORD", 0, ""}, w{"", "ADV", 1, ""}, say)},
		{name: "result", id: Result, hit: true, subtype: "", anchor: 0, extent: []int{0},
			tree: build(w{"ὥστε", "AuxC", 3, ""}, w{"", "COORD", 0, ""}, w{"", "ADV", 1, ""}, say)},
	})
}
