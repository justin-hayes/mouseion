package epub

import "github.com/justin-hayes/mouseion/internal/domain"

const (
	ClassificationSchemaVersion = domain.EPUBClassificationSchemaVersion
	ConfidenceMaximum           = domain.EPUBConfidenceMaximum
	ConfidenceLowMaximum        = domain.EPUBConfidenceLowMaximum
	ConfidenceHighMinimum       = domain.EPUBConfidenceHighMinimum

	CategoryFrontMatter = domain.EPUBCategoryFrontMatter
	CategoryMainMatter  = domain.EPUBCategoryMainMatter
	CategoryBackMatter  = domain.EPUBCategoryBackMatter
	CategoryUnknown     = domain.EPUBCategoryUnknown
)

type UnitCategory = domain.EPUBUnitCategory
type UnitClassification = domain.EPUBUnitClassification
type ClassifierIdentity = domain.EPUBClassifierIdentity
type SourceUnitSnapshotIdentity = domain.EPUBSourceUnitSnapshotIdentity
type ClassificationReason = domain.EPUBClassificationReason
