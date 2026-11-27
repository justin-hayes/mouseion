// Package study runs the durable Study jobs: it keeps Study material prepared
// a short way ahead of the new words Today will offer, one word at a time
// (ADR 0134 §8). It is a job package in the sense of ADR 0091 §3: a job is
// enqueued in the same transaction as the pending record it prepares
// (ADR 0010), so the package holds pgx.
package study

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/riverdriver/riverpgxv5"
	"github.com/riverqueue/river/rivertype"
)

// Queue is the River queue Study material preparation runs on. The River
// client's queue map in internal/analysis must name it.
const Queue = "study"

// maxPreparationAttempts bounds River's retries of one preparation job. A word
// left pending when the job is discarded is enqueued again the next time the
// Study page is opened.
const maxPreparationAttempts = 8

// PrepareMaterialArgs identifies one pending Study material record. It carries
// no source text, sentence or Gloss.
type PrepareMaterialArgs struct {
	OwnerID    string `json:"owner_id"`
	MaterialID string `json:"material_id" river:"unique"`
}

// Kind names the job for River.
func (PrepareMaterialArgs) Kind() string { return "study_prepare_material" }

type riverClient interface {
	Insert(context.Context, river.JobArgs, *river.InsertOpts) (*rivertype.JobInsertResult, error)
	InsertTx(context.Context, pgx.Tx, river.JobArgs, *river.InsertOpts) (*rivertype.JobInsertResult, error)
}

// Service enqueues Study material preparation. The web app reaches it only
// through the seam it defines, so no page request calls a provider.
type Service struct {
	store  *persistence.PostgresStore
	client riverClient
	// dictionaryPin reports the loaded dictionary's pin for a language, or empty
	// when none is loaded; see WithDictionaryPin.
	dictionaryPin func(language string) string
	// idioms is Study's idiom switch; nil leaves Idioms out of the stream.
	idioms *IdiomAvoidance
	// now is the clock planning reconciles reports by; see WithClock.
	now func() time.Time
}

// WithDictionaryPin sets how planning learns the loaded dictionary's pin for a
// language, so a word omitted for having no dictionary entry is prepared again
// when the pin changes (#2775). Without it such a word is never retried.
func (s *Service) WithDictionaryPin(pin func(language string) string) *Service {
	s.dictionaryPin = pin
	return s
}

// WithIdioms sets Study's idiom switch. With a language's switch on, its Idioms
// join the new-word stream (ADR 0137 §4); without it no Idiom is a candidate.
func (s *Service) WithIdioms(idioms *IdiomAvoidance) *Service {
	s.idioms = idioms
	return s
}

// WithClock sets the clock planning reconciles Wrong-word reports by. It must
// be the web app's clock: a report the planning job ends releases its word for
// the start of the Study day after this time, as the page request would
// (#2786). Without it the wall clock is used.
func (s *Service) WithClock(now func() time.Time) *Service {
	s.now = now
	return s
}

// pinOf is the pin a no-dictionary-entry omission of the item is compared with:
// an Idiom's is the idiom index version, a word's the language's dictionary pin.
func (s *Service) pinOf(item domain.StudyItem, languagePin string) string {
	if item.IsIdiom() && s.idioms != nil {
		return strings.TrimSpace(s.idioms.IndexVersion)
	}
	return languagePin
}

// currentDictionaryPin is the loaded dictionary's pin for the language, or
// empty when there is none to compare with.
func (s *Service) currentDictionaryPin(language string) string {
	if s.dictionaryPin == nil {
		return ""
	}
	return strings.TrimSpace(s.dictionaryPin(language))
}

// NewService returns the preparation enqueuer over the store and River client.
func NewService(store *persistence.PostgresStore, client riverClient) *Service {
	return &Service{store: store, client: client, now: time.Now}
}

// PlanPreparationArgs identifies the learner and Study language whose
// preparation is to be planned. It carries no source text.
type PlanPreparationArgs struct {
	OwnerID  string `json:"owner_id" river:"unique"`
	Language string `json:"language" river:"unique"`
	// Trigger is empty for a page trigger, so waiting and running plans absorb
	// it. A publication sets a value no other plan has: a running plan may
	// already have read the stream before that analysis was published, so it
	// must never absorb the publication's plan.
	Trigger string `json:"trigger,omitempty" river:"unique"`
}

// Kind names the job for River.
func (PlanPreparationArgs) Kind() string { return "study_plan_preparation" }

// planUniqueStates are River's default unique states without completed: a plan
// that has finished must not stop the next trigger from queueing another.
var planUniqueStates = []rivertype.JobState{
	rivertype.JobStateAvailable, rivertype.JobStatePending, rivertype.JobStateRunning,
	rivertype.JobStateRetryable, rivertype.JobStateScheduled,
}

// EnqueueStudyPreparation asks for the learner's Study material to be brought
// up to date with the stream. It inserts one planning job, which River does not
// queue twice while one is waiting, and reads nothing else: the planning runs in
// PlanWorker. It is safe to call whenever Today is opened or a batch is taken;
// the Study-day rollover is seen by the next page load, as before.
func (s *Service) EnqueueStudyPreparation(ctx context.Context, owner, language string) error {
	if _, err := s.client.Insert(ctx, PlanPreparationArgs{OwnerID: owner, Language: language}, planInsertOpts()); err != nil {
		return fmt.Errorf("enqueue study preparation planning: %w", err)
	}
	return nil
}

func planInsertOpts() *river.InsertOpts {
	return &river.InsertOpts{
		Queue: Queue, MaxAttempts: maxPreparationAttempts,
		UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: planUniqueStates},
	}
}

// planInserter is the insert-only River client that queues planning from inside
// another package's transaction. It has no workers and no queues and never
// connects: InsertTx writes through the caller's transaction.
var planInserter = sync.OnceValues(func() (*river.Client[pgx.Tx], error) {
	return river.NewClient(riverpgxv5.New(nil), &river.Config{})
})

// EnqueuePlanningTx queues Study preparation planning for the learner and
// language in the caller's transaction, so planning is durable exactly when the
// analysis it follows is published (ADR 0010). It queues nothing for a language
// Study does not serve, and River does not queue a second plan while one waits.
func EnqueuePlanningTx(ctx context.Context, tx pgx.Tx, owner, language string) error {
	if language == "" || !Available(language) {
		return nil
	}
	trigger := strconv.FormatInt(time.Now().UnixNano(), 10)
	client, err := planInserter()
	if err != nil {
		return fmt.Errorf("create study planning inserter: %w", err)
	}
	if _, err := client.InsertTx(ctx, tx, PlanPreparationArgs{OwnerID: owner, Language: language, Trigger: trigger}, planInsertOpts()); err != nil {
		return fmt.Errorf("enqueue study preparation planning: %w", err)
	}
	return nil
}

// PlanPreparation brings the learner's Study material up to date with the
// stream: the first domain.StudyPreparationWindow words in stream order that
// are not in study and not unpreparable each get a pending record and a durable job. A word
// whose material is ready is left alone, and one whose job is still live is not
// enqueued twice.
func (s *Service) PlanPreparation(ctx context.Context, owner, language string) error {
	// A publication may have repaired a reported word's occurrence; a held word
	// is never due, so Today alone would not notice.
	if err := ReconcileReports(ctx, s.store, owner, language, s.now()); err != nil {
		return err
	}
	candidates, err := s.store.ListStudyCandidates(ctx, owner, language)
	if err != nil {
		return err
	}
	if s.idioms.active(language) {
		// An Idiom's count comes from the per-Book counts, which a job keeps;
		// a Book whose counts are not ready adds its Idioms when they are, at
		// the next plan.
		idiomCandidates, _, err := s.store.ListStudyIdiomCandidates(ctx, owner, language, s.idioms.IndexVersion)
		if err != nil {
			return err
		}
		candidates = append(candidates, idiomCandidates...)
	}
	progress, err := s.store.ListStudyProgress(ctx, owner, language)
	if err != nil {
		return err
	}
	material, err := s.store.ListCurrentStudyMaterial(ctx, owner, language)
	if err != nil {
		return err
	}
	// A held word is not prepared ahead: it would crowd the window.
	reports, err := s.store.ListOpenStudyReports(ctx, owner, language, domain.StudyReportWrongWord)
	if err != nil {
		return err
	}
	pin := s.currentDictionaryPin(language)
	current := make(map[domain.StudyItem]domain.StudyMaterial, len(material))
	states := make(map[domain.StudyItem]domain.StudyMaterialState, len(material))
	for _, item := range material {
		current[item.StudyItem] = item
		states[item.StudyItem] = item.State
		if item.RetriesAgainstDictionary(s.pinOf(item.StudyItem, pin)) {
			// Its omission was made against another dictionary: the word counts
			// as waiting, not skipped, so it takes its place in the window.
			states[item.StudyItem] = domain.StudyMaterialPending
		}
	}
	targets := domain.StudyPreparationTargets(domain.OrderStudyStream(domain.WithoutStudyProgress(candidates, progress, domain.StudyHeldIdentities(reports))), states)
	if err := s.prepareTargets(ctx, owner, targets, current, func(item domain.StudyItem) string { return s.pinOf(item, pin) }); err != nil {
		return err
	}
	return s.enqueueReintroductions(ctx, owner, progress, current)
}

// prepareTargets enqueues each target whose material is not ready. A word
// skipped for having no dictionary entry against another pin is made pending
// again first, so it is prepared against the dictionary now loaded.
func (s *Service) prepareTargets(ctx context.Context, owner string, targets []domain.StudyCandidate, current map[domain.StudyItem]domain.StudyMaterial, pinOf func(domain.StudyItem) string) error {
	for _, target := range targets {
		existing, ok := current[target.StudyItem]
		if ok && existing.State == domain.StudyMaterialReady {
			continue
		}
		pin := pinOf(target.StudyItem)
		if ok && existing.RetriesAgainstDictionary(pin) {
			// A missing record means the omission was settled again meanwhile.
			if err := s.store.ReopenStudyMaterialNoDictionaryEntry(ctx, owner, existing.ID, pin); err != nil && !errors.Is(err, domain.ErrNotFound) {
				return err
			}
		}
		if err := s.enqueueWord(ctx, owner, target.StudyItem, existing.ID); err != nil {
			return err
		}
	}
	return nil
}

// enqueueReintroductions prepares new material for the words in study whose
// material a correction replaced, so a due review is re-introduced in a sentence
// that belongs to the word. A word withdrawn by a correction waits.
func (s *Service) enqueueReintroductions(ctx context.Context, owner string, progress []domain.StudyProgress, current map[domain.StudyItem]domain.StudyMaterial) error {
	for _, item := range progress {
		existing, ok := current[item.StudyItem]
		if item.Withdrawn || (ok && existing.State != domain.StudyMaterialPending) {
			continue
		}
		if err := s.enqueueWord(ctx, owner, item.StudyItem, existing.ID); err != nil {
			return err
		}
	}
	return nil
}

// enqueueWord reserves the word's pending record, unless existingID names one,
// and inserts its preparation job in the same transaction.
func (s *Service) enqueueWord(ctx context.Context, owner string, identity domain.StudyItem, existingID string) (err error) {
	tx, err := s.store.Pool().Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	id := existingID
	if id == "" {
		reserved, created, err := s.store.ReserveStudyMaterialTx(ctx, tx, owner, identity)
		if err != nil {
			return err
		}
		if !created {
			return nil
		}
		id = reserved
	}
	if _, err := s.client.InsertTx(ctx, tx, PrepareMaterialArgs{OwnerID: owner, MaterialID: id}, &river.InsertOpts{
		Queue: Queue, MaxAttempts: maxPreparationAttempts, UniqueOpts: river.UniqueOpts{ByArgs: true},
	}); err != nil {
		return fmt.Errorf("enqueue study material preparation: %w", err)
	}
	return tx.Commit(ctx)
}
