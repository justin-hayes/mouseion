package domain

// JobLifecycleState is the one learner-facing lifecycle shared by every durable
// job on /jobs. Analysis, Classical preparation and Catalog sync describe
// themselves with these five values and no other state vocabulary.
type JobLifecycleState string

const (
	JobLifecycleQueued    JobLifecycleState = "queued"
	JobLifecycleRunning   JobLifecycleState = "running"
	JobLifecycleCompleted JobLifecycleState = "completed"
	JobLifecycleFailed    JobLifecycleState = "failed"
	JobLifecycleCancelled JobLifecycleState = "cancelled"
)

// JobLifecycleForRiverState maps a River job state onto the one lifecycle
// state. River's JobState is an open upstream enumeration, so an unrecognised
// value is queued. It also accepts the logical states analysis records for a
// run, whose names match the lifecycle except that a failed run is "failed"
// rather than River's "discarded".
func JobLifecycleForRiverState(state string) JobLifecycleState {
	switch state {
	case "completed":
		return JobLifecycleCompleted
	case "running":
		return JobLifecycleRunning
	case "cancelled":
		return JobLifecycleCancelled
	case "discarded", "failed":
		return JobLifecycleFailed
	case "available", "pending", "retryable", "scheduled", "queued":
		return JobLifecycleQueued
	default:
		return JobLifecycleQueued
	}
}
