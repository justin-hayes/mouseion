package prepareddeck

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/txcleanup"
	"github.com/riverqueue/river"
)

type CustomDeckPreparationJobArgs struct {
	OwnerID, PreparationID string
}

func (CustomDeckPreparationJobArgs) Kind() string { return "custom_deck_preparation" }

type customDeckFrozenSpec struct {
	Presentation json.RawMessage                        `json:"presentation"`
	Evidence     []domain.CustomDeckPreparationEvidence `json:"evidence"`
	Omissions    []domain.CustomDeckPreparationOmission `json:"omissions"`
}

type CustomDeckPreparationService struct {
	store        *persistence.PostgresStore
	client       riverClient
	presentation *cardexport.Presentation
	provider     enrichment.TranslationProvider
}

func NewCustomDeckPreparationService(store *persistence.PostgresStore, client riverClient, presentation *cardexport.Presentation, provider enrichment.TranslationProvider) *CustomDeckPreparationService {
	return &CustomDeckPreparationService{store: store, client: client, presentation: presentation, provider: provider}
}

func (s *CustomDeckPreparationService) Submit(ctx context.Context, owner, deckID, actionKey, expectedEvidence string) (result domain.CustomDeckPreparation, err error) {
	if s == nil || s.store == nil || s.client == nil || s.presentation == nil || strings.TrimSpace(owner) == "" || strings.TrimSpace(deckID) == "" || strings.TrimSpace(actionKey) == "" {
		return domain.CustomDeckPreparation{}, ErrInvalidInput
	}
	tx, err := s.store.Pool().BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead})
	if err != nil {
		return domain.CustomDeckPreparation{}, err
	}
	defer func() { err = errors.Join(err, txcleanup.Rollback(ctx, tx)) }()
	if _, err = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1||':'||$2,0))`, owner, deckID); err != nil {
		return domain.CustomDeckPreparation{}, err
	}
	p, err := persistence.CreateCustomDeckPreparationTx(ctx, tx, owner, deckID, actionKey, expectedEvidence)
	if err != nil {
		return domain.CustomDeckPreparation{}, err
	}
	if len(p.FrozenSpec) == 0 {
		projections, _, deckName, evidence, omissions, buildErr := s.store.LoadCustomDeckProjectionsTx(ctx, tx, owner, p.DeckID, p.ID)
		if buildErr != nil {
			return domain.CustomDeckPreparation{}, buildErr
		}
		providerName, providerVersion := "unconfigured", "unconfigured"
		if s.provider != nil {
			providerName, providerVersion = s.provider.Name(), s.provider.Version()
		}
		for i := range projections {
			projections[i].Provider = providerName
			projections[i].ProviderVersion = providerVersion
			projections[i].TargetLanguage = "en"
			projections[i].RequireContextualGloss = true
		}
		deck, _, freezeErr := s.presentation.Freeze(ctx, owner, deckName, projections)
		if freezeErr != nil {
			return domain.CustomDeckPreparation{}, freezeErr
		}
		presentationJSON, marshalErr := json.Marshal(deck.StorageProjection())
		if marshalErr != nil {
			return domain.CustomDeckPreparation{}, marshalErr
		}
		frozen := customDeckFrozenSpec{Presentation: presentationJSON, Evidence: evidence, Omissions: omissions}
		encoded, marshalErr := json.Marshal(frozen)
		if marshalErr != nil {
			return domain.CustomDeckPreparation{}, marshalErr
		}
		if err = persistence.FreezeCustomDeckPreparationTx(ctx, tx, owner, p.ID, encoded); err != nil {
			return domain.CustomDeckPreparation{}, err
		}
		p.FrozenSpec = encoded
	}
	_, err = s.client.InsertTx(ctx, tx, CustomDeckPreparationJobArgs{OwnerID: owner, PreparationID: p.ID}, &river.InsertOpts{
		Queue: Queue, MaxAttempts: durableJobMaxAttempts,
		UniqueOpts: river.UniqueOpts{ByArgs: true, ByState: livePreparationJobStates},
	})
	if err != nil {
		return domain.CustomDeckPreparation{}, fmt.Errorf("enqueue Custom deck preparation: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return domain.CustomDeckPreparation{}, err
	}
	return p, nil
}

func (s *CustomDeckPreparationService) EvidenceFingerprint(ctx context.Context, owner, deckID string) (string, error) {
	if s == nil || s.store == nil || strings.TrimSpace(owner) == "" || strings.TrimSpace(deckID) == "" {
		return "", ErrInvalidInput
	}
	return s.store.CustomDeckEvidenceFingerprint(ctx, owner, deckID)
}

func (s *CustomDeckPreparationService) Get(ctx context.Context, owner, preparationID string) (domain.CustomDeckPreparation, error) {
	if s == nil || s.store == nil {
		return domain.CustomDeckPreparation{}, ErrInvalidInput
	}
	p, err := s.store.GetCustomDeckPreparation(ctx, owner, preparationID)
	return addCustomPreparationEvidence(p, err)
}

func (s *CustomDeckPreparationService) Latest(ctx context.Context, owner, deckID string) (domain.CustomDeckPreparation, error) {
	if s == nil || s.store == nil {
		return domain.CustomDeckPreparation{}, ErrInvalidInput
	}
	p, err := s.store.LatestCustomDeckPreparation(ctx, owner, deckID)
	return addCustomPreparationEvidence(p, err)
}

func (s *CustomDeckPreparationService) List(ctx context.Context, owner, deckID string) ([]domain.CustomDeckPreparation, error) {
	if s == nil || s.store == nil {
		return nil, ErrInvalidInput
	}
	preparations, err := s.store.ListCustomDeckPreparations(ctx, owner, deckID)
	if err != nil {
		return nil, err
	}
	for i := range preparations {
		preparations[i], err = addCustomPreparationEvidence(preparations[i], nil)
		if err != nil {
			return nil, err
		}
	}
	return preparations, nil
}

func (s *CustomDeckPreparationService) LatestReady(ctx context.Context, owner, deckID string) (domain.CustomDeckPreparation, error) {
	if s == nil || s.store == nil {
		return domain.CustomDeckPreparation{}, ErrInvalidInput
	}
	return s.store.LatestReadyCustomDeckPreparation(ctx, owner, deckID)
}

func addCustomPreparationEvidence(p domain.CustomDeckPreparation, err error) (domain.CustomDeckPreparation, error) {
	if err != nil || len(p.FrozenSpec) == 0 {
		return p, err
	}
	var frozen customDeckFrozenSpec
	if err := json.Unmarshal(p.FrozenSpec, &frozen); err != nil {
		return domain.CustomDeckPreparation{}, fmt.Errorf("decode frozen Custom deck evidence: %w", err)
	}
	p.Evidence = append(p.Evidence, frozen.Evidence...)
	return p, nil
}

func (s *CustomDeckPreparationService) Download(ctx context.Context, owner, preparationID string) (domain.CustomDeckPreparation, error) {
	if s == nil || s.store == nil {
		return domain.CustomDeckPreparation{}, ErrInvalidInput
	}
	return s.store.DownloadCustomDeckPreparation(ctx, owner, preparationID)
}

func (s *CustomDeckPreparationService) Cancel(ctx context.Context, owner, preparationID string) error {
	if s == nil || s.store == nil || strings.TrimSpace(owner) == "" || strings.TrimSpace(preparationID) == "" {
		return ErrInvalidInput
	}
	return s.store.CancelCustomDeckPreparation(ctx, owner, preparationID)
}

type CustomDeckPreparationWorker struct {
	river.WorkerDefaults[CustomDeckPreparationJobArgs]
	Store        *persistence.PostgresStore
	Presentation *cardexport.Presentation
	Provider     enrichment.TranslationProvider
	Configured   bool
}

func (w *CustomDeckPreparationWorker) Work(ctx context.Context, job *river.Job[CustomDeckPreparationJobArgs]) error {
	if w == nil || job == nil || w.Store == nil || w.Presentation == nil {
		return ErrInvalidInput
	}
	a := job.Args
	p, err := w.Store.ClaimCustomDeckPreparation(ctx, a.OwnerID, a.PreparationID)
	if err != nil {
		return err
	}
	if p.State == "ready" || p.State == "complete_with_omissions" || p.State == "failed" || p.State == "cancelled" {
		return nil
	}
	if !w.Configured || w.Provider == nil {
		return w.fail(ctx, a, errors.New("a configured translation provider is required for contextual Glosses"))
	}
	var frozen customDeckFrozenSpec
	if len(p.FrozenSpec) == 0 {
		return w.fail(ctx, a, errors.New("Custom deck preparation is missing its submission-time frozen specification"))
	}
	if err = json.Unmarshal(p.FrozenSpec, &frozen); err != nil {
		return w.fail(ctx, a, fmt.Errorf("decode frozen Custom deck: %w", err))
	}
	var projection cardexport.StorageProjection
	if err := json.Unmarshal(frozen.Presentation, &projection); err != nil {
		return w.fail(ctx, a, fmt.Errorf("decode frozen Custom deck presentation: %w", err))
	}
	deck, err := w.Presentation.Restore(projection)
	if err != nil {
		return w.fail(ctx, a, err)
	}
	work := deck.WorkProjection()
	results := make([]cardexport.StoredResult, 0, len(work))
	for _, item := range work {
		cancelled, cancelErr := w.Store.CustomDeckPreparationCancelled(ctx, a.OwnerID, a.PreparationID)
		if cancelErr != nil {
			return cancelErr
		}
		if cancelled {
			return nil
		}
		response, translateErr := w.Provider.Translate(ctx, item.Request)
		cancelled, cancelErr = w.Store.CustomDeckPreparationCancelled(ctx, a.OwnerID, a.PreparationID)
		if cancelErr != nil {
			return cancelErr
		}
		if cancelled {
			return nil
		}
		if translateErr != nil {
			if job.MaxAttempts > 0 && job.Attempt >= job.MaxAttempts {
				return w.fail(ctx, a, fmt.Errorf("translation failed after %d attempts", job.Attempt))
			}
			return translateErr // River retries the exact frozen inputs.
		}
		results = append(results, cardexport.StoredResult{CacheKey: item.CacheKey, Record: enrichment.CacheEntry{
			CacheKey: item.CacheKey, Translation: response.Translation, FallbackGloss: response.Gloss,
			SentenceTranslation: response.SentenceTranslation, SentenceTranslationTargets: append([]string(nil), response.SentenceTranslationTargets...),
			SenseSelection: append([]int(nil), response.SenseOrder...), CachedAt: time.Now().UTC(),
		}, OmissionReason: response.UnresolvedReason})
	}
	artifact, _, err := w.Presentation.Finalize(ctx, deck, results, cardexport.RunFacts{
		Consent: true, Configured: true, ExecutionMode: string(domain.PreparedDeckExecutionStandard),
		TargetLanguage: "en", Provider: w.Provider.Name(), ProviderVersion: w.Provider.Version(),
	})
	if err != nil {
		return w.fail(ctx, a, err)
	}
	if artifact.Completeness.TotalCards == 0 || len(artifact.APKG) == 0 {
		return w.fail(ctx, a, errors.New("preparation produced no exportable cards"))
	}
	for _, omitted := range deck.Diagnostics().QualityOmissions {
		frozen.Omissions = append(frozen.Omissions, domain.CustomDeckPreparationOmission{Kind: "quality", Lemma: omitted.CanonicalLemma, UPOS: omitted.UPOS, Reason: strings.Join(omitted.Reasons, ", ")})
	}
	for i, result := range results {
		if result.OmissionReason == "" || i >= len(work) {
			continue
		}
		identity := work[i].Request
		frozen.Omissions = append(frozen.Omissions, domain.CustomDeckPreparationOmission{Kind: "meaning", Lemma: identity.CanonicalLemma, UPOS: identity.UPOS, Reason: result.OmissionReason})
	}
	_, err = w.Store.CompleteCustomDeckPreparation(ctx, a.OwnerID, a.PreparationID, artifact.APKG, artifact.Completeness.TotalCards, frozen.Omissions)
	return err
}

func (w *CustomDeckPreparationWorker) fail(ctx context.Context, args CustomDeckPreparationJobArgs, cause error) error {
	log.Printf("custom deck preparation failed id=%s cause=%v", args.PreparationID, cause)
	if err := w.Store.FailCustomDeckPreparation(context.WithoutCancel(ctx), args.OwnerID, args.PreparationID, "Custom deck preparation failed; review the deck and retry."); err != nil && !errors.Is(err, persistence.ErrInvalidTransition) {
		return errors.Join(cause, err)
	}
	return nil
}

func AddCustomDeckPreparationWorker(workers *river.Workers, store *persistence.PostgresStore, presentation *cardexport.Presentation, provider enrichment.TranslationProvider, configured bool) {
	river.AddWorker(workers, &CustomDeckPreparationWorker{Store: store, Presentation: presentation, Provider: provider, Configured: configured})
}
