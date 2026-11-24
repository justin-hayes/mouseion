package classical

// changeNotes holds the optional Mouseion-authored change note of each
// interpretation version (ADR 0126), keyed by InterpretationVersion. A note
// tells the learner what an Analysis update to that version adds. It is looked
// up when the confirmation renders and is not evidence. A version with no entry
// has no note.
var changeNotes = map[string]string{
	"oga-agdt-syntax/1": "Adds sentence constructions in Sentence study.",
}

// ChangeNote returns the change note for an interpretation version and reports
// whether the version has one.
func ChangeNote(interpretationVersion string) (string, bool) {
	note, ok := changeNotes[interpretationVersion]
	return note, ok
}
