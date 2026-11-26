// Command studyevidencerun measures the source-evidence Study pass on real
// words (ADR 0138 §6). It runs the production Study material worker once per
// word, in Study's new-word order for one learner, and writes a Markdown report
// with an empty judgement column for the learner to fill in.
//
// It is a prototype and never merges. Three guards keep it harmless:
//
//   - It reads MOUSEION_EVIDENCE_DATABASE_URL only, never MOUSEION_DATABASE_URL,
//     so it is pointed at a restored copy, not the live database.
//   - Its store records nothing: every write the worker makes is captured in
//     memory and reported.
//   - It sends nothing unless ProviderAssessments().MaySend accepts the
//     compiled codec for the language, and stops when it does not.
//
// MOUSEION_LLM_* and MOUSEION_DICTIONARY_INDEX are read at run time. The API key
// is never printed or written.
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/justin-hayes/mouseion/internal/cardpolicy"
	"github.com/justin-hayes/mouseion/internal/dictionary"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/persistence"
	"github.com/justin-hayes/mouseion/internal/study"
	"github.com/riverqueue/river"
	"github.com/riverqueue/river/rivertype"
)

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout); err != nil {
		log.Fatal(err)
	}
}

// outcome names are the report's own. A meaning omission carries a sub-reason.
const (
	outcomeReady     = "ready"
	outcomeMeaning   = "meaning omission"
	outcomeNoEntry   = "no-dictionary-entry"
	outcomeSentence  = "sentence-quality omission"
	outcomeWaiting   = "waiting (worker snoozed)"
	outcomeFailed    = "provider or store failure"
	maxResponseBytes = 4 << 20
)

type wordResult struct {
	Identity  domain.StudyIdentity
	Outcome   string
	SubReason string
	Candidate enrichment.Candidate
	Passage   string
	Meaning   enrichment.EvidenceAssistedMeaning
	Ready     *domain.StudyMaterialReadyContent
	// Candidates are the evidence IDs and glosses the request sent, in order.
	Candidates []enrichment.LexicalSense
	// Invented are handles the answer cited that were not in the candidate set.
	Invented []string
	Failure  string
	Calls    int64
}

func run(ctx context.Context, args []string, out io.Writer) error {
	flags := flag.NewFlagSet("studyevidencerun", flag.ContinueOnError)
	language := flags.String("language", "grc", "study language")
	count := flags.Int("n", 50, "number of words, in Study's new-word order")
	reportPath := flags.String("out", "", "Markdown report path (required unless -dry)")
	dry := flags.Bool("dry", false, "list the words and check the gate; send nothing")
	migrate := flags.Bool("migrate", false, "bring the restored copy up to this commit's schema, then continue")
	owner := flags.String("owner", "", "learner ID; defaults to the only learner with Books in the language")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if !*dry && *reportPath == "" {
		return errors.New("-out is required unless -dry")
	}
	databaseURL := strings.TrimSpace(os.Getenv("MOUSEION_EVIDENCE_DATABASE_URL"))
	if databaseURL == "" {
		return errors.New("MOUSEION_EVIDENCE_DATABASE_URL must name a restored copy of the database")
	}
	if *migrate {
		if err := persistence.Migrate(databaseURL); err != nil {
			return err
		}
	}
	store, err := persistence.Open(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()

	policy := cardpolicy.For(*language)
	if !policy.SourceEvidence {
		return fmt.Errorf("language %q is not a source-evidence language", *language)
	}
	cfg, err := enrichment.LLMConfigFromEnv()
	if err != nil {
		return err
	}
	if !cfg.Enabled {
		return errors.New("the provider is not enabled (MOUSEION_LLM_ENABLED)")
	}
	codec, err := enrichment.NewTranslationCodec(cfg)
	if err != nil {
		return err
	}
	assessments := enrichment.ProviderAssessments()
	if err := assessments.MaySend(enrichment.EvidenceAssistedPassKind(*language), *language, codec); err != nil {
		return fmt.Errorf("the gate refuses the %s pass payload; nothing sent: %w", *language, err)
	}
	index, err := dictionary.OpenIndex(ctx, os.Getenv("MOUSEION_DICTIONARY_INDEX"))
	if err != nil {
		return err
	}
	defer func() { _ = index.Close() }()

	learner, err := resolveOwner(ctx, store, *owner, *language)
	if err != nil {
		return err
	}
	words, err := newWords(ctx, store, learner, *language, *count)
	if err != nil {
		return err
	}
	_, _ = fmt.Fprintf(out, "owner %s: %d words, gate open, dictionary pin %s\n", learner, len(words), index.Version())
	if *dry {
		for i, word := range words {
			_, _ = fmt.Fprintf(out, "%2d %s (%s)\n", i+1, word.CanonicalLemma, word.UPOS)
		}
		return nil
	}

	transport := &countingTransport{base: http.DefaultTransport}
	provider, err := enrichment.NewConfiguredLLMProvider(cfg, &http.Client{Transport: transport, Timeout: cfg.Timeout})
	if err != nil {
		return err
	}
	service := enrichment.NewService(enrichment.Config{ExternalEnabled: true, UserOptIn: true, MaxAttempts: 3}, nil, nil, nil, provider, nil)
	capture := &captureEnricher{Service: service}
	evidence := &study.SourceEvidence{
		Lexical:      index,
		Translations: store,
		MaySend: func(language string) error {
			return assessments.MaySend(enrichment.EvidenceAssistedPassKind(language), language, codec)
		},
		Available: func(policy cardpolicy.Policy) bool {
			return policy.Entries == cardpolicy.EntryLSJ && index.AncientGreekLSJAvailable()
		},
	}
	started := time.Now()
	var results []wordResult
	for _, word := range words {
		result, err := runWord(ctx, store, capture, transport, evidence, learner, word, policy)
		if err != nil {
			return err
		}
		results = append(results, result)
		_, _ = fmt.Fprintf(out, "%s: %s %s\n", word.CanonicalLemma, result.Outcome, result.SubReason)
		if result.Outcome == outcomeWaiting {
			return fmt.Errorf("the worker snoozed on %q: the gate, dictionary or provider is not ready; stopping", word.CanonicalLemma)
		}
	}
	meta := reportMeta{
		Owner: learner, Language: *language, Date: started.UTC().Format("2006-01-02"),
		Model: cfg.Model, Reasoning: cfg.ReasoningEffort,
		ProviderVersion: codec.EvidenceAssistedPassProviderVersion(),
		DictionaryPin:   index.Version(),
		Calls:           transport.calls.Load(),
	}
	if err := pinsOf(ctx, store, results, &meta); err != nil {
		return err
	}
	return os.WriteFile(*reportPath, []byte(renderReport(meta, results)), 0o600)
}

// resolveOwner finds the learner. It stops when the choice is ambiguous.
func resolveOwner(ctx context.Context, store *persistence.PostgresStore, given, language string) (string, error) {
	if given != "" {
		return given, nil
	}
	rows, err := store.Pool().Query(ctx, `SELECT DISTINCT owner_id::text FROM books WHERE language_tag = $1`, language)
	if err != nil {
		return "", err
	}
	defer rows.Close()
	var owners []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return "", err
		}
		owners = append(owners, id)
	}
	if err := rows.Err(); err != nil {
		return "", err
	}
	if len(owners) != 1 {
		return "", fmt.Errorf("%d learners have %s Books; pass -owner", len(owners), language)
	}
	return owners[0], nil
}

// newWords is Study's new-word order exactly as PlanPreparation derives it,
// cut at n.
func newWords(ctx context.Context, store *persistence.PostgresStore, owner, language string, n int) ([]domain.StudyIdentity, error) {
	candidates, err := store.ListStudyCandidates(ctx, owner, language)
	if err != nil {
		return nil, err
	}
	progress, err := store.ListStudyProgress(ctx, owner, language)
	if err != nil {
		return nil, err
	}
	reports, err := store.ListOpenStudyReports(ctx, owner, language, domain.StudyReportWrongWord)
	if err != nil {
		return nil, err
	}
	ordered := domain.OrderStudyStream(domain.WithoutStudyProgress(candidates, progress, domain.StudyHeldIdentities(reports)))
	var words []domain.StudyIdentity
	for _, candidate := range ordered {
		if len(words) == n {
			break
		}
		words = append(words, candidate.StudyIdentity)
	}
	return words, nil
}

// runWord runs the production worker for one word against a recording store.
func runWord(ctx context.Context, store *persistence.PostgresStore, enricher *captureEnricher, transport *countingTransport, evidence *study.SourceEvidence, owner string, identity domain.StudyIdentity, policy cardpolicy.Policy) (wordResult, error) {
	result := wordResult{Identity: identity}
	recording := &recordingStore{PostgresStore: store, material: domain.StudyMaterial{
		ID: "evidence-run", OwnerID: owner, StudyIdentity: identity, State: domain.StudyMaterialPending,
	}}
	enricher.reset()
	before := transport.calls.Load()
	worker := &study.MaterialWorker{Store: recording, Enricher: enricher, Evidence: evidence}
	err := worker.Work(ctx, &river.Job[study.PrepareMaterialArgs]{
		JobRow: &rivertype.JobRow{},
		Args:   study.PrepareMaterialArgs{OwnerID: owner, MaterialID: "evidence-run"},
	})
	result.Calls = transport.calls.Load() - before
	result.Candidate, result.Passage, result.Meaning = enricher.candidate, enricher.passage, enricher.meaning
	result.Candidates = enricher.candidate.CandidateSenses
	var snooze *river.JobSnoozeError
	switch {
	case errors.As(err, &snooze):
		result.Outcome = outcomeWaiting
	case err != nil:
		result.Outcome, result.Failure = outcomeFailed, err.Error()
	case recording.ready != nil:
		result.Outcome, result.Ready = outcomeReady, recording.ready
	default:
		classify(&result, recording.omission, enricher, transport)
	}
	result.Invented = inventedHandles(result, enricher, transport)
	return result, nil
}

// classify names an omission and, for a meaning omission, its sub-reason.
func classify(result *wordResult, reason string, enricher *captureEnricher, _ *countingTransport) {
	switch reason {
	case domain.StudyOmissionNoDictionaryEntry:
		result.Outcome = outcomeNoEntry
	case domain.StudyOmissionSentenceQuality:
		result.Outcome = outcomeSentence
	case domain.StudyOmissionMeaning:
		result.Outcome = outcomeMeaning
		m := result.Meaning
		switch {
		case !enricher.called:
			result.SubReason = "the entry has no citable sense"
		case m.InvalidCitation:
			result.SubReason = "invalid citation (a handle outside the candidate set, or one cited twice)"
		case m.ContextOnly && strings.TrimSpace(m.Gloss) == "":
			result.SubReason = "abstention"
			if m.UnresolvedReason != "" {
				result.SubReason += ": " + m.UnresolvedReason
			}
		case m.ContextOnly:
			result.SubReason = "context-only Gloss, no cited sense"
		case strings.TrimSpace(m.Gloss) == "":
			result.SubReason = "empty Gloss"
		case strings.TrimSpace(m.SentenceTranslation) == "":
			result.SubReason = "empty sentence translation"
		default:
			result.SubReason = "cited sense the resolved entries do not hold"
		}
	default:
		result.Outcome = "omission " + reason
	}
}

// inventedHandles lists the handles a pass answer cited that the request's
// candidate set did not hold. The codec keeps no handle of an invalid
// citation, so they are read back from the raw provider answer.
func inventedHandles(result wordResult, enricher *captureEnricher, transport *countingTransport) []string {
	known := map[string]bool{}
	for _, sense := range result.Candidates {
		known[sense.EvidenceID] = true
	}
	var invented []string
	for _, id := range result.Meaning.CitedSenseIDs {
		if !known[id] && !slices.Contains(invented, id) {
			invented = append(invented, id)
		}
	}
	if result.Meaning.InvalidCitation {
		count := 0
		seen := map[string]bool{}
		for _, sense := range result.Candidates {
			if !seen[sense.EvidenceID] {
				seen[sense.EvidenceID] = true
				count++
			}
		}
		for _, handle := range transport.lastHandles() {
			var n int
			if _, err := fmt.Sscanf(handle, "e%d", &n); err != nil || n < 1 || n > count || fmt.Sprintf("e%d", n) != handle {
				if !slices.Contains(invented, handle) {
					invented = append(invented, handle)
				}
			}
		}
		if len(invented) == 0 {
			invented = append(invented, "(duplicate handle; no unknown handle in the answer)")
		}
	}
	return invented
}

// captureEnricher is the production service plus a record of the last pass.
type captureEnricher struct {
	*enrichment.Service
	called    bool
	candidate enrichment.Candidate
	passage   string
	meaning   enrichment.EvidenceAssistedMeaning
}

func (c *captureEnricher) reset() {
	c.called, c.candidate, c.passage, c.meaning = false, enrichment.Candidate{}, "", enrichment.EvidenceAssistedMeaning{}
}

func (c *captureEnricher) EnrichEvidenceAssistedPass(ctx context.Context, candidate enrichment.Candidate, passageContext string) (enrichment.EvidenceAssistedMeaning, error) {
	c.called, c.candidate, c.passage = true, candidate, passageContext
	meaning, err := c.Service.EnrichEvidenceAssistedPass(ctx, candidate, passageContext)
	c.meaning = meaning
	return meaning, err
}

// recordingStore reads the restored database and records what the worker
// would have written. Nothing it holds writes.
type recordingStore struct {
	*persistence.PostgresStore
	material domain.StudyMaterial
	ready    *domain.StudyMaterialReadyContent
	omission string
}

func (s *recordingStore) GetStudyMaterial(context.Context, string, string) (domain.StudyMaterial, error) {
	return s.material, nil
}

func (s *recordingStore) RecordStudyMaterialReady(_ context.Context, _, _ string, content domain.StudyMaterialReadyContent) error {
	s.ready = &content
	return nil
}

func (s *recordingStore) RecordStudyMaterialUnpreparable(_ context.Context, _, _, reason string) error {
	s.omission = reason
	return nil
}

func (s *recordingStore) SetStudyMaterialEvidence(context.Context, string, string, string) error {
	return nil
}

// countingTransport counts provider requests and keeps the evidence handles of
// the last answer. It stores no request, header or key.
type countingTransport struct {
	base  http.RoundTripper
	calls atomic.Int64
	last  atomic.Value // []string
}

var handlePattern = regexp.MustCompile(`evidence_ids\\*"\s*:\s*\[([^\]]*)\]`)
var quotedPattern = regexp.MustCompile(`[A-Za-z0-9_.:-]+`)

func (t *countingTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	t.calls.Add(1)
	response, err := t.base.RoundTrip(request)
	if err != nil {
		return response, err
	}
	body, readErr := io.ReadAll(io.LimitReader(response.Body, maxResponseBytes))
	_ = response.Body.Close()
	if readErr != nil {
		return nil, readErr
	}
	var handles []string
	if match := handlePattern.FindSubmatch(body); match != nil {
		handles = quotedPattern.FindAllString(string(match[1]), -1)
	}
	t.last.Store(handles)
	response.Body = io.NopCloser(bytes.NewReader(body))
	return response, nil
}

func (t *countingTransport) lastHandles() []string {
	handles, _ := t.last.Load().([]string)
	return handles
}

// pinsOf reads the Books, analysis runs and translation pins the ready rows
// came from.
func pinsOf(ctx context.Context, store *persistence.PostgresStore, results []wordResult, meta *reportMeta) error {
	books, runs := map[string]bool{}, map[string]bool{}
	for _, r := range results {
		if r.Ready == nil {
			continue
		}
		books[r.Ready.BookID], runs[r.Ready.AnalysisRunID] = true, true
		if pin := r.Ready.Passage.TranslationPin; pin != "" && !slices.Contains(meta.TranslationPins, pin) {
			meta.TranslationPins = append(meta.TranslationPins, pin)
		}
	}
	for id := range books {
		var title, author string
		if err := store.Pool().QueryRow(ctx, `SELECT title, author FROM books WHERE id = $1`, id).Scan(&title, &author); err != nil {
			return err
		}
		meta.BookList = append(meta.BookList, strings.TrimSpace(author+", "+title))
	}
	for id := range runs {
		var name, version string
		if err := store.Pool().QueryRow(ctx, `SELECT analyzer_name, analyzer_version FROM analysis_runs WHERE id = $1`, id).Scan(&name, &version); err != nil {
			return err
		}
		if pin := name + " " + version; !slices.Contains(meta.Analyzers, pin) {
			meta.Analyzers = append(meta.Analyzers, pin)
		}
	}
	slices.Sort(meta.BookList)
	slices.Sort(meta.TranslationPins)
	slices.Sort(meta.Analyzers)
	return nil
}
