package epub

import "github.com/justin-hayes/mouseion/internal/domain"

const ReviewedScopeSchemaVersion = domain.EPUBReviewedScopeSchemaVersion

var ErrReviewedScopeUnavailable = domain.ErrEPUBReviewedScopeUnavailable

type ReviewedScopeSnapshot = domain.EPUBReviewedScopeSnapshot
type UnitSnapshotIdentity = domain.EPUBUnitSnapshotIdentity
type SelectedUnitReference = domain.EPUBSelectedUnitReference
type ScopeSourceSnapshot = domain.EPUBScopeSourceSnapshot
