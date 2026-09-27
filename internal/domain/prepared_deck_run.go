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

func (s PreparedDeckOutcomeState) CanTransitionTo(next PreparedDeckOutcomeState) bool {
	if s == next {
		return true
	}
	switch s {
	case PreparedDeckOutcomePending:
		return next == PreparedDeckOutcomeRunning || next == PreparedDeckOutcomeCancelled
	case PreparedDeckOutcomeRunning:
		return next == PreparedDeckOutcomePending || next == PreparedDeckOutcomeCompleted || next == PreparedDeckOutcomeFailed || next == PreparedDeckOutcomeCancelled
	case PreparedDeckOutcomeCompleted, PreparedDeckOutcomeFailed, PreparedDeckOutcomeCancelled:
		return false
	default: // Invalid persisted states cannot transition.
		return false
	}
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

func (s PreparedDeckBatchChunkState) CanTransitionTo(next PreparedDeckBatchChunkState) bool {
	if s == next {
		return true
	}
	switch s {
	case PreparedDeckBatchPending:
		return next == PreparedDeckBatchSubmitting || next == PreparedDeckBatchFailed || next == PreparedDeckBatchCancelled
	case PreparedDeckBatchSubmitting:
		return next == PreparedDeckBatchPending || next == PreparedDeckBatchSubmitted || next == PreparedDeckBatchAmbiguous || next == PreparedDeckBatchFailed || next == PreparedDeckBatchCancelled
	case PreparedDeckBatchSubmitted, PreparedDeckBatchPolling:
		return next == PreparedDeckBatchPolling || next == PreparedDeckBatchReconciling || next == PreparedDeckBatchFailed || next == PreparedDeckBatchCancelled
	case PreparedDeckBatchReconciling:
		return next == PreparedDeckBatchPolling || next == PreparedDeckBatchCompleted || next == PreparedDeckBatchFailed || next == PreparedDeckBatchCancelled
	case PreparedDeckBatchCompleted, PreparedDeckBatchFailed, PreparedDeckBatchCancelled, PreparedDeckBatchAmbiguous:
		return false
	default: // Invalid persisted states cannot transition.
		return false
	}
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
