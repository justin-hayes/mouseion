package epub

import "github.com/justin-hayes/mouseion/internal/domain"

const ReviewedScopeSchemaVersion = domain.EPUBReviewedScopeSchemaVersion

var ErrReviewedScopeUnavailable = domain.ErrEPUBReviewedScopeUnavailable

const (
	ScopeSelectionRecommended = domain.EPUBScopeSelectionRecommended
	ScopeSelectionOverridden  = domain.EPUBScopeSelectionOverridden
)

type ScopeSelectionMode = domain.EPUBScopeSelectionMode
type ReviewedScopeSnapshot = domain.EPUBReviewedScopeSnapshot
type UnitSnapshotIdentity = domain.EPUBUnitSnapshotIdentity
type SelectedUnitReference = domain.EPUBSelectedUnitReference
type ScopeSourceSnapshot = domain.EPUBScopeSourceSnapshot
