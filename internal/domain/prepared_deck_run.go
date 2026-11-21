package domain

import "time"

type PreparedDeckRunState string

type PreparedDeckExecutionMode string

const (
	PreparedDeckExecutionStandard PreparedDeckExecutionMode = "standard"
	PreparedDeckExecutionBatch    PreparedDeckExecutionMode = "batch"
)

const (
	PreparedDeckRunTranslating PreparedDeckRunState = "translating"
	PreparedDeckRunFinalizing  PreparedDeckRunState = "finalizing"
	PreparedDeckRunCompleted   PreparedDeckRunState = "completed"
	PreparedDeckRunFailed      PreparedDeckRunState = "failed"
	PreparedDeckRunCancelled   PreparedDeckRunState = "cancelled"
)

func (s PreparedDeckRunState) CanTransitionTo(next PreparedDeckRunState) bool {
	if s == next {
		return true
	}
	switch s {
	case PreparedDeckRunTranslating:
		return next == PreparedDeckRunFinalizing || next == PreparedDeckRunFailed || next == PreparedDeckRunCancelled
	case PreparedDeckRunFinalizing:
		return next == PreparedDeckRunCompleted || next == PreparedDeckRunFailed || next == PreparedDeckRunCancelled
	case PreparedDeckRunCompleted, PreparedDeckRunFailed, PreparedDeckRunCancelled:
		return false
	default: // Invalid persisted states cannot transition.
		return false
	}
}

type PreparedDeckTranslationState string

const (
	PreparedDeckTranslationPending   PreparedDeckTranslationState = "pending"
	PreparedDeckTranslationRunning   PreparedDeckTranslationState = "running"
	PreparedDeckTranslationCompleted PreparedDeckTranslationState = "completed"
	PreparedDeckTranslationFailed    PreparedDeckTranslationState = "failed"
	PreparedDeckTranslationCancelled PreparedDeckTranslationState = "cancelled"
)

func (s PreparedDeckTranslationState) CanTransitionTo(next PreparedDeckTranslationState) bool {
	if s == next {
		return true
	}
	switch s {
	case PreparedDeckTranslationPending:
		return next == PreparedDeckTranslationRunning || next == PreparedDeckTranslationCompleted || next == PreparedDeckTranslationFailed || next == PreparedDeckTranslationCancelled
	case PreparedDeckTranslationRunning:
		return next == PreparedDeckTranslationCompleted || next == PreparedDeckTranslationFailed || next == PreparedDeckTranslationCancelled
	case PreparedDeckTranslationCompleted, PreparedDeckTranslationFailed, PreparedDeckTranslationCancelled:
		return false
	default: // Invalid persisted states cannot transition.
		return false
	}
}

type PreparedDeckRun struct {
	ID, OwnerID, PreparationID                        string
	RunNumber                                         int
	State                                             PreparedDeckRunState
	ExecutionMode                                     PreparedDeckExecutionMode
	TargetLanguage                                    string
	TranslationState                                  PreparedDeckTranslationState
	ExternalTranslationConsent                        bool
	ExternalTranslationConfigured                     bool
	ContextMode, Provider, ProviderVersion            string
	Endpoint, Model                                   string
	ManifestSchemaVersion, RenderInputVersion         int
	PresentationVersion, RetryPolicyVersion           int
	MaxProviderAttempts, MaxBatchGenerations          int
	BatchMaxRequests                                  int
	BatchMaxBytes                                     int64
	CandidateCount, CompletedCount, FailedCount       int
	FinalizationDispatchGeneration                    int
	FinalizationDispatchCount                         int
	FinalizationJobID                                 int64
	FinalizationClaimToken                            string
	ErrorClass, ErrorCode                             string
	CreatedAt, UpdatedAt                              time.Time
	FinalizationClaimedAt, FinalizationLeaseExpiresAt *time.Time
	TranslationCompletedAt, CompletedAt               *time.Time
}

type PreparedDeckOutcomeState string

const (
	PreparedDeckOutcomePending   PreparedDeckOutcomeState = "pending"
	PreparedDeckOutcomeRunning   PreparedDeckOutcomeState = "running"
	PreparedDeckOutcomeCompleted PreparedDeckOutcomeState = "completed"
	PreparedDeckOutcomeFailed    PreparedDeckOutcomeState = "failed"
	PreparedDeckOutcomeCancelled PreparedDeckOutcomeState = "cancelled"
)

// Terminal reports whether the outcome can no longer change.
func (s PreparedDeckOutcomeState) Terminal() bool {
	switch s {
	case PreparedDeckOutcomeCompleted, PreparedDeckOutcomeFailed, PreparedDeckOutcomeCancelled:
		return true
	default:
		return false
	}
}

// Active reports whether the outcome is still awaiting or running a
// translation. Only active outcomes may be settled by a new result.
func (s PreparedDeckOutcomeState) Active() bool {
	return s == PreparedDeckOutcomePending || s == PreparedDeckOutcomeRunning
}

// TranslationSettled reports the outcomes a standard translation result may
// set: a completed card or a recorded failure.
func (s PreparedDeckOutcomeState) TranslationSettled() bool {
	return s == PreparedDeckOutcomeCompleted || s == PreparedDeckOutcomeFailed
}

// BatchSettable reports the outcomes a Batch result may set. A retryable item
// stays pending for the next generation.
func (s PreparedDeckOutcomeState) BatchSettable() bool {
	switch s {
	case PreparedDeckOutcomeCompleted, PreparedDeckOutcomePending, PreparedDeckOutcomeFailed:
		return true
	default:
		return false
	}
}

// BatchSubmittable reports the outcomes that may remain in an uploaded Batch
// chunk: pending work, or an exact cache hit already completed locally.
func (s PreparedDeckOutcomeState) BatchSubmittable() bool {
	return s == PreparedDeckOutcomePending || s == PreparedDeckOutcomeCompleted
}

type PreparedDeckTranslationOutcome struct {
	OwnerID, PreparationID, RunID         string
	Ordinal                               int
	State                                 PreparedDeckOutcomeState
	DispatchCount, ProviderAttemptCount   int
	MaxProviderAttempts                   int
	NextAttemptAt                         time.Time
	DispatchGeneration                    int
	RiverJobID                            int64
	ClaimToken                            string
	ClaimedAt, LeaseExpiresAt, TerminalAt *time.Time
	ErrorClass, ErrorCode                 string
	OmissionReason                        string
	CacheHitCount, ProviderCallCount      int
	CacheLatency, ProviderLatency         time.Duration
	UpdatedAt                             time.Time
}

// IsDeckPreparationFailureOmission reports whether a failed translation outcome
// may be left out instead of failing the deck. An idiom card is optional, so any
// failure omits it; an idiom has no vocabulary identity, so it is never a
// vocabulary omission (ADR 0114). A lemma card is omitted only for the four
// bounded provider response-validation failures ADR 0100 allows; any other
// failure fails the deck, and widening the four codes is a deliberate ADR
// change.
func IsDeckPreparationFailureOmission(isIdiom bool, errorClass, errorCode string) bool {
	if isIdiom {
		return true
	}
	if errorClass != "validation" {
		return false
	}
	switch errorCode {
	case "invalid_response", "invalid_translation_response", "malformed_response", "response_too_large":
		return true
	default:
		return false
	}
}

type PreparedDeckBatchChunkState string

const (
	PreparedDeckBatchPending     PreparedDeckBatchChunkState = "pending"
	PreparedDeckBatchSubmitting  PreparedDeckBatchChunkState = "submitting"
	PreparedDeckBatchSubmitted   PreparedDeckBatchChunkState = "submitted"
	PreparedDeckBatchPolling     PreparedDeckBatchChunkState = "polling"
	PreparedDeckBatchReconciling PreparedDeckBatchChunkState = "reconciling"
	PreparedDeckBatchCompleted   PreparedDeckBatchChunkState = "completed"
	PreparedDeckBatchFailed      PreparedDeckBatchChunkState = "failed"
	PreparedDeckBatchCancelled   PreparedDeckBatchChunkState = "cancelled"
	PreparedDeckBatchAmbiguous   PreparedDeckBatchChunkState = "ambiguous"
)

// Terminal reports whether the chunk has settled for work scheduling: it is
// never polled, submitted or reconciled again. Bulk cancellation is the only
// write that may still move an ambiguous chunk.
func (s PreparedDeckBatchChunkState) Terminal() bool {
	switch s {
	case PreparedDeckBatchCompleted, PreparedDeckBatchFailed, PreparedDeckBatchCancelled, PreparedDeckBatchAmbiguous:
		return true
	default:
		return false
	}
}

// AwaitingSubmission reports the states from which a submission claim may
// start work.
func (s PreparedDeckBatchChunkState) AwaitingSubmission() bool {
	return s == PreparedDeckBatchPending || s == PreparedDeckBatchSubmitting
}

// AwaitingPoll reports the states where a submitted provider Batch is waiting
// for its next reconciliation.
func (s PreparedDeckBatchChunkState) AwaitingPoll() bool {
	return s == PreparedDeckBatchSubmitted || s == PreparedDeckBatchPolling
}

// SubmissionClosed reports whether the chunk has left the submission phase, so
// a submission attempt must not start. An unknown persisted state stays open so
// the submission claim remains the authority that rejects it.
func (s PreparedDeckBatchChunkState) SubmissionClosed() bool {
	return s.Terminal() || s.AwaitingPoll() || s == PreparedDeckBatchReconciling
}

// CleanupEligible reports the settled states whose provider files are cleaned
// up after reconciliation or cancellation.
func (s PreparedDeckBatchChunkState) CleanupEligible() bool {
	return s == PreparedDeckBatchCompleted || s == PreparedDeckBatchCancelled
}

// SubmissionSettled reports the states a submission attempt may settle to
// without reaching the provider: completed cache hits, a pre-network failure or
// an ambiguous provider acknowledgment.
func (s PreparedDeckBatchChunkState) SubmissionSettled() bool {
	switch s {
	case PreparedDeckBatchCompleted, PreparedDeckBatchFailed, PreparedDeckBatchAmbiguous:
		return true
	default:
		return false
	}
}

// ReconciliationUpdateAllowed reports the states a reconciliation attempt may
// set: another poll, completion or failure.
func (s PreparedDeckBatchChunkState) ReconciliationUpdateAllowed() bool {
	switch s {
	case PreparedDeckBatchCompleted, PreparedDeckBatchFailed, PreparedDeckBatchPolling:
		return true
	default:
		return false
	}
}

// FailureReported reports the chunks that carry a run-level failure class.
func (s PreparedDeckBatchChunkState) FailureReported() bool {
	return s == PreparedDeckBatchFailed || s == PreparedDeckBatchAmbiguous
}

// PreparedDeckBatchFileCleanupState is one provider file's durable cleanup
// state. The values match the database check constraint.
type PreparedDeckBatchFileCleanupState string

const (
	PreparedDeckBatchCleanupPending   PreparedDeckBatchFileCleanupState = "pending"
	PreparedDeckBatchCleanupDeleted   PreparedDeckBatchFileCleanupState = "deleted"
	PreparedDeckBatchCleanupFailed    PreparedDeckBatchFileCleanupState = "failed"
	PreparedDeckBatchCleanupNotNeeded PreparedDeckBatchFileCleanupState = "not_needed"
)

// Valid reports whether the state is one the database accepts.
func (s PreparedDeckBatchFileCleanupState) Valid() bool {
	switch s {
	case PreparedDeckBatchCleanupPending, PreparedDeckBatchCleanupDeleted, PreparedDeckBatchCleanupFailed, PreparedDeckBatchCleanupNotNeeded:
		return true
	default:
		return false
	}
}

// Settled reports whether the file has reached a final cleanup state. Pending
// and failed files remain retryable.
func (s PreparedDeckBatchFileCleanupState) Settled() bool {
	return s == PreparedDeckBatchCleanupDeleted || s == PreparedDeckBatchCleanupNotNeeded
}

type PreparedDeckBatchChunk struct {
	ID, OwnerID, PreparationID, RunID                                             string
	ChunkIndex, Generation                                                        int
	State                                                                         PreparedDeckBatchChunkState
	ProviderStatus, Model, Endpoint, SplitReason                                  string
	FirstOrdinal, LastOrdinal                                                     int
	InputDigest                                                                   string
	RequestCount                                                                  int
	InputBytes, EstimatedPromptTokens                                             int64
	CompletedCount, FailedCount, ExpiredCount                                     int
	InputFileID, BatchID, OutputFileID, ErrorFileID                               string
	InputFileCleanupState, OutputFileCleanupState, ErrorFileCleanupState          string
	InputFileCleanupAttempts, OutputFileCleanupAttempts, ErrorFileCleanupAttempts int
	CleanupErrorClass, CleanupErrorCode                                           string
	SubmissionJobID, ReconciliationJobID                                          int64
	SubmissionGeneration, ReconciliationGeneration                                int
	SubmissionClaimToken, ReconciliationClaimToken                                string
	ErrorClass, ErrorCode                                                         string
	InputTokens, OutputTokens, TotalTokens                                        int64
	CreatedAt, UpdatedAt                                                          time.Time
	SubmissionClaimedAt, SubmissionLeaseExpiresAt                                 *time.Time
	ReconciliationClaimedAt, ReconciliationLeaseExpiresAt                         *time.Time
	SubmittedAt, LastPolledAt, ProviderCompletedAt, ReconciledAt                  *time.Time
	CleanupClaimToken                                                             string
	CleanupClaimedAt, CleanupLeaseExpiresAt, CleanupCompletedAt                   *time.Time
	Ordinals                                                                      []int
}

type PreparedDeckRunProgress struct {
	CandidateCount, PendingCount, RunningCount                                           int
	CompletedCount, FailedCount, CancelledCount                                          int
	ManifestOmissions                                                                    int
	RetryingCount                                                                        int
	CardsWithEnglish, CardsWithContextualSentenceTranslations                            int
	BatchChunkCount, BatchSubmittedChunks, BatchPollingChunks, BatchReconcilingChunks    int
	BatchCompletedChunks, BatchFailedChunks, BatchCancelledChunks                        int
	BatchRequestCount, BatchCompletedRequests, BatchFailedRequests, BatchExpiredRequests int
	BatchInputTokens, BatchOutputTokens                                                  int64
	BatchAge                                                                             time.Duration
	Phase                                                                                string
	FailureClass                                                                         string
}

// PreparedDeckRecoveryWork is a content-free projection used by later
// coordinator and reconciliation jobs after process restarts.
type PreparedDeckRecoveryWork struct {
	OwnerID, PreparationID, RunID, ChunkID string
	Ordinal, Generation                    int
	Kind                                   string
	LeaseExpired                           bool
}

// PreparedDeckBatchClaimKind identifies the independently fenced parts of a
// Batch lifecycle. Submission and reconciliation must not share a lease.
type PreparedDeckBatchClaimKind string

const (
	PreparedDeckBatchSubmissionClaim     PreparedDeckBatchClaimKind = "submission"
	PreparedDeckBatchReconciliationClaim PreparedDeckBatchClaimKind = "reconciliation"
)
