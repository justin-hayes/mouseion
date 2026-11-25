package persistence

import "github.com/justin-hayes/mouseion/internal/classical"

// A Reading unit's classical language lives in the manifest's Definition, not in
// the database (ADR 0132 adds no migration). Reconciliation records each
// definition's language for the life of the process, so Add, "I've read this"
// and preparation set the Book's language from the Definition they came from.

// rememberClassicalLanguages records the language of every reconciled
// definition.
func (s *PostgresStore) rememberClassicalLanguages(definitions []classical.Definition) {
	for _, definition := range definitions {
		s.classicalLanguages.Store(definition.Key, definition.Language)
	}
}

// classicalLanguage is the classical language of a Reading unit. A unit that
// was not reconciled by this process is Ancient Greek, the language every unit
// had before Latin.
func (s *PostgresStore) classicalLanguage(readingUnitKey string) string {
	if language, ok := s.classicalLanguages.Load(readingUnitKey); ok {
		if value, ok := language.(string); ok && classical.IsLanguage(value) {
			return value
		}
	}
	return classical.Language
}

// classicalDisplayName is the display name a unit's language is registered as a study
// language under.
func (s *PostgresStore) classicalDisplayName(language string) string {
	if name := classical.DisplayName(language); name != "" {
		return name
	}
	return classical.LanguageDisplayName
}
