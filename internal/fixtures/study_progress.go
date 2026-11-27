package fixtures

// Contract status: contractual. Study progress, review log and batch ledger
// transitions must match internal/storecontract scenarios shared with
// PostgreSQL (ADR 0088).

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/justin-hayes/mouseion/internal/domain"
)

func studyLedgerKey(owner, language, studyDay string) string {
	return owner + "\x00" + strings.ToLower(language) + "\x00" + studyDay
}

// ListStudyProgress returns every word the learner has in study in the study
// language, oldest introduction first.
func (s *Store) ListStudyProgress(_ context.Context, owner, language string) ([]domain.StudyProgress, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	progress := make([]domain.StudyProgress, 0, len(s.studyProgress))
	for _, item := range s.studyProgress {
		if item.OwnerID == owner && item.Language == language {
			item.Reintroducing = s.studySuperseded[item.MaterialID]
			item.Held = s.studyHeldLocked(owner, item.StudyItem)
			item.ContextReportedAt = s.contextReportedAtLocked(owner, item.ID)
			progress = append(progress, item)
		}
	}
	sort.SliceStable(progress, func(i, j int) bool {
		a, b := progress[i], progress[j]
		if !a.IntroducedAt.Equal(b.IntroducedAt) {
			return a.IntroducedAt.Before(b.IntroducedAt)
		}
		if a.CanonicalLemma != b.CanonicalLemma {
			return a.CanonicalLemma < b.CanonicalLemma
		}
		if a.UPOS != b.UPOS {
			return a.UPOS < b.UPOS
		}
		return a.LexicalDiscriminator < b.LexicalDiscriminator
	})
	return progress, nil
}

// StudyBatchLedger reports the batches taken and declined on a Study day.
func (s *Store) StudyBatchLedger(_ context.Context, owner, language, studyDay string) (domain.StudyBatchLedger, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	ledger := s.studyLedger[studyLedgerKey(owner, language, studyDay)]
	ledger.StudyDay = studyDay
	return ledger, nil
}

// takeCarriedLocked takes a word of an earlier Study day's batch again: it
// becomes new and due now. A word no longer waiting is skipped. The caller
// holds s.mu.
func (s *Store) takeCarriedLocked(owner, language, progressID string, now time.Time) []domain.StudyProgress {
	for i := range s.studyProgress {
		item := &s.studyProgress[i]
		if item.OwnerID != owner || item.ID != progressID || item.Language != language {
			continue
		}
		if item.Memory.State != domain.StudyStateNew || item.Graded() || item.Withdrawn || !item.IntroducedAt.Before(domain.StudyDayStart(now)) {
			return nil
		}
		item.IntroducedAt, item.Memory.Due, item.UpdatedAt = now.UTC(), now.UTC(), now.UTC()
		return []domain.StudyProgress{*item}
	}
	return nil
}

// RecordStudyBatchOffer records that Today has offered this many batches on the
// Study day. The count only grows.
func (s *Store) RecordStudyBatchOffer(_ context.Context, owner, language, studyDay string, offered int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := studyLedgerKey(owner, language, studyDay)
	ledger := s.studyLedger[key]
	ledger.Offered = max(ledger.Offered, offered)
	s.storeLedgerLocked(key, ledger)
	return nil
}

func (s *Store) storeLedgerLocked(key string, ledger domain.StudyBatchLedger) {
	if s.studyLedger == nil {
		s.studyLedger = map[string]domain.StudyBatchLedger{}
	}
	s.studyLedger[key] = ledger
}

// TakeStudyBatch takes one batch: it records it in the day's ledger and
// introduces each word from its ready Study material, due now, with the
// material's Gloss as its learned meaning.
func (s *Store) TakeStudyBatch(_ context.Context, owner, language, studyDay string, now time.Time, batchLimit int, words []domain.StudyBatchItem) ([]domain.StudyProgress, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := studyLedgerKey(owner, language, studyDay)
	ledger, err := s.studyLedger[key].Take(batchLimit)
	if err != nil {
		return nil, err
	}
	var introduced []domain.StudyProgress
	for _, word := range words {
		if word.ProgressID != "" {
			introduced = append(introduced, s.takeCarriedLocked(owner, language, word.ProgressID, now)...)
			continue
		}
		material, ok := s.currentReadyMaterialLocked(owner, language, word.MaterialID)
		if !ok || s.progressIndexLocked(owner, material.StudyItem) >= 0 || s.studyHeldLocked(owner, material.StudyItem) {
			continue
		}
		item := domain.StudyProgress{
			ID: s.nextStudyProgressIDLocked(), OwnerID: owner, StudyItem: material.StudyItem,
			MaterialID: material.ID, IntroducedAt: now.UTC(), LearnedMeaning: material.Gloss, LearnedCitations: material.Cited.Clone(),
			Memory:    domain.NewStudyMemory(now),
			CreatedAt: now.UTC(), UpdatedAt: now.UTC(),
		}
		s.studyProgress = append(s.studyProgress, item)
		introduced = append(introduced, item)
	}
	if len(introduced) == 0 {
		return nil, errNotFound
	}
	s.noteStudyActionLocked(owner, language)
	s.storeLedgerLocked(key, ledger)
	return introduced, nil
}

// DeclineStudyBatch declines one batch. It introduces no word.
func (s *Store) DeclineStudyBatch(_ context.Context, owner, language, studyDay string, batchLimit int) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	key := studyLedgerKey(owner, language, studyDay)
	ledger, err := s.studyLedger[key].Decline(batchLimit)
	if err != nil {
		return err
	}
	s.noteStudyActionLocked(owner, language)
	s.storeLedgerLocked(key, ledger)
	return nil
}

// NextStudyCard returns the word to show next with its Study material, or
// ErrNotFound when nothing is due at now. What is due is decided by the domain.
func (s *Store) NextStudyCard(ctx context.Context, owner, language string, now time.Time) (domain.StudyCard, error) {
	all, err := s.ListStudyProgress(ctx, owner, language)
	if err != nil {
		return domain.StudyCard{}, err
	}
	progress, ok := domain.NextStudyReview(all, now)
	if !ok {
		return domain.StudyCard{}, errNotFound
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, material := range s.studyMaterial {
		if material.OwnerID == owner && material.ID == progress.MaterialID {
			return domain.StudyCard{Progress: progress, Material: cloneStudyMaterial(material)}, nil
		}
	}
	return domain.StudyCard{}, errNotFound
}

// RecordStudyGrade stores one graded review: the review-log row with the state
// before and after, and the word's new scheduling state. A card graded twice is
// graded once: the second grade is domain.ErrStudyStale.
func (s *Store) RecordStudyGrade(_ context.Context, owner string, record domain.StudyGradeRecord) error {
	if _, ok := domain.ParseStudyGrade(string(record.Grade)); !ok {
		return fmt.Errorf("record study grade: unknown grade %q", record.Grade)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.studyProgress {
		item := &s.studyProgress[i]
		if item.OwnerID != owner || item.ID != record.ProgressID {
			continue
		}
		if item.Withdrawn || item.Memory.Reps != record.ExpectedReps || s.studyHeldLocked(owner, item.StudyItem) || s.studySuperseded[item.MaterialID] {
			return domain.ErrStudyStale
		}
		s.studyReviewLog = append(s.studyReviewLog, domain.StudyReviewLog{
			ID: fmt.Sprintf("fixture-study-review-%d", len(s.studyReviewLog)+1), OwnerID: owner, ProgressID: item.ID, MaterialID: item.MaterialID,
			Grade: record.Grade, ReviewedAt: record.ReviewedAt.UTC(), StudyDay: record.StudyDay, Before: item.Memory, After: record.After,
		})
		item.Memory = record.After
		item.UpdatedAt = record.ReviewedAt.UTC()
		s.noteStudyActionLocked(owner, item.Language)
		return nil
	}
	return errNotFound
}

// RecordStudyAlreadyKnown stores an Already-known answer on an introduction: the
// event, which is never a grade and so is not in the review log, and the word's
// new scheduling state. An introduction answered twice is answered once: the
// second answer, or an answer to a card that is not an introduction, is
// domain.ErrStudyStale.
func (s *Store) RecordStudyAlreadyKnown(_ context.Context, owner string, record domain.StudyAlreadyKnownRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for i := range s.studyProgress {
		item := &s.studyProgress[i]
		if item.OwnerID != owner || item.ID != record.ProgressID {
			continue
		}
		if item.Withdrawn || item.Memory.Reps != record.ExpectedReps || s.studyHeldLocked(owner, item.StudyItem) || s.studySuperseded[item.MaterialID] || !domain.StudyAlreadyKnownOffered(*item) {
			return domain.ErrStudyStale
		}
		s.studyAlreadyKnown = append(s.studyAlreadyKnown, domain.StudyAlreadyKnownEvent{
			ID: fmt.Sprintf("fixture-study-already-known-%d", len(s.studyAlreadyKnown)+1), OwnerID: owner, StudyItem: item.StudyItem,
			ProgressID: item.ID, MaterialID: item.MaterialID, Source: domain.StudyAlreadyKnownIntroduction,
			AnsweredAt: record.AnsweredAt.UTC(), StudyDay: record.StudyDay,
		})
		item.Memory = record.After
		item.UpdatedAt = record.AnsweredAt.UTC()
		s.noteStudyActionLocked(owner, item.Language)
		return nil
	}
	return errNotFound
}

// ListStudyAlreadyKnownEvents returns the learner's Already-known events of a
// study language, oldest first.
func (s *Store) ListStudyAlreadyKnownEvents(_ context.Context, owner, language string) ([]domain.StudyAlreadyKnownEvent, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var events []domain.StudyAlreadyKnownEvent
	for _, event := range s.studyAlreadyKnown {
		if event.OwnerID == owner && event.Language == language {
			events = append(events, event)
		}
	}
	return events, nil
}

// ListStudyReviewLog returns a word's review log, oldest first.
func (s *Store) ListStudyReviewLog(_ context.Context, owner, progressID string) ([]domain.StudyReviewLog, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var logs []domain.StudyReviewLog
	for _, entry := range s.studyReviewLog {
		if entry.OwnerID == owner && entry.ProgressID == progressID {
			logs = append(logs, entry)
		}
	}
	return logs, nil
}

// currentReadyMaterialLocked returns the owner's current, ready material with
// the ID. The caller holds s.mu.
func (s *Store) currentReadyMaterialLocked(owner, language, id string) (domain.StudyMaterial, bool) {
	for _, item := range s.studyMaterial {
		if item.OwnerID == owner && item.ID == id && item.Language == language && item.State == domain.StudyMaterialReady && !s.studySuperseded[item.ID] {
			return item, true
		}
	}
	return domain.StudyMaterial{}, false
}

// progressIndexLocked is the index of the word's progress, or -1. The caller
// holds s.mu.
func (s *Store) progressIndexLocked(owner string, identity domain.StudyItem) int {
	for i, item := range s.studyProgress {
		if item.OwnerID == owner && item.StudyItem == identity {
			return i
		}
	}
	return -1
}
