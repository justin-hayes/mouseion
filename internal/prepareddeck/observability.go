package prepareddeck

import (
	"encoding/json"
	"log"
	"time"

	"github.com/justin-hayes/mouseion/internal/cardexport"
)

// Observation is the privacy-safe aggregate emitted once for each prepared
// deck worker execution. Content and identity fields have no representation in
// this type, which prevents accidental logging of sentences, lemmas, titles,
// owner IDs, prompts, responses, credentials, or raw provider errors.
type Observation struct {
	Event                      string         `json:"event"`
	Outcome                    string         `json:"outcome"`
	JobAttempt                 int            `json:"job_attempt"`
	ConfiguredConcurrency      int            `json:"configured_concurrency"`
	EffectiveConcurrency       int            `json:"effective_concurrency"`
	PeakInFlightProviderCalls  int            `json:"peak_in_flight_provider_calls"`
	ArtifactDeterminismChecked bool           `json:"artifact_determinism_checked"`
	ArtifactDeterministic      bool           `json:"artifact_deterministic"`
	Durations                  PhaseDurations `json:"durations"`
	Counts                     OutcomeCounts  `json:"counts"`
	Errors                     ErrorCounts    `json:"errors"`
	Completeness               Completeness   `json:"completeness"`
}

type PhaseDurations struct {
	QueueWait    time.Duration `json:"queue_wait_ns"`
	Claim        time.Duration `json:"claim_ns"`
	InitialBuild time.Duration `json:"initial_build_ns"`
	Translation  time.Duration `json:"translation_ns"`
	FinalBuild   time.Duration `json:"final_build_ns"`
	Commit       time.Duration `json:"commit_ns"`
	Total        time.Duration `json:"total_preparation_ns"`
	Cache        time.Duration `json:"cache_ns"`
	Provider     time.Duration `json:"provider_ns"`
}

type OutcomeCounts struct {
	Selected            int `json:"selected"`
	Accepted            int `json:"accepted"`
	Omitted             int `json:"omitted"`
	TranslationEligible int `json:"translation_eligible"`
	Translated          int `json:"translated"`
	Untranslated        int `json:"untranslated"`
	CacheHits           int `json:"cache_hits"`
	CacheMisses         int `json:"cache_misses"`
	ProviderCalls       int `json:"provider_calls"`
	Attempts            int `json:"attempts"`
	Retries             int `json:"retries"`
	Cancellations       int `json:"cancellations"`
}

type ErrorCounts struct {
	RateLimit   int `json:"rate_limit"`
	Provider5xx int `json:"provider_5xx"`
	Timeout     int `json:"timeout"`
	Cache       int `json:"cache"`
	Build       int `json:"build"`
	Commit      int `json:"commit"`
	Other       int `json:"other"`
}

type Completeness struct {
	TotalCards                     int `json:"total_cards"`
	CardsWithEnglish               int `json:"cards_with_english"`
	CardsWithContextualTranslation int `json:"cards_with_contextual_translation"`
	QualityOmissions               int `json:"quality_omissions"`
}

func completenessOf(c cardexport.Completeness) Completeness {
	return Completeness{
		TotalCards:                     c.TotalCards,
		CardsWithEnglish:               c.CardsWithEnglish,
		CardsWithContextualTranslation: c.CardsWithEnglishSentence,
		QualityOmissions:               c.QualityOmitted,
	}
}

// Observer receives only the bounded aggregate above. Implementations must be
// fast and must not panic; worker correctness never depends on observation.
type Observer interface {
	Observe(Observation)
}

type ObserverFunc func(Observation)

func (f ObserverFunc) Observe(observation Observation) { f(observation) }

func observeSafely(observer Observer, observation Observation) {
	if observer == nil {
		return
	}
	defer func() { _ = recover() }()
	observer.Observe(observation)
}

type printfLogger interface {
	Printf(string, ...any)
}

// LogObserver writes one machine-readable JSON record through the repository's
// existing logger. Durations use nanoseconds, matching time.Duration.
type LogObserver struct{ Logger printfLogger }

func NewLogObserver(logger printfLogger) LogObserver {
	if logger == nil {
		logger = log.Default()
	}
	return LogObserver{Logger: logger}
}

func (o LogObserver) Observe(observation Observation) {
	encoded, err := json.Marshal(observation)
	if err != nil {
		return
	}
	o.Logger.Printf("mouseion: prepared_deck_observation %s", encoded)
}
