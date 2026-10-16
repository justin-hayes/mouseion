// Command greekhitrate reports how many of the installation's Ancient Greek
// vocabulary identities have at least one Wiktionary dictionary entry. It
// prints counts only and never any Greek text.
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"sort"
	"strings"

	"github.com/justin-hayes/mouseion/internal/dictionary"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

const usage = "usage: greekhitrate"

// identityStore is the persistence the command needs, plus the lifecycle run
// closes so tests can supply a double.
type identityStore interface {
	ListAncientGreekVocabularyIdentities(context.Context) ([]persistence.AncientGreekVocabularyIdentity, error)
	Close() error
}

// dictionaryIndex is the lexical-provider seam the report reads.
type dictionaryIndex interface {
	Lookup(context.Context, enrichment.LexicalLookupRequest) (enrichment.LexicalEntry, bool, error)
	AncientGreekAvailable() bool
}

func main() {
	if err := run(context.Background(), os.Args[1:], os.Stdout, openStore, openIndex); err != nil {
		log.Fatal(err)
	}
}

func openStore(ctx context.Context, databaseURL string) (identityStore, error) {
	return persistence.Open(ctx, databaseURL)
}

func openIndex(ctx context.Context, path string) (dictionaryIndex, error) {
	return dictionary.OpenIndex(ctx, path)
}

// run lists the installation's Ancient Greek vocabulary identities and counts
// how many resolve to at least one dictionary entry. It prints counts only.
func run(ctx context.Context, args []string, output io.Writer, open identityStoreOpener, openDict dictionaryIndexOpener) (err error) {
	if len(args) != 0 {
		return errors.New(usage)
	}
	databaseURL := strings.TrimSpace(os.Getenv("MOUSEION_DATABASE_URL"))
	if databaseURL == "" {
		return errors.New("MOUSEION_DATABASE_URL is required")
	}
	indexPath := strings.TrimSpace(os.Getenv("MOUSEION_DICTIONARY_INDEX"))
	if indexPath == "" {
		return errors.New("MOUSEION_DICTIONARY_INDEX is required")
	}
	store, err := open(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, store.Close()) }()
	index, err := openDict(ctx, indexPath)
	if err != nil {
		return err
	}
	if !index.AncientGreekAvailable() {
		return errors.New("greekhitrate: the dictionary index carries no Ancient Greek entries; rebuild it with `make dictionary-index`")
	}
	identities, err := store.ListAncientGreekVocabularyIdentities(ctx)
	if err != nil {
		return err
	}
	report, err := summarize(ctx, identities, index)
	if err != nil {
		return err
	}
	return writeReport(output, report)
}

type identityStoreOpener func(context.Context, string) (identityStore, error)
type dictionaryIndexOpener func(context.Context, string) (dictionaryIndex, error)

// hitRateReport is the counts-only summary of one run.
type hitRateReport struct {
	Total     int
	WithEntry int
	ByUPOS    []uposHitRate
}

type uposHitRate struct {
	UPOS      string
	Total     int
	WithEntry int
}

// summarize counts the identities that resolve to at least one entry, overall
// and per part of speech. The discriminator never reaches the lookup, so the
// two homographs of one lemma share whatever evidence the lemma and POS carry.
func summarize(ctx context.Context, identities []persistence.AncientGreekVocabularyIdentity, index dictionaryIndex) (hitRateReport, error) {
	report := hitRateReport{}
	buckets := make(map[string]*uposHitRate)
	for _, identity := range identities {
		_, found, err := index.Lookup(ctx, enrichment.LexicalLookupRequest{Language: "grc", CanonicalLemma: identity.CanonicalLemma, UPOS: identity.UPOS})
		if err != nil {
			return hitRateReport{}, err
		}
		report.Total++
		bucket, ok := buckets[identity.UPOS]
		if !ok {
			bucket = &uposHitRate{UPOS: identity.UPOS}
			buckets[identity.UPOS] = bucket
		}
		bucket.Total++
		if found {
			report.WithEntry++
			bucket.WithEntry++
		}
	}
	ordered := make([]string, 0, len(buckets))
	for upos := range buckets {
		ordered = append(ordered, upos)
	}
	sort.Strings(ordered)
	for _, upos := range ordered {
		report.ByUPOS = append(report.ByUPOS, *buckets[upos])
	}
	return report, nil
}

// writeReport prints the summary and per-part-of-speech counts, never any
// stored text.
func writeReport(output io.Writer, report hitRateReport) error {
	if _, err := fmt.Fprintf(output, "greek dictionary hit rate: identities=%d with_entry=%d without_entry=%d\n", report.Total, report.WithEntry, report.Total-report.WithEntry); err != nil {
		return err
	}
	for _, bucket := range report.ByUPOS {
		if _, err := fmt.Fprintf(output, "greek dictionary hit rate by upos: upos=%s identities=%d with_entry=%d\n", bucket.UPOS, bucket.Total, bucket.WithEntry); err != nil {
			return err
		}
	}
	return nil
}
