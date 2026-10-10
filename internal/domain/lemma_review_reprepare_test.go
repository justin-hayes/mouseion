package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDecideLemmaReprepare(t *testing.T) {
	// Each case changes one fact from the restart baseline, so the table reads
	// as the policy's branches in order.
	restart := LemmaRepreparationFacts{IdentityChanged: true, ReadyDeck: true, SnapshotBound: true, ToRead: true, Confirmed: true}
	cases := []struct {
		name   string
		facts  LemmaRepreparationFacts
		want   LemmaRepreparationAction
		wantOK bool
	}{
		{"no identity change needs nothing", LemmaRepreparationFacts{ReadyDeck: true, SnapshotBound: true, ToRead: true, Confirmed: true}, LemmaRepreparationNone, true},
		{"no ready deck needs nothing", LemmaRepreparationFacts{IdentityChanged: true, ToRead: true, Confirmed: true}, LemmaRepreparationNone, true},
		{"unconfirmed change to a ready deck is refused", LemmaRepreparationFacts{IdentityChanged: true, ReadyDeck: true, SnapshotBound: true, ToRead: true}, LemmaRepreparationUnconfirmed, false},
		{"unbound ready deck is re-prepared directly", LemmaRepreparationFacts{IdentityChanged: true, ReadyDeck: true, ToRead: true, Confirmed: true}, LemmaRepreparationDirect, true},
		{"another current reading blocks the restart", func() LemmaRepreparationFacts { f := restart; f.OtherCurrentReading = true; return f }(), LemmaRepreparationBlockedByReading, false},
		{"a Book that is not To Read cannot restart", func() LemmaRepreparationFacts { f := restart; f.ToRead = false; return f }(), LemmaRepreparationNotToRead, false},
		{"a To Read Book restarts under a new snapshot", restart, LemmaRepreparationRestart, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			action := DecideLemmaReprepare(tc.facts)
			assert.Equal(t, tc.want, action)
			if tc.wantOK {
				assert.NoError(t, action.Err())
			} else {
				assert.Error(t, action.Err())
			}
		})
	}
}

func TestLemmaRepreparationActionErrMapsRejections(t *testing.T) {
	require.ErrorIs(t, LemmaRepreparationUnconfirmed.Err(), ErrLemmaReviewRepreparationRequired)
	require.ErrorIs(t, LemmaRepreparationBlockedByReading.Err(), ErrLemmaReviewBlockedByReading)
	require.ErrorIs(t, LemmaRepreparationNotToRead.Err(), ErrLemmaReviewNotToRead)
	require.NoError(t, LemmaRepreparationNone.Err())
	require.NoError(t, LemmaRepreparationDirect.Err())
	require.NoError(t, LemmaRepreparationRestart.Err())
}
