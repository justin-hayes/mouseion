package fixtures

import "github.com/justin-hayes/mouseion/internal/classical"

// setClassicalDefinitions installs the release the fixture Add resolves units
// from, so the shared store contract can seed its own small catalog.
func (s *Store) setClassicalDefinitions(definitions []classical.Definition) {
	byKey := make(map[string]classical.Definition, len(definitions))
	for _, definition := range definitions {
		byKey[definition.Key] = definition
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.classicalDefinitions = byKey
}
