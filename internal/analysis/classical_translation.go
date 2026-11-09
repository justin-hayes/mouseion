package analysis

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/justin-hayes/mouseion/internal/ancientgreek"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/riverqueue/river"
)

const (
	// ClassicalTranslationQueue is the separate queue for importing a pinned
	// reference translation after a Book's analysis has published. It is not
	// the preparation queue, so a slow or failing translation download can
	// never delay a Book.
	ClassicalTranslationQueue = "classical_translation"

	// classicalTranslationMaxAttempts bounds the import's own retries. A
	// permanent failure leaves the Book analyzed and deck preparation without
	// passage context; nothing learner-facing reports it.
	classicalTranslationMaxAttempts = 5

	classicalTranslationTimeout = 5 * time.Minute
)

// ClassicalTranslationArgs carries the exact reviewed translation pin, not a
// Book or owner, because the parsed rows are installation-wide. River's
// argument uniqueness therefore also deduplicates imports of the same file
// across Books and learners.
type ClassicalTranslationArgs struct {
	Layer         string `json:"layer" river:"unique"`
	FileName      string `json:"file_name" river:"unique"`
	Size          int64  `json:"size" river:"unique"`
	SHA256        string `json:"sha256" river:"unique"`
	CitationStart string `json:"citation_start" river:"unique"`
	CitationEnd   string `json:"citation_end" river:"unique"`
	Language      string `json:"language" river:"unique"`
}

// Kind implements river.JobArgs.
func (ClassicalTranslationArgs) Kind() string { return ClassicalTranslationQueue }

// ClassicalTranslationStore is the persistence the translation import needs.
// *persistence.PostgresStore implements it.
type ClassicalTranslationStore interface {
	HasReferenceTranslations(context.Context, string) (bool, error)
	PutReferenceTranslations(context.Context, string, string, map[string]string) (int, error)
}

// ClassicalTranslationWorker downloads one pinned translation file, verifies
// its reviewed size and SHA-256, parses its citation-aligned sections with the
// generalised parser and stores the rows once for the installation. It never
// stores the file itself.
type ClassicalTranslationWorker struct {
	river.WorkerDefaults[ClassicalTranslationArgs]

	Store  ClassicalTranslationStore
	Config ClassicalPreparationConfig
}

// Timeout bounds one translation import.
func (w *ClassicalTranslationWorker) Timeout(*river.Job[ClassicalTranslationArgs]) time.Duration {
	return classicalTranslationTimeout
}

// Work implements river.Worker. A pin whose rows already exist is a no-op, so
// a second Book or learner never downloads the same translation again.
func (w *ClassicalTranslationWorker) Work(ctx context.Context, job *river.Job[ClassicalTranslationArgs]) error {
	if w == nil || w.Store == nil {
		return errors.New("analysis: classical translation store is required")
	}
	args := job.Args
	if err := (classical.TranslationPin{
		Layer:         args.Layer,
		FileName:      args.FileName,
		Size:          args.Size,
		SHA256:        args.SHA256,
		CitationStart: args.CitationStart,
		CitationEnd:   args.CitationEnd,
	}).Validate(); err != nil {
		return err
	}
	if strings.TrimSpace(args.Language) == "" {
		return errors.New("analysis: classical translation language is required")
	}
	exists, err := w.Store.HasReferenceTranslations(ctx, args.SHA256)
	if err != nil {
		return err
	}
	if exists {
		logClassicalTranslation("complete", args, 0)
		return nil
	}
	data, err := w.download(ctx, args)
	if err != nil {
		return err
	}
	sections, err := ancientgreek.ParseReferenceTranslation(data, args.CitationStart, args.CitationEnd)
	if err != nil {
		return err
	}
	translations := make(map[string]string, len(sections))
	for _, section := range sections {
		if _, duplicate := translations[section.Citation]; duplicate {
			return &ancientgreek.ImportError{Code: ancientgreek.CodeMalformed, Detail: "duplicate cited section"}
		}
		translations[section.Citation] = section.Text
	}
	written, err := w.Store.PutReferenceTranslations(ctx, args.SHA256, args.Language, translations)
	if err != nil {
		return err
	}
	logClassicalTranslation("imported", args, written)
	return nil
}

// download fetches the pinned translation into the acquisition temp directory.
// The read is bounded by the reviewed size plus one, so a hostile response
// cannot exhaust memory, and the temporary file is always removed.
func (w *ClassicalTranslationWorker) download(ctx context.Context, args ClassicalTranslationArgs) ([]byte, error) {
	layer, ok := w.Config.Layers.Layer(args.Layer)
	if !ok {
		return nil, &ancientgreek.ImportError{Code: ancientgreek.CodeSourceLayerUnknown}
	}
	if args.Size > maxClassicalSourceBytes {
		return nil, &ancientgreek.ImportError{Code: ancientgreek.CodeOversize}
	}
	sourceURL, err := classicalSourceURL(layer.BaseURL, args.FileName)
	if err != nil {
		return nil, &ancientgreek.ImportError{Code: ancientgreek.CodeSourceLayerUnknown}
	}
	client := w.Config.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: classicalTranslationTimeout}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, sourceURL, nil)
	if err != nil {
		return nil, &ancientgreek.ImportError{Code: ancientgreek.CodeSourceDownloadFailed}
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, &ancientgreek.ImportError{Code: ancientgreek.CodeSourceDownloadFailed}
	}
	defer func() {
		if closeErr := response.Body.Close(); closeErr != nil {
			log.Printf("mouseion: close reference translation response: %v", closeErr)
		}
	}()
	if response.StatusCode != http.StatusOK {
		return nil, &ancientgreek.ImportError{Code: ancientgreek.CodeSourceDownloadFailed}
	}
	return stagePinnedSource(response.Body, args.Size, args.SHA256, w.Config.TempDir)
}

// AddClassicalTranslationWorker registers the translation import worker on a
// River worker set.
func AddClassicalTranslationWorker(workers *river.Workers, store ClassicalTranslationStore, config ClassicalPreparationConfig) {
	worker := &ClassicalTranslationWorker{Store: store, Config: config}
	if err := river.AddWorkerSafely(workers, worker); err != nil {
		log.Printf("mouseion: register classical translation worker: %v", err)
	}
}

// logClassicalTranslation records a state word, the pin and a count only. It
// never logs source text, a Work title or Greek.
func logClassicalTranslation(state string, args ClassicalTranslationArgs, sections int) {
	log.Printf("mouseion: classical translation %s pin=%s sections=%d", state, args.SHA256, sections)
}
