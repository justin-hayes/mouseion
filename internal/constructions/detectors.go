package constructions

import (
	"slices"
	"strings"

	"github.com/justin-hayes/mouseion/internal/syntax"
)

type detector struct {
	id     ID
	detect func(r reader, n int) (SubtypeID, bool)
}

var detectors = []detector{
	{Conditional, detectConditional},
	{ArticularInfinitive, detectArticularInfinitive},
	{Result, detectResult},
	{Purpose, detectPurpose},
	{GenitiveAbsolute, detectGenitiveAbsolute},
	{IndirectStatement, detectIndirectStatement},
}

// reader reads a tree for the detectors. Every helper treats an unprinted
// node as having no lemma and no morphology, so a required printed input that
// is missing, or is an unprinted node, never matches (fail closed).
type reader struct{ t syntax.Tree }

func (r reader) node(i int) syntax.Node { return r.t.Nodes[i] }

// lemma is the source lemma without a homograph number; empty when unprinted.
func (r reader) lemma(i int) string {
	n := r.node(i)
	if !n.Printed {
		return ""
	}
	return strings.TrimRight(n.Lemma, "0123456789")
}

func (r reader) feature(i int, name string) string {
	n := r.node(i)
	if !n.Printed {
		return ""
	}
	return n.Morphology[name]
}

func (r reader) category(i int) syntax.Category { return r.node(i).Category }

// verb reports a printed verb form. With moods, it must be finite in one of
// them.
func (r reader) verb(i int, moods ...string) bool {
	form := r.feature(i, "VerbForm")
	if len(moods) == 0 {
		// A printed verb whose mood is unknown is still a verb.
		return form != "" || (r.node(i).Printed && r.node(i).UPOS == "VERB")
	}
	return form == "Fin" && slices.Contains(moods, r.feature(i, "Mood"))
}

func (r reader) verbForm(i int, form string) bool { return r.feature(i, "VerbForm") == form }

func (r reader) extent(i int) []int {
	var out []int
	for _, n := range r.t.PrintedSubtree(i) {
		out = append(out, int(r.node(n).Ordinal))
	}
	return out
}

// head is the effective head, looking up through printed coordinators only. An
// unprinted coordinator on the way makes the head unknown, so ok is false; a
// node that depends on nothing returns syntax.Root with ok true.
func (r reader) head(i int) (head int, ok bool) {
	seen := map[int]bool{i: true}
	head = r.node(i).Head
	for head != syntax.Root && r.category(head) == syntax.CategoryCoordination {
		if !r.node(head).Printed || seen[head] {
			return syntax.Root, false
		}
		seen[head] = true
		head = r.node(head).Head
	}
	return head, true
}

// children returns the children of i, looking down through printed
// coordinators only: the children of an unprinted coordinator are not reached.
// When categories are given only those are returned.
func (r reader) children(i int, categories ...syntax.Category) []int {
	var out []int
	seen := map[int]bool{i: true}
	var walk func(parent int)
	walk = func(parent int) {
		for _, child := range r.t.Children(parent) {
			if seen[child] {
				continue
			}
			seen[child] = true
			switch {
			case r.category(child) == syntax.CategoryCoordination:
				if r.node(child).Printed {
					walk(child)
				}
			case len(categories) == 0 || slices.Contains(categories, r.category(child)):
				out = append(out, child)
			}
		}
	}
	walk(i)
	return out
}

// clauseChildren returns the adverbial children (through coordination) that
// are verbs, or, with unprinted allowed, unprinted nodes. moods restricts the
// verbs to finite forms in those moods.
func (r reader) clauseChildren(i int, unprinted bool, moods ...string) []int {
	var out []int
	for _, child := range r.children(i, syntax.CategoryAdverbial) {
		switch {
		case r.node(child).Printed && r.verb(child, moods...):
			out = append(out, child)
		case unprinted && !r.node(child).Printed:
			out = append(out, child)
		}
	}
	if unprinted {
		out = append(out, r.unprintedAdverbials(i, out)...)
		slices.Sort(out)
	}
	return out
}

// unprintedAdverbials returns the unprinted ADV nodes under i that are
// reached through coordinators, printed or not, and are not already in have.
// The conditional and result definitions accept such a node as the clause by
// its relation alone.
func (r reader) unprintedAdverbials(i int, have []int) []int {
	var out []int
	seen := map[int]bool{i: true}
	var walk func(parent int)
	walk = func(parent int) {
		for _, child := range r.t.Children(parent) {
			if seen[child] {
				continue
			}
			seen[child] = true
			switch {
			case r.category(child) == syntax.CategoryCoordination:
				walk(child)
			case !r.node(child).Printed && r.category(child) == syntax.CategoryAdverbial && !slices.Contains(have, child):
				out = append(out, child)
			}
		}
	}
	walk(i)
	return out
}

func (r reader) isConjunction(i int, lemmas ...string) bool {
	return r.node(i).Printed && r.category(i) == syntax.CategorySubordinatingConjunction && slices.Contains(lemmas, r.lemma(i))
}

func detectConditional(r reader, n int) (SubtypeID, bool) {
	if !r.isConjunction(n, "εἰ", "ἐάν", "εἴπερ", "ἄν") || len(r.clauseChildren(n, true)) == 0 {
		return "", false
	}
	if previous := r.t.PreviousPrinted(n); previous != syntax.Root && likeLemmas[r.lemma(previous)] {
		return "", false
	}
	if head := r.node(n).Head; head != syntax.Root && likeLemmas[r.lemma(head)] {
		return "", false
	}
	return "", true
}

func isNeuterSingularArticle(r reader, i int) bool {
	return r.node(i).Printed && r.node(i).UPOS == "DET" && r.feature(i, "Gender") == "Neut" && r.feature(i, "Number") == "Sing"
}

var articleCaseSubtypes = map[string]SubtypeID{
	"Nom": SubtypeNominative, "Gen": SubtypeGenitive, "Dat": SubtypeDative, "Acc": SubtypeAccusative,
}

func detectArticularInfinitive(r reader, n int) (SubtypeID, bool) {
	if !r.node(n).Printed || !r.verbForm(n, "Inf") {
		return "", false
	}
	for _, child := range r.t.Children(n) {
		if isNeuterSingularArticle(r, child) {
			return articleCaseSubtypes[r.feature(child, "Case")], true
		}
	}
	return "", false
}

func detectResult(r reader, n int) (SubtypeID, bool) {
	if !r.isConjunction(n, "ὥστε") {
		return "", false
	}
	clauses := r.clauseChildren(n, true)
	if len(clauses) == 0 {
		return "", false
	}
	for _, clause := range clauses {
		if r.verbForm(clause, "Part") {
			return "", false
		}
	}
	switch first := clauses[0]; {
	case r.verbForm(first, "Inf"):
		return SubtypeNatural, true
	case r.verb(first, "Ind", "Sub", "Opt", "Imp"):
		return SubtypeActual, true
	}
	return "", true
}

func detectPurpose(r reader, n int) (SubtypeID, bool) {
	if !r.isConjunction(n, "ἵνα") || len(r.clauseChildren(n, false, "Sub", "Opt")) == 0 {
		return "", false
	}
	return "", true
}

// genitiveSubjects are the printed genitive subjects of a participle, directly
// or through the coordination group it belongs to.
func genitiveSubjects(r reader, n int) []int {
	subjects := r.children(n, syntax.CategorySubject)
	seen := map[int]bool{n: true}
	for head := r.node(n).Head; head != syntax.Root && r.category(head) == syntax.CategoryCoordination && r.node(head).Printed && !seen[head]; head = r.node(head).Head {
		seen[head] = true
		subjects = append(subjects, r.children(head, syntax.CategorySubject)...)
	}
	var out []int
	for _, subject := range subjects {
		if r.node(subject).Printed && r.feature(subject, "Case") == "Gen" {
			out = append(out, subject)
		}
	}
	return out
}

func detectGenitiveAbsolute(r reader, n int) (SubtypeID, bool) {
	if !r.node(n).Printed || r.category(n) != syntax.CategoryAdverbial ||
		!r.verbForm(n, "Part") || r.feature(n, "Case") != "Gen" || len(genitiveSubjects(r, n)) == 0 {
		return "", false
	}
	head, ok := r.head(n)
	if !ok || (head != syntax.Root && perceivingVerbs[r.lemma(head)]) {
		return "", false
	}
	return "", true
}

func detectIndirectStatement(r reader, n int) (SubtypeID, bool) {
	node := r.node(n)
	if !node.Printed {
		return "", false
	}
	if r.verbForm(n, "Inf") && (node.Category == syntax.CategoryObject || node.Category == syntax.CategorySubject) {
		return infinitiveStatement(r, n)
	}
	return hotiHosStatement(r, n)
}

// infinitiveStatement is an infinitive OBJ or SBJ of a saying or thinking
// verb, with every subject (or none) in the accusative.
func infinitiveStatement(r reader, n int) (SubtypeID, bool) {
	head, ok := r.head(n)
	if !ok || head == syntax.Root || !infinitiveVerbs[r.lemma(head)] {
		return "", false
	}
	if futureOnlyVerbs[r.lemma(head)] && (r.feature(n, "Tense") != "Fut" || r.feature(n, "Aspect") == "Perf") {
		return "", false
	}
	// A subject behind an unprinted coordinator cannot be checked.
	for _, child := range r.t.Children(n) {
		if r.category(child) == syntax.CategoryCoordination && !r.node(child).Printed {
			return "", false
		}
	}
	for _, subject := range r.children(n, syntax.CategorySubject) {
		if r.feature(subject, "Case") != "Acc" {
			return "", false
		}
	}
	return SubtypeInfinitive, true
}

// hotiHosStatement is ὅτι or ὡς heading a finite OBJ or SBJ verb, under a verb
// of saying, thinking, knowing or perceiving.
func hotiHosStatement(r reader, n int) (SubtypeID, bool) {
	if !r.isConjunction(n, "ὅτι", "ὡς") {
		return "", false
	}
	head, ok := r.head(n)
	if !ok || head == syntax.Root || !hotiHosVerbs[r.lemma(head)] {
		return "", false
	}
	for _, child := range r.children(n, syntax.CategoryObject, syntax.CategorySubject) {
		if r.verb(child, "Ind", "Sub", "Opt") {
			return SubtypeHotiHos, true
		}
	}
	return "", false
}
