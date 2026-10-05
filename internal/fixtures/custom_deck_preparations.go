package fixtures

import (
	"context"
	"strings"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

const customDeckPreparationPrefix = "fixture-custom-deck-preparation-"

// CustomDeckPreparations supplies a durable-looking queued response for browser
// acceptance of the native Custom deck preparation form.
type CustomDeckPreparations struct{}

func (CustomDeckPreparations) Submit(_ context.Context, owner, deckID, _, _ string) (domain.CustomDeckPreparation, error) {
	preparationID := customDeckPreparationPrefix + deckID
	return domain.CustomDeckPreparation{
		ID: preparationID, OwnerID: owner, DeckID: deckID, DeckName: "Fixture practice",
		Language: "de", State: "queued", SelectedIdentities: 1,
	}, nil
}

func (CustomDeckPreparations) EvidenceFingerprint(context.Context, string, string) (string, error) {
	return "fixture-evidence", nil
}

func (CustomDeckPreparations) Get(_ context.Context, _, id string) (domain.CustomDeckPreparation, error) {
	if !strings.HasPrefix(id, customDeckPreparationPrefix) {
		return domain.CustomDeckPreparation{}, persistence.ErrNotFound
	}
	deckID := strings.TrimPrefix(id, customDeckPreparationPrefix)
	return domain.CustomDeckPreparation{ID: id, DeckID: deckID, Language: "de", State: "queued", SelectedIdentities: 1}, nil
}

func (CustomDeckPreparations) Latest(context.Context, string, string) (domain.CustomDeckPreparation, error) {
	return domain.CustomDeckPreparation{}, persistence.ErrNotFound
}

func (CustomDeckPreparations) List(context.Context, string, string) ([]domain.CustomDeckPreparation, error) {
	return nil, nil
}

func (CustomDeckPreparations) LatestReady(context.Context, string, string) (domain.CustomDeckPreparation, error) {
	return domain.CustomDeckPreparation{}, persistence.ErrNotFound
}

func (f CustomDeckPreparations) Download(ctx context.Context, owner, id string) (domain.CustomDeckPreparation, error) {
	return f.Get(ctx, owner, id)
}

func (CustomDeckPreparations) Cancel(context.Context, string, string) error { return nil }
