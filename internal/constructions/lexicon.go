package constructions

// The verb lists are Mouseion-authored, compiled, reviewed data (spec,
// "Authored lexical data"), keyed by source lemma and part of the detector
// version. Homograph numbers are not part of a lemma here.

// infinitiveVerbs are the governing verbs of an infinitive indirect statement.
var infinitiveVerbs = setOf(
	"φημί", "φάσκω", "λέγω", "εἶπον", "ἐρῶ", "ἀγγέλλω", "ἀπαγγέλλω", "ἐξαγγέλλω",
	"ἀποκρίνω", "ὁμολογέω", "ἀρνέομαι", "ἀπαρνέομαι", "ὑπισχνέομαι", "ὄμνυμι",
	"νομίζω", "οἴομαι", "ἡγέομαι", "ὑπολαμβάνω", "ἐλπίζω", "ὑποπτεύω", "πιστεύω",
	"διηγέομαι", "ἐννοέω",
)

// futureOnlyVerbs govern an indirect statement only with a future infinitive.
var futureOnlyVerbs = setOf("ὑπισχνέομαι", "ὄμνυμι")

// hotiHosVerbs are the governing verbs of an ὅτι/ὡς indirect statement: the
// infinitive list plus δοκέω and further verbs of knowing and perceiving.
var hotiHosVerbs = merge(infinitiveVerbs, setOf(
	"δοκέω", "οἶδα", "ἐπίσταμαι", "μανθάνω", "ὁράω", "ἀκούω", "μιμνῄσκω", "μιμνήσκω",
	"ἀναμιμνῄσκω", "ἀναμιμνήσκω", "πείθω",
))

// perceivingVerbs exclude a participle from the genitive absolute.
var perceivingVerbs = setOf("ἀκούω", "ἀκροάομαι", "αἰσθάνομαι", "πυνθάνομαι")

// likeLemmas mark "as if": a conditional after them is not a conditional.
var likeLemmas = setOf("ὡς", "ὥσπερ")

func setOf(lemmas ...string) map[string]bool {
	out := make(map[string]bool, len(lemmas))
	for _, lemma := range lemmas {
		out[lemma] = true
	}
	return out
}

func merge(sets ...map[string]bool) map[string]bool {
	out := map[string]bool{}
	for _, set := range sets {
		for lemma := range set {
			out[lemma] = true
		}
	}
	return out
}
