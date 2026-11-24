package constructions

import (
	"fmt"
	"strconv"
	"strings"
)

// GrammarReference is a printed grammar a Concept note cites (ADR 0124).
type GrammarReference struct {
	ID          string
	Language    string
	Label       string
	URLTemplate string
}

// SmythGrammar is the Greek grammar reference: Smyth, A Greek Grammar for
// Colleges (1920), on Perseus Hopper. Swapping the host is one edit here.
var SmythGrammar = GrammarReference{
	ID:          "smyth-1920",
	Language:    "grc",
	Label:       "Smyth 1920",
	URLTemplate: "https://www.perseus.tufts.edu/hopper/text?doc=Perseus:text:1999.04.0007:smythp={§}",
}

// Citation is a section or range of sections of the grammar reference. Last is
// zero or equal to First for a single section. Suffix is a sub-section letter
// such as "a".
type Citation struct {
	First, Last int
	Suffix      string
}

// CitationText renders "Smyth 1920 §N", "Smyth 1920 §§N–M" or, with a suffix,
// "Smyth 1920 §N a".
func CitationText(c Citation) string {
	var b strings.Builder
	b.WriteString(SmythGrammar.Label)
	b.WriteByte(' ')
	if c.Last > c.First {
		fmt.Fprintf(&b, "§§%d–%d", c.First, c.Last)
	} else {
		fmt.Fprintf(&b, "§%d", c.First)
	}
	if c.Suffix != "" {
		b.WriteByte(' ')
		b.WriteString(c.Suffix)
	}
	return b.String()
}

// CitationURL links a citation to the grammar reference at its first section.
func CitationURL(c Citation) string {
	return strings.ReplaceAll(SmythGrammar.URLTemplate, "{§}", strconv.Itoa(c.First))
}

// ConceptNote is Mouseion's own learner-facing note on a construction. It is
// reviewed data, looked up at render, and never evidence (ADR 0124). Its
// Definition describes the concept in Mouseion's words, not the detection rule,
// and carries no example.
type ConceptNote struct {
	Name         string
	Definition   string
	Citation     Citation
	ReviewTicket int
}

// ReviewTicket is the issue on which these notes, the subtype labels and the
// verb lists are reviewed.
const ReviewTicket = 2621

// Notes holds one reviewed note per construction id the detectors can emit.
var Notes = map[ID]ConceptNote{
	Conditional: {
		Name:         "Conditional clause",
		Definition:   "A clause that states a condition. What the rest of the sentence says depends on that condition holding.",
		Citation:     Citation{First: 2280, Last: 2368},
		ReviewTicket: ReviewTicket,
	},
	ArticularInfinitive: {
		Name:         "Articular infinitive",
		Definition:   "An infinitive with a neuter article in front of it, so that it works like a noun and names an action as a thing.",
		Citation:     Citation{First: 2025, Last: 2038},
		ReviewTicket: ReviewTicket,
	},
	Result: {
		Name:         "Result clause",
		Definition:   "A clause that tells what outcome follows from the action of the main clause.",
		Citation:     Citation{First: 2249, Last: 2278},
		ReviewTicket: ReviewTicket,
	},
	Purpose: {
		Name:         "Purpose clause",
		Definition:   "A clause that tells what the action of the main clause is meant to achieve.",
		Citation:     Citation{First: 2193, Last: 2206},
		ReviewTicket: ReviewTicket,
	},
	GenitiveAbsolute: {
		Name:         "Genitive absolute",
		Definition:   "A participle and its own subject, both in the genitive, that stand apart from the main clause and give the circumstances of its action.",
		Citation:     Citation{First: 2070, Last: 2075},
		ReviewTicket: ReviewTicket,
	},
	IndirectStatement: {
		Name:         "Indirect statement",
		Definition:   "A report of what someone says, thinks or knows, built into the sentence instead of quoted directly.",
		Citation:     Citation{First: 2016, Last: 2024},
		ReviewTicket: ReviewTicket,
	},
}

// subtypeLabels are the learner-facing labels of the subtypes that ship.
var subtypeLabels = map[labelKey]string{
	{Result, SubtypeNatural}:               "natural result",
	{IndirectStatement, SubtypeInfinitive}: "infinitive",
	{ArticularInfinitive, SubtypeGenitive}: "genitive",
	{ArticularInfinitive, SubtypeDative}:   "dative",
}

// SubtypeLabel returns the learner-facing label of a shipped subtype.
func SubtypeLabel(construction ID, subtype SubtypeID) (string, bool) {
	label, ok := subtypeLabels[labelKey{construction, subtype}]
	return label, ok
}
