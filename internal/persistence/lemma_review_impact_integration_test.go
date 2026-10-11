//go:build integration

package persistence

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/selection"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// reviewImpactBook creates a Book whose analysis repeats one word per entry in
// words, builds its count projection, and returns it.
func reviewImpactBook(t *testing.T, ctx context.Context, store *PostgresStore, ownerID, suffix string, toRead bool, words []string) domain.Book {
	t.Helper()
	text := strings.Join(words, " ")
	end := int64(len([]rune(text)))
	book, source := createConcordanceBook(t, ctx, store, ownerID, "Review impact "+suffix, suffix, toRead,
		[]domain.ExtractedUnit{concordanceUnit(0, suffix, text, 0, uint64(end))})
	tokens := make([]concordanceToken, 0, len(words))
	offset := int64(0)
	for _, word := range words {
		length := int64(len([]rune(word)))
		tokens = append(tokens, concordanceToken{Surface: word, Lemma: strings.ToLower(word), Upos: "NOUN", Start: offset, End: offset + length})
		offset += length + 1
	}
	insertConcordanceAnalysis(t, ctx, store, source, true, []concordanceSentence{{
		UnitID: "epub-unit-v1:0:" + suffix, Ordinal: 0, Text: text, Start: 0, End: end, Tokens: tokens,
	}})
	var runID, corpusID string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT analysis_run_id::text,corpus_id::text FROM current_analysis_identity WHERE owner_id=$1 AND book_id=$2`, ownerID, book.ID).Scan(&runID, &corpusID))
	tx, err := store.Pool().Begin(ctx)
	require.NoError(t, err)
	require.NoError(t, BuildVocabularyBrowseCountsTx(ctx, tx, ownerID, book.ID, source.ID, runID, corpusID, "de"))
	require.NoError(t, tx.Commit(ctx))
	return book
}

// reviewImpactReservingBook creates a To Read Book whose three "haus"
// occurrences exist only through a correction, which makes the snapshot freeze
// project them, so starting it reserves the identity "haus".
func reviewImpactReservingBook(t *testing.T, ctx context.Context, store *PostgresStore, ownerID string) domain.Book {
	t.Helper()
	book := reviewImpactBook(t, ctx, store, ownerID, "reserving", true, []string{"Haus", "Haus", "Heim"})
	occurrences, err := store.ListLemmaReviewOccurrences(ctx, ownerID, book.ID, "Heim")
	require.NoError(t, err)
	require.NoError(t, store.PutLemmaDecisions(ctx, []domain.LemmaReviewDecision{{Occurrence: occurrences[0], CanonicalLemma: "haus", NormalizationProfile: "german-post-1996", NormalizationVersion: "6"}}))
	return book
}

// reviewProposal builds a single-occurrence proposal on the Book's surface form.
func reviewProposal(ownerID, bookID, form, action, lemma string, occurrence domain.LemmaReviewOccurrence) domain.LemmaReviewProposal {
	return domain.LemmaReviewProposal{
		OwnerID: ownerID, BookID: bookID, Language: "de", Surface: form, Action: action, Lemma: lemma,
		Occurrences: []domain.LemmaReviewOccurrence{occurrence}, NormalizationProfile: "german-post-1996", NormalizationVersion: "6",
	}
}

// TestLemmaReviewImpactAgreesWithSnapshotFreeze previews a decision, confirms
// it, freezes the Current reading, and requires the previewed "after"
// eligibility to match whether the identity is in the frozen snapshot.
func TestLemmaReviewImpactAgreesWithSnapshotFreeze(t *testing.T) {
	repeat := func(word string, n int) []string { return strings.Fields(strings.Repeat(word+" ", n)) }
	tests := []struct {
		name          string
		target        []string
		otherBooks    [][]string
		known         bool
		reserved      bool
		action, lemma string
		form          string
		wantBefore    bool
		wantAfter     bool
		wantAfterSize int
	}{
		{
			name:   "a correction crosses the three-occurrence threshold",
			target: append(repeat("Haus", 2), "Heim"),
			form:   "Heim", action: "correct", lemma: "haus",
			wantBefore: false, wantAfter: true, wantAfterSize: 3,
		},
		{
			name:       "a correction qualifies only through two occurrences with ten across Books",
			target:     append(repeat("Haus", 1), "Heim"),
			otherBooks: [][]string{repeat("Haus", 8)},
			form:       "Heim", action: "correct", lemma: "haus",
			wantBefore: false, wantAfter: true, wantAfterSize: 2,
		},
		{
			name:       "two occurrences with nine across Books stay ineligible",
			target:     append(repeat("Haus", 1), "Heim"),
			otherBooks: [][]string{repeat("Haus", 7)},
			form:       "Heim", action: "correct", lemma: "haus",
			wantBefore: false, wantAfter: false, wantAfterSize: 0,
		},
		{
			name:   "a correction into a Known identity is not eligible",
			target: append(repeat("Haus", 2), "Heim"),
			known:  true,
			form:   "Heim", action: "correct", lemma: "haus",
			wantBefore: false, wantAfter: false, wantAfterSize: 0,
		},
		{
			name:     "a correction into a Reserved identity is not eligible",
			target:   append(repeat("Haus", 2), "Heim"),
			reserved: true,
			form:     "Heim", action: "correct", lemma: "haus",
			wantBefore: false, wantAfter: false, wantAfterSize: 0,
		},
		{
			name:   "an exclusion drops an identity below the threshold",
			target: repeat("Haus", 3),
			form:   "Haus", action: "exclude",
			wantBefore: true, wantAfter: false, wantAfterSize: 0,
		},
	}
	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
			owner, err := store.CreateUser(ctx, fmt.Sprintf("review-impact-%d", index), false)
			require.NoError(t, err)
			for i, words := range tt.otherBooks {
				reviewImpactBook(t, ctx, store, owner.ID, fmt.Sprintf("other%d", i), false, words)
			}
			if tt.known {
				_, err = store.PutKnownVocabulary(ctx, owner.ID, "de", "haus", "NOUN")
				require.NoError(t, err)
			}
			var reservingBook domain.Book
			if tt.reserved {
				reservingBook = reviewImpactReservingBook(t, ctx, store, owner.ID)
			}
			book := reviewImpactBook(t, ctx, store, owner.ID, "target", true, tt.target)
			if tt.reserved {
				_, err = store.StartCurrentReading(ctx, owner.ID, "de", reservingBook.ID)
				require.NoError(t, err)
			}

			occurrences, err := store.ListLemmaReviewOccurrences(ctx, owner.ID, book.ID, tt.form)
			require.NoError(t, err)
			proposal := reviewProposal(owner.ID, book.ID, tt.form, tt.action, tt.lemma, occurrences[0])
			preview, err := store.ReadLemmaReviewProposal(ctx, proposal)
			require.NoError(t, err)
			var haus selection.Impact
			for _, impact := range selection.ReviewImpact(preview.States) {
				if impact.Identity.CanonicalLemma == "haus" {
					haus = impact
				}
			}
			assert.Equal(t, tt.wantBefore, haus.BeforeEligible)
			assert.Equal(t, tt.wantAfter, haus.AfterEligible)

			unchanged, err := store.ListLemmaReviewOccurrences(ctx, owner.ID, book.ID, tt.form)
			require.NoError(t, err)
			assert.Equal(t, occurrences, unchanged, "previewing writes nothing")

			if tt.reserved {
				require.NoError(t, store.PutLemmaDecisionProposal(ctx, proposal, preview.Fingerprint))
				_, err = store.StartCurrentReading(ctx, owner.ID, "de", book.ID)
				require.ErrorIs(t, err, ErrCurrentReadingExists, "while the identity is Reserved no other snapshot can freeze it")
				return
			}
			require.NoError(t, store.PutLemmaDecisionProposal(ctx, proposal, preview.Fingerprint))
			reading, err := store.StartCurrentReading(ctx, owner.ID, "de", book.ID)
			require.NoError(t, err)
			snapshot, err := store.ListCurrentReadingSnapshotVocabulary(ctx, owner.ID, reading.SnapshotID)
			require.NoError(t, err)
			frozen := 0
			for _, candidate := range snapshot {
				if candidate.CanonicalLemma == "haus" {
					frozen = candidate.OccurrenceCount
				}
			}
			assert.Equal(t, haus.AfterEligible, frozen > 0, "the previewed eligibility is what the freeze selects")
			assert.Equal(t, tt.wantAfterSize, frozen)
		})
	}
}

func TestReadLemmaReviewProposalWithholdsUntilProjectionsAreReady(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "review-impact-pending", false)
	require.NoError(t, err)
	book := reviewImpactBook(t, ctx, store, owner.ID, "pending", false, []string{"Haus", "Heim"})
	occurrences, err := store.ListLemmaReviewOccurrences(ctx, owner.ID, book.ID, "Heim")
	require.NoError(t, err)
	_, err = store.Pool().Exec(ctx, `DELETE FROM vocabulary_browse_count_readiness WHERE owner_id=$1 AND book_id=$2`, owner.ID, book.ID)
	require.NoError(t, err)
	_, err = store.ReadLemmaReviewProposal(ctx, reviewProposal(owner.ID, book.ID, "Heim", "correct", "haus", occurrences[0]))
	require.ErrorIs(t, err, ErrVocabularyBrowseCountsPending)
}

// TestLemmaReviewFingerprintRejectsEveryChangeThePreviewDependedOn previews a
// correction of "Heim" into "haus" and then changes one thing the preview's
// impact read. Each change must make confirming stale; a change to an identity
// the proposal does not affect must not.
func TestLemmaReviewFingerprintRejectsEveryChangeThePreviewDependedOn(t *testing.T) {
	repeat := func(word string, n int) []string { return strings.Fields(strings.Repeat(word+" ", n)) }
	tests := []struct {
		name      string
		target    []string
		other     []string
		reserving bool
		change    func(t *testing.T, ctx context.Context, store *PostgresStore, ownerID string, target, other, reserving domain.Book)
		wantStale bool
	}{
		{
			name:   "a decision on another surface form of the same lemma",
			target: []string{"Haus", "Heim", "Dach"},
			change: func(t *testing.T, ctx context.Context, store *PostgresStore, ownerID string, target, _, _ domain.Book) {
				occurrences, err := store.ListLemmaReviewOccurrences(ctx, ownerID, target.ID, "Dach")
				require.NoError(t, err)
				require.NoError(t, store.PutLemmaDecisions(ctx, []domain.LemmaReviewDecision{{Occurrence: occurrences[0], CanonicalLemma: "haus", NormalizationProfile: "german-post-1996", NormalizationVersion: "6"}}))
			},
			wantStale: true,
		},
		{
			name:   "a cross-Book count change that flips the two-plus-ten route",
			target: []string{"Haus", "Heim"},
			other:  repeat("Haus", 8),
			change: func(t *testing.T, ctx context.Context, store *PostgresStore, ownerID string, _, other, _ domain.Book) {
				occurrences, err := store.ListLemmaReviewOccurrences(ctx, ownerID, other.ID, "Haus")
				require.NoError(t, err)
				require.NoError(t, store.PutLemmaDecisions(ctx, []domain.LemmaReviewDecision{{Occurrence: occurrences[0], Excluded: true}}))
			},
			wantStale: true,
		},
		{
			name:   "an affected identity becoming Known",
			target: []string{"Haus", "Heim"},
			change: func(t *testing.T, ctx context.Context, store *PostgresStore, ownerID string, _, _, _ domain.Book) {
				_, err := store.PutKnownVocabulary(ctx, ownerID, "de", "haus", "NOUN")
				require.NoError(t, err)
			},
			wantStale: true,
		},
		{
			name:      "an affected identity becoming Reserved",
			target:    []string{"Haus", "Heim"},
			reserving: true,
			change: func(t *testing.T, ctx context.Context, store *PostgresStore, ownerID string, _, _, reserving domain.Book) {
				_, err := store.StartCurrentReading(ctx, ownerID, "de", reserving.ID)
				require.NoError(t, err)
			},
			wantStale: true,
		},
		{
			name:   "an unaffected identity becoming Known",
			target: []string{"Haus", "Heim"},
			change: func(t *testing.T, ctx context.Context, store *PostgresStore, ownerID string, _, _, _ domain.Book) {
				_, err := store.PutKnownVocabulary(ctx, ownerID, "de", "unrelated", "NOUN")
				require.NoError(t, err)
			},
		},
	}
	for index, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
			owner, err := store.CreateUser(ctx, fmt.Sprintf("review-fingerprint-%d", index), false)
			require.NoError(t, err)
			var other, reserving domain.Book
			if tt.other != nil {
				other = reviewImpactBook(t, ctx, store, owner.ID, "other", false, tt.other)
			}
			if tt.reserving {
				reserving = reviewImpactReservingBook(t, ctx, store, owner.ID)
			}
			target := reviewImpactBook(t, ctx, store, owner.ID, "target", true, tt.target)
			occurrences, err := store.ListLemmaReviewOccurrences(ctx, owner.ID, target.ID, "Heim")
			require.NoError(t, err)
			proposal := reviewProposal(owner.ID, target.ID, "Heim", "correct", "haus", occurrences[0])

			previewed, err := store.ReadLemmaReviewProposal(ctx, proposal)
			require.NoError(t, err)
			again, err := store.ReadLemmaReviewProposal(ctx, proposal)
			require.NoError(t, err)
			require.Equal(t, previewed.Fingerprint, again.Fingerprint, "the fingerprint is stable while nothing changes")

			tt.change(t, ctx, store, owner.ID, target, other, reserving)

			err = store.PutLemmaDecisionProposal(ctx, proposal, previewed.Fingerprint)
			if tt.wantStale {
				require.ErrorIs(t, err, domain.ErrLemmaReviewPreviewStale)
				unchanged, listErr := store.ListLemmaReviewOccurrences(ctx, owner.ID, target.ID, "Heim")
				require.NoError(t, listErr)
				assert.Empty(t, unchanged[0].CorrectedLemma, "a stale preview persists nothing")
				return
			}
			require.NoError(t, err)
		})
	}
}
