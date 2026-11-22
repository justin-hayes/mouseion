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

// JobLifecycleForRiverState maps a job state onto the one lifecycle state.
// River's JobState is an open upstream enumeration, so an unrecognised value is
// queued. It also accepts the logical "failed" state the jobs tables record
// where River uses "discarded"; every other value matches a River state.
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

// Active reports whether the work is queued or running.
func (s JobLifecycleState) Active() bool {
	return s == JobLifecycleQueued || s == JobLifecycleRunning
}

// Retryable reports whether a settled job can be retried.
func (s JobLifecycleState) Retryable() bool {
	return s == JobLifecycleFailed || s == JobLifecycleCancelled
}
