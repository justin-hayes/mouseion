// Package sourcecheck runs extraction-only checks that tell an existing EPUB
// source whether the configured extraction contract or markup convention set
// would change its source structure. A check never selects a snapshot, runs
// NLP, publishes analysis or rewrites frozen evidence.
package sourcecheck

import (
	"errors"
	"fmt"
	"strings"

	"github.com/justin-hayes/mouseion/internal/domain"
)

const (
	// TargetContractVersion is the extraction contract that structure checks
	// target. It follows the configured v2 writer switch: while the writer
	// remains v1, no check is enqueued or run.
	TargetContractVersion = domain.ExtractedUnitsSchemaVersion

	// Queue keeps extraction-only checks off the analysis and catalogue queues.
	Queue = "source_checks"

	// maxAttempts bounds River's automatic retries. A discarded job leaves no
	// successful check row, so an explicit retry can enqueue it again.
	maxAttempts = 5

	// startupScanPageSize bounds each candidate query. A startup scan walks all
	// eligible exact keys once, one bounded page at a time.
	startupScanPageSize = 100
)

// Target names the extraction contract and structure convention set a check
// compares against.
type Target struct {
	ContractVersion int
	ConventionSet   string
}

// ConfiguredTarget is the target the application currently checks. Its contract
// version is the single switch that turns checks on with the v2 writer.
func ConfiguredTarget() Target {
	return Target{ContractVersion: TargetContractVersion, ConventionSet: domain.StructureConventionSet}
}

// Active reports whether a target asks for a newer extraction contract. An
// inactive target is inert: the current v1 snapshot already satisfies it.
func (t Target) Active() bool {
	return t.ContractVersion >= domain.ExtractedUnitsSchemaVersionV2
}

// Validate rejects a target this build cannot extract. Supporting a later
// convention set means teaching the extractor that set first; checks never
// pretend to have compared a contract they cannot produce.
func (t Target) Validate() error {
	if !t.Active() {
		return fmt.Errorf("sourcecheck: target contract %d is not newer than v1", t.ContractVersion)
	}
	if t.ContractVersion != domain.ExtractedUnitsSchemaVersionV2 {
		return fmt.Errorf("sourcecheck: unsupported target contract %d", t.ContractVersion)
	}
	if strings.TrimSpace(t.ConventionSet) == "" {
		return errors.New("sourcecheck: target convention set is required")
	}
	if t.ConventionSet != domain.StructureConventionSet {
		return fmt.Errorf("sourcecheck: unsupported structure convention set %q", t.ConventionSet)
	}
	return nil
}

// needsCheck reports whether an exact current snapshot is older or different
// enough from the target to deserve an extraction check.
func needsCheck(current domain.ExtractedSnapshot, target Target) bool {
	if current.Units.SchemaVersion < target.ContractVersion {
		return true
	}
	return current.Units.SchemaVersion == target.ContractVersion &&
		current.Structure.ConventionSet != target.ConventionSet
}
