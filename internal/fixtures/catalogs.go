package fixtures

// Contract status: illustrative. Canned state for browser scenarios; not held
// to internal/storecontract parity (ADR 0088).

import (
	"context"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/epub"
	"github.com/justin-hayes/mouseion/internal/opds"
)

func (s *Store) CreateOpdsConnection(_ context.Context, o string, c domain.OpdsConnection) (domain.OpdsConnection, error) {
	c.ID = "fixture-new-connection"
	c.OwnerID = o
	s.connections = append(s.connections, c)
	return c, nil
}

func (s *Store) GetOpdsConnection(_ context.Context, o, id string) (domain.OpdsConnection, error) {
	for _, c := range s.connections {
		if c.ID == id && c.OwnerID == o {
			return c, nil
		}
	}
	return domain.OpdsConnection{}, errNotFound
}

func (s *Store) ListOpdsConnections(context.Context, string) ([]domain.OpdsConnection, error) {
	return append([]domain.OpdsConnection(nil), s.connections...), nil
}

func (s *Store) ListCatalogueSyncStatuses(_ context.Context, owner string) ([]domain.CatalogueSyncStatus, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var result []domain.CatalogueSyncStatus
	for _, status := range s.syncStatuses {
		if status.OwnerID == owner {
			result = append(result, status)
		}
	}
	return result, nil
}

func (s *Store) SetCatalogueSyncStatus(_ context.Context, status domain.CatalogueSyncStatus) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if status.UpdatedAt.IsZero() {
		status.UpdatedAt = fixtureJourneyTime
	}
	for i := range s.syncStatuses {
		if s.syncStatuses[i].OwnerID == status.OwnerID && s.syncStatuses[i].ConnectionID == status.ConnectionID {
			s.syncStatuses[i] = status
			return nil
		}
	}
	s.syncStatuses = append(s.syncStatuses, status)
	return nil
}

func (s *Store) UpdateOpdsConnection(_ context.Context, _ string, c domain.OpdsConnection) (domain.OpdsConnection, error) {
	return c, nil
}

func (s *Store) DeleteOpdsConnection(context.Context, string, string) error { return nil }

func (s *Store) admitFixtureCatalogueLanguage(owner, connectionID string) {
	if connectionID != "fixture-browser-sync-connection" {
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, alias := range s.aliases {
		if alias.OwnerID != owner || alias.ConnectionID != connectionID {
			continue
		}
		for i := range s.myBooks {
			if s.myBooks[i].Book.OwnerID == owner && s.myBooks[i].Book.ID == alias.BookID && s.myBooks[i].Book.LanguageState == domain.LanguageUnknown {
				s.myBooks[i].Book.LanguageState = domain.LanguageChosen
				s.myBooks[i].Book.LanguageTag = fixtureCatalogueLanguage
			}
		}
	}
}

type OPDS struct{}

func (OPDS) AcquireForBook(context.Context, string, string, string, string, opds.Entry) (epub.ImportResult, error) {
	return epub.ImportResult{Source: domain.SourceMaterial{ID: "fixture-metadata-only", OwnerID: OwnerID, Language: "de", Title: "Metadata-only migration book"}}, nil
}
