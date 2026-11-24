package domain

import (
	"errors"
	"strings"
	"time"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
)

const (
	LanguageUnknown                 = "unknown"
	LanguageChosen                  = "chosen"
	MetadataProvenanceCatalogueSync = "catalog_sync"
	MetadataProvenanceAcquisition   = "acquisition"
	AliasCatalogEntry               = "catalog_entry"
	AliasStrongBibliographic        = "strong_bibliographic"
	NamespaceSourceIdentifier       = "source_identifier"
	MetadataProvenanceBackfill      = "source_materials_backfill"
	// MetadataProvenanceOperatorRegistration marks a Book added for a classical
	// Reading unit, whose analysis comes from an import rather than EPUB
	// acquisition. The stored value predates the Classical catalog and is kept
	// for existing rows.
	MetadataProvenanceOperatorRegistration = "operator_registration"
	BookCoverNone                          = "none"
	BookCoverPending                       = "pending"
	BookCoverAvailable                     = "available"
	BookCoverUnavailable                   = "unavailable"
)

type CatalogueSyncState string

const (
	CatalogueSyncSyncing CatalogueSyncState = "syncing"
	CatalogueSyncSynced  CatalogueSyncState = "synced"
	CatalogueSyncFailed  CatalogueSyncState = "failed"
)

type CatalogueSyncStatus struct {
	OwnerID, ConnectionID string
	State                 CatalogueSyncState
	LastError             string
	LastSyncedAt          *time.Time
	// LastUpsertedCount is the number of linked Books the last sync updated.
	// The persisted column keeps its historical upsert name.
	LastUpsertedCount int
	UpdatedAt         time.Time
}

type BookEvidenceState string

const (
	BookUnavailable        BookEvidenceState = "unavailable"
	BookNotAcquired        BookEvidenceState = "not_acquired"
	BookAcquiredUnassessed BookEvidenceState = "acquired_unassessed"
	BookAnalyzed           BookEvidenceState = "analyzed"
	BookStale              BookEvidenceState = "stale"
)

type BookDisposition string

// MyBookBucket is the one visible workflow role a Book occupies in My Books.
// It is derived from current-reading status, persisted disposition, and history.
type MyBookBucket string

const (
	MyBookBucketNone           MyBookBucket = ""
	MyBookBucketCurrentReading MyBookBucket = "current_reading"
	MyBookBucketToRead         MyBookBucket = "to_read"
	MyBookBucketRead           MyBookBucket = "read"
)

type ReadingCompletionSource string

const (
	BookDispositionInbox  BookDisposition = "inbox"
	BookDispositionToRead BookDisposition = "to_read"
)

const (
	ReadingCompletionCurrentReading ReadingCompletionSource = "primary_goal"
	ReadingCompletionPreviouslyRead ReadingCompletionSource = "previously_read_import"
)

type Book struct {
	ID, OwnerID, Title, Author, MetadataProvenance, LanguageState, LanguageTag string
	CreatedAt, UpdatedAt                                                       time.Time
}

type BookAlias struct {
	ID, OwnerID, BookID, ConnectionID, AliasType, Namespace, Value string
	CreatedAt                                                      time.Time
}

// BookCover is the byte-free cover projection used by collection views.
type BookCover struct {
	State  string
	Width  int
	Height int
}

// BookCoverResource is the owner-scoped stored resource read by the cover
// endpoint. It is deliberately not embedded in MyBook or any collection query.
type BookCoverResource struct {
	OwnerID, BookID, MediaType, ContentHash string
	Bytes                                   []byte
	Width, Height                           int
}

// BookCoverRetrieval is the owner-scoped cover state read by the retrieval
// worker before it fetches anything. It names the selected Catalog entry so the
// worker can reload the current entry without persisting URLs or credentials.
type BookCoverRetrieval struct {
	State                    string
	SelectedConnectionID     string
	SelectedSourceIdentifier string
	FailureReason            string
	AdvertisedAt             time.Time
}

// MyBook is the complete learner-facing My Books read model. Acquired is nil
// for metadata-only membership; when present it contains only the current
// owner-scoped acquired source and its derived analysis state.
type MyBook struct {
	Book                 Book
	Cover                BookCover
	ReadingUnit          *BookReadingUnit
	ClassicalPreparation *ClassicalPreparationStatus
	EPUBAcquisition      *EPUBAcquisitionStatus
	Acquired             *SourceMaterialSummary
	Disposition          BookDisposition
	DispositionRevision  int64
	IsToRead             bool
	IsCurrentReading     bool
	// EverCurrentReading reports that the Book has held the Current reading
	// role at least once. A released snapshot proves it after the reading ends.
	EverCurrentReading     bool
	CompletionCount        int
	LatestCompletionAt     *time.Time
	LatestCompletionSource ReadingCompletionSource
	CoverageKnownTokens    int64
	CoverageTotalTokens    int64
	DeckState              string
	DeckCardCount          int64
	DeckPreparedAt         *time.Time
}

// WorkflowBucket derives the visible My Books role. A current reading keeps
// its persisted To Read disposition and appears in the To Read browse tab.
// Every Book with active membership is Currently reading, To Read or Read; an
// active member that is neither To Read nor has reading history has no visible
// role and is reported by ValidateMembership.
func (m MyBook) WorkflowBucket() MyBookBucket {
	bucket, _ := m.ChosenBucket()
	return bucket
}

// ChosenBucket derives the visible role and reports whether the Book has one.
// The second result is false for the retired Inbox state: an active member that
// is neither To Read nor has reading history.
func (m MyBook) ChosenBucket() (MyBookBucket, bool) {
	if m.IsCurrentReading {
		return MyBookBucketCurrentReading, true
	}
	disposition := m.Disposition
	if disposition == "" {
		if m.IsToRead {
			disposition = BookDispositionToRead
		} else {
			disposition = BookDispositionInbox
		}
	}
	switch disposition {
	case BookDispositionToRead:
		return MyBookBucketToRead, true
	case BookDispositionInbox:
		if m.CompletionCount > 0 {
			return MyBookBucketRead, true
		}
		return MyBookBucketNone, false
	default:
		return MyBookBucketNone, false
	}
}

// ValidateMembership rejects a Book with active My Books membership that has
// no visible role. Before the Inbox retirement that state was displayed as an
// Inbox Book; My Books now holds only chosen Books.
func (m MyBook) ValidateMembership() error {
	if _, ok := m.ChosenBucket(); !ok {
		return ErrBookNotChosen
	}
	return nil
}

// CanMarkPreviouslyRead reports whether My Books offers the "Mark as previously
// read" action. Only an unstarted To Read Book that has never held the Current
// reading role qualifies: marking a started or finished Book would rewrite a
// reading the learner already recorded.
func (m MyBook) CanMarkPreviouslyRead() bool {
	return m.WorkflowBucket() == MyBookBucketToRead &&
		m.CompletionCount == 0 &&
		!m.EverCurrentReading &&
		m.Book.LanguageState == LanguageChosen
}

// MatchesBrowseFilter reports whether a visible bucket belongs in the selected
// My Books tab. The retired Read-history and Inbox filters are no longer
// selectable; an empty disposition and no history match every visible role.
func (b MyBookBucket) MatchesBrowseFilter(disposition BookDisposition, readHistory bool) bool {
	if readHistory && b != MyBookBucketRead {
		return false
	}
	if disposition == "" {
		return b != MyBookBucketNone
	}
	switch disposition {
	case BookDispositionToRead:
		return b == MyBookBucketToRead || b == MyBookBucketCurrentReading
	default:
		return false
	}
}

// PersistedDisposition reports the underlying disposition for disposition
// buckets. Current reading and Read are derived roles without such a value.
func (b MyBookBucket) PersistedDisposition() (BookDisposition, bool) {
	switch b {
	case MyBookBucketToRead:
		return BookDispositionToRead, true
	case MyBookBucketCurrentReading, MyBookBucketRead:
		return "", false
	default:
		return "", false
	}
}

// Classification classifies this Book's Analysis evidence with its disposition
// and chosen language, so eligibility reflects the learner's Book. A Book with
// no acquired source has no signals.
func (m MyBook) Classification() BookEvidenceClassification {
	var signals AnalysisSignals
	if m.Acquired != nil {
		signals = m.Acquired.Signals
		signals.PipelineOutdated = m.Acquired.AnalysisUpdate != nil
	}
	signals.CurrentReading = m.IsCurrentReading
	if m.ReadingUnit != nil {
		signals.ClassicalPreparation = ClassicalPreparationSignalFor(m.ClassicalPreparation, m.ReadingUnit)
	}
	if m.EPUBAcquisition != nil {
		signals.EPUBAcquisition = m.EPUBAcquisition
	}
	language := ""
	if m.Book.LanguageState == LanguageChosen {
		language = m.Book.LanguageTag
	}
	return ClassifyBookEvidence(signals, m.Disposition, language)
}

// EvidenceState classifies the acquired evidence shown in My Books.
func (m MyBook) EvidenceState() BookEvidenceState {
	return m.Classification().Evidence
}

// AwaitingPreparation reports a classical Book whose imported analysis is not
// yet in effect. Ordinary catalogue and acquisition Books are never awaiting
// preparation, and a classical Book stops awaiting preparation once its
// current analysis is published.
func (m MyBook) AwaitingPreparation() bool {
	return m.ReadingUnit != nil && !m.Classification().AnalysisInEffect()
}

func (d BookDisposition) Validate() error {
	switch d {
	case BookDispositionInbox, BookDispositionToRead:
		return nil
	default:
		return errors.New("domain: invalid book disposition")
	}
}

func NewBook(ownerID, title, metadataProvenance, languageState, languageTag string) (Book, error) {
	if languageState == LanguageChosen {
		languageTag = canonicalization.NormalizeLanguage(languageTag)
	}
	b := Book{OwnerID: ownerID, Title: title, MetadataProvenance: metadataProvenance, LanguageState: languageState, LanguageTag: languageTag}
	return b, b.Validate()
}

func (b Book) Validate() error {
	if strings.TrimSpace(b.OwnerID) == "" {
		return errors.New("domain: book owner is required")
	}
	if strings.TrimSpace(b.Title) == "" {
		return errors.New("domain: book title is required")
	}
	if strings.TrimSpace(b.MetadataProvenance) == "" {
		return errors.New("domain: book metadata provenance is required")
	}
	switch b.MetadataProvenance {
	case MetadataProvenanceCatalogueSync, MetadataProvenanceAcquisition, MetadataProvenanceBackfill, MetadataProvenanceOperatorRegistration:
	default:
		return errors.New("domain: invalid book metadata provenance")
	}
	switch b.LanguageState {
	case LanguageUnknown:
		if b.LanguageTag != "" {
			return errors.New("domain: unknown book language cannot have a tag")
		}
	case LanguageChosen:
		if strings.TrimSpace(b.LanguageTag) == "" {
			return errors.New("domain: chosen book language requires a tag")
		}
		if b.LanguageTag != canonicalization.NormalizeLanguage(b.LanguageTag) {
			return errors.New("domain: chosen book language must be canonical")
		}
	default:
		return errors.New("domain: invalid book language state")
	}
	return nil
}

func (a BookAlias) Validate() error {
	if strings.TrimSpace(a.OwnerID) == "" || strings.TrimSpace(a.BookID) == "" {
		return errors.New("domain: book alias identity is required")
	}
	if a.AliasType != AliasCatalogEntry && a.AliasType != AliasStrongBibliographic {
		return errors.New("domain: invalid book alias type")
	}
	if strings.TrimSpace(a.Namespace) == "" || strings.TrimSpace(a.Value) == "" {
		return errors.New("domain: book alias namespace and value are required")
	}
	return nil
}
