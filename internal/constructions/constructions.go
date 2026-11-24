// Package constructions finds Construction occurrences in one sentence's
// syntax tree. It is a pure reading of a syntax.Tree: no database, no UI. The
// six detectors are the measured Ancient Greek definitions over native AGDT
// (ADR 0123); the concept notes and grammar reference are ADR 0124.
package constructions

import (
	"cmp"
	"maps"
	"slices"

	"github.com/justin-hayes/mouseion/internal/syntax"
)

// AGDTDetectorVersion is the construction detector version for the AGDT
// scheme. It covers the adapter's normalisation, the six detectors, the verb
// lists and the shipping gate, and is bumped by hand: the golden test fails
// when the output changes while the recorded version equals this one.
const AGDTDetectorVersion = 2

// ID is a stable construction key.
type ID string

// The six first-slice constructions.
const (
	Conditional         ID = "conditional"
	ArticularInfinitive ID = "articular_infinitive"
	Result              ID = "result"
	Purpose             ID = "purpose"
	GenitiveAbsolute    ID = "genitive_absolute"
	IndirectStatement   ID = "indirect_statement"
)

// SubtypeID is a stable construction subtype key. The empty value means the
// occurrence is unlabelled or its subtype is undetermined.
type SubtypeID string

// The construction subtypes the detectors can determine.
const (
	SubtypeNatural    SubtypeID = "natural"
	SubtypeActual     SubtypeID = "actual"
	SubtypeInfinitive SubtypeID = "infinitive"
	SubtypeHotiHos    SubtypeID = "hoti_hos"
	SubtypeGenitive   SubtypeID = "genitive"
	SubtypeDative     SubtypeID = "dative"
	SubtypeNominative SubtypeID = "nominative"
	SubtypeAccusative SubtypeID = "accusative"
)

// All lists every construction id the detectors can emit, in a fixed order.
var All = []ID{Conditional, ArticularInfinitive, Result, Purpose, GenitiveAbsolute, IndirectStatement}

// Occurrence is one construction found in one sentence. Anchor is a printed
// token ordinal. Extent holds the sorted printed token ordinals of the
// anchor's subtree; it may be discontinuous and always contains the anchor.
type Occurrence struct {
	Construction ID
	Subtype      SubtypeID
	Anchor       int
	Extent       []int
}

// Gate is the shipping gate: which constructions and which subtype labels are
// emitted.
type Gate struct {
	constructions map[ID]bool
	labels        map[labelKey]bool
}

type labelKey struct {
	construction ID
	subtype      SubtypeID
}

// ShippingGate is the compiled gate. Conditional ships since the release
// check (#2622) cleared it at 0.912; only four subtype labels clear their bar.
var ShippingGate = Gate{
	constructions: map[ID]bool{
		Conditional: true, ArticularInfinitive: true, Result: true, Purpose: true,
		GenitiveAbsolute: true, IndirectStatement: true,
	},
	labels: map[labelKey]bool{
		{Result, SubtypeNatural}:               true,
		{IndirectStatement, SubtypeInfinitive}: true,
		{ArticularInfinitive, SubtypeGenitive}: true,
		{ArticularInfinitive, SubtypeDative}:   true,
	},
}

// WithConstruction returns a copy of the gate with one construction switched
// on or off.
func (g Gate) WithConstruction(id ID, on bool) Gate {
	next := Gate{constructions: maps.Clone(g.constructions), labels: g.labels}
	next.constructions[id] = on
	return next
}

// Ships reports whether the gate emits a construction.
func (g Gate) Ships(id ID) bool { return g.constructions[id] }

// ShipsLabel reports whether the gate emits a construction's subtype label.
func (g Gate) ShipsLabel(id ID, subtype SubtypeID) bool {
	return g.constructions[id] && g.labels[labelKey{id, subtype}]
}

// Detect returns the shipped occurrences of one sentence, sorted by first
// extent ordinal, then anchor, then construction id. Subtypes that do not
// clear the gate come out as "".
func Detect(t syntax.Tree) []Occurrence {
	return DetectWith(t, ShippingGate)
}

// DetectWith applies a gate to the ungated detector output.
func DetectWith(t syntax.Tree, g Gate) []Occurrence {
	var out []Occurrence
	for _, occurrence := range DetectUngated(t) {
		if !g.constructions[occurrence.Construction] {
			continue
		}
		if !g.labels[labelKey{occurrence.Construction, occurrence.Subtype}] {
			occurrence.Subtype = ""
		}
		out = append(out, occurrence)
	}
	return out
}

// DetectUngated returns every detector's occurrences with every determinable
// subtype, whatever the shipping gate says. The release check (#2622) measures
// the detectors through it.
func DetectUngated(t syntax.Tree) []Occurrence {
	r := reader{t}
	var out []Occurrence
	seen := map[labelKey]map[int]bool{}
	for anchor, node := range t.Nodes {
		if !node.Printed {
			continue
		}
		for _, d := range detectors {
			subtype, ok := d.detect(r, anchor)
			if !ok {
				continue
			}
			key := labelKey{construction: d.id}
			if seen[key] == nil {
				seen[key] = map[int]bool{}
			}
			if seen[key][anchor] {
				continue
			}
			seen[key][anchor] = true
			out = append(out, Occurrence{
				Construction: d.id, Subtype: subtype, Anchor: int(node.Ordinal), Extent: r.extent(anchor),
			})
		}
	}
	slices.SortFunc(out, func(a, b Occurrence) int {
		return cmp.Or(cmp.Compare(a.Extent[0], b.Extent[0]), cmp.Compare(a.Anchor, b.Anchor), cmp.Compare(a.Construction, b.Construction))
	})
	return out
}
