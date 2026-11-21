package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDeckPreparationStateSets(t *testing.T) {
	tests := []struct {
		state               DeckPreparationState
		active, requeueable bool
	}{
		{DeckPreparationQueued, true, false},
		{DeckPreparationPreparing, true, false},
		{DeckPreparationReady, false, false},
		{DeckPreparationFailed, false, true},
		{DeckPreparationCancelled, false, true},
	}
	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			assert.Equal(t, tt.active, tt.state.Active(), "Active")
			assert.Equal(t, tt.requeueable, tt.state.Requeueable(), "Requeueable")
		})
	}
}

func TestPreparedDeckRunStateSets(t *testing.T) {
	tests := []struct {
		state                     PreparedDeckRunState
		active, finalizingOrLater bool
	}{
		{PreparedDeckRunTranslating, true, false},
		{PreparedDeckRunFinalizing, true, true},
		{PreparedDeckRunCompleted, false, true},
		{PreparedDeckRunFailed, false, false},
		{PreparedDeckRunCancelled, false, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			assert.Equal(t, tt.active, tt.state.Active(), "Active")
			assert.Equal(t, tt.finalizingOrLater, tt.state.FinalizingOrLater(), "FinalizingOrLater")
		})
	}
}

func TestPreparedDeckTranslationStateSets(t *testing.T) {
	tests := []struct {
		state  PreparedDeckTranslationState
		active bool
	}{
		{PreparedDeckTranslationPending, true},
		{PreparedDeckTranslationRunning, true},
		{PreparedDeckTranslationCompleted, false},
		{PreparedDeckTranslationFailed, false},
		{PreparedDeckTranslationCancelled, false},
	}
	for _, tt := range tests {
		t.Run(string(tt.state), func(t *testing.T) {
			assert.Equal(t, tt.active, tt.state.Active(), "Active")
		})
	}
}

func TestPreparedDeckOutcomeStateSets(t *testing.T) {
	tests := []struct {
		state                                                                 PreparedDeckOutcomeState
		terminal, active, translationSettled, batchSettable, batchSubmittable bool
	}{
		{state: PreparedDeckOutcomePending, active: true, batchSettable: true, batchSubmittable: true},
		{state: PreparedDeckOutcomeRunning, active: true},
		{state: PreparedDeckOutcomeCompleted, terminal: true, translationSettled: true, batchSettable: true, batchSubmittable: true},
		{state: PreparedDeckOutcomeFailed, terminal: true, translationSettled: true, batchSettable: true},
		{state: PreparedDeckOutcomeCancelled, terminal: true},
		{state: "invalid"},
	}
	checks := []struct {
		name string
		got  func(PreparedDeckOutcomeState) bool
		want func(int) bool
	}{
		{"terminal", PreparedDeckOutcomeState.Terminal, func(i int) bool { return tests[i].terminal }},
		{"active", PreparedDeckOutcomeState.Active, func(i int) bool { return tests[i].active }},
		{"translation settled", PreparedDeckOutcomeState.TranslationSettled, func(i int) bool { return tests[i].translationSettled }},
		{"batch settable", PreparedDeckOutcomeState.BatchSettable, func(i int) bool { return tests[i].batchSettable }},
		{"batch submittable", PreparedDeckOutcomeState.BatchSubmittable, func(i int) bool { return tests[i].batchSubmittable }},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			for i, tt := range tests {
				t.Run(string(tt.state), func(t *testing.T) {
					require.Equal(t, check.want(i), check.got(tt.state))
				})
			}
		})
	}
}

func TestPreparedDeckBatchChunkStateSets(t *testing.T) {
	tests := []struct {
		state                                                                                                                                    PreparedDeckBatchChunkState
		terminal, awaitingSubmission, awaitingPoll, submissionClosed, cleanupEligible, submissionSettled, reconciliationAllowed, failureReported bool
	}{
		{state: PreparedDeckBatchPending, awaitingSubmission: true},
		{state: PreparedDeckBatchSubmitting, awaitingSubmission: true},
		{state: PreparedDeckBatchSubmitted, awaitingPoll: true, submissionClosed: true},
		{state: PreparedDeckBatchPolling, awaitingPoll: true, submissionClosed: true, reconciliationAllowed: true},
		{state: PreparedDeckBatchReconciling, submissionClosed: true},
		{state: PreparedDeckBatchCompleted, terminal: true, submissionClosed: true, cleanupEligible: true, submissionSettled: true, reconciliationAllowed: true},
		{state: PreparedDeckBatchFailed, terminal: true, submissionClosed: true, submissionSettled: true, reconciliationAllowed: true, failureReported: true},
		{state: PreparedDeckBatchCancelled, terminal: true, submissionClosed: true, cleanupEligible: true},
		{state: PreparedDeckBatchAmbiguous, terminal: true, submissionClosed: true, submissionSettled: true, failureReported: true},
		{state: "invalid"},
	}
	checks := []struct {
		name string
		got  func(PreparedDeckBatchChunkState) bool
		want func(int) bool
	}{
		{"terminal", PreparedDeckBatchChunkState.Terminal, func(i int) bool { return tests[i].terminal }},
		{"awaiting submission", PreparedDeckBatchChunkState.AwaitingSubmission, func(i int) bool { return tests[i].awaitingSubmission }},
		{"awaiting poll", PreparedDeckBatchChunkState.AwaitingPoll, func(i int) bool { return tests[i].awaitingPoll }},
		{"submission closed", PreparedDeckBatchChunkState.SubmissionClosed, func(i int) bool { return tests[i].submissionClosed }},
		{"cleanup eligible", PreparedDeckBatchChunkState.CleanupEligible, func(i int) bool { return tests[i].cleanupEligible }},
		{"submission settled", PreparedDeckBatchChunkState.SubmissionSettled, func(i int) bool { return tests[i].submissionSettled }},
		{"reconciliation update allowed", PreparedDeckBatchChunkState.ReconciliationUpdateAllowed, func(i int) bool { return tests[i].reconciliationAllowed }},
		{"failure reported", PreparedDeckBatchChunkState.FailureReported, func(i int) bool { return tests[i].failureReported }},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			for i, tt := range tests {
				t.Run(string(tt.state), func(t *testing.T) {
					require.Equal(t, check.want(i), check.got(tt.state))
				})
			}
		})
	}
}

// TestPreparedDeckBatchFileCleanupStates pins the domain values to the database
// check constraint on the three cleanup-state columns.
func TestPreparedDeckBatchFileCleanupStates(t *testing.T) {
	t.Run("values", func(t *testing.T) {
		require.Equal(t, []string{"pending", "deleted", "failed", "not_needed"}, []string{
			string(PreparedDeckBatchCleanupPending),
			string(PreparedDeckBatchCleanupDeleted),
			string(PreparedDeckBatchCleanupFailed),
			string(PreparedDeckBatchCleanupNotNeeded),
		})
	})
	tests := []struct {
		state          PreparedDeckBatchFileCleanupState
		valid, settled bool
	}{
		{state: PreparedDeckBatchCleanupPending, valid: true},
		{state: PreparedDeckBatchCleanupDeleted, valid: true, settled: true},
		{state: PreparedDeckBatchCleanupFailed, valid: true},
		{state: PreparedDeckBatchCleanupNotNeeded, valid: true, settled: true},
		{state: "invalid"},
	}
	checks := []struct {
		name string
		got  func(PreparedDeckBatchFileCleanupState) bool
		want func(int) bool
	}{
		{"valid", PreparedDeckBatchFileCleanupState.Valid, func(i int) bool { return tests[i].valid }},
		{"settled", PreparedDeckBatchFileCleanupState.Settled, func(i int) bool { return tests[i].settled }},
	}
	for _, check := range checks {
		t.Run(check.name, func(t *testing.T) {
			for i, tt := range tests {
				t.Run(string(tt.state), func(t *testing.T) {
					require.Equal(t, check.want(i), check.got(tt.state))
				})
			}
		})
	}
}
