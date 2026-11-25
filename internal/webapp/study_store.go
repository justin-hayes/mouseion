package webapp

import (
	"context"

	"github.com/justin-hayes/mouseion/internal/domain"
)

// StudyStore is the Study page's read of the learner's Study material. Both
// the PostgreSQL store and the fixtures store satisfy it (ADR 0088).
type StudyStore interface {
	CountStudyMaterial(ctx context.Context, owner, language string) (domain.StudyMaterialCounts, error)
}

// Study is the seam through which the page asks for Study material to be
// prepared ahead. It only enqueues durable work: a page request never calls a
// provider (ADR 0134 §8).
type Study interface {
	EnqueueStudyPreparation(ctx context.Context, owner, language string) error
}
