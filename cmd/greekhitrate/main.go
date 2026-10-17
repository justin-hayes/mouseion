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

// dictionaryIndex is the lexical-provider seam the report reads. Lookup is the
// Wiktionary (Kaikki) hit rate; LookupAncientGreekLSJ is the unambiguous LSJ
// match the ticket requires, which never equates an OGA morpheus number with
// LSJ numbering.
type dictionaryIndex interface {
	Lookup(context.Context, enrichment.LexicalLookupRequest) (enrichment.LexicalEntry, bool, error)
	AncientGreekAvailable() bool
	LookupAncientGreekLSJ(context.Context, string) (enrichment.AncientGreekLSJEntry, bool)
	AncientGreekLSJAvailable() bool
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
	if !index.AncientGreekLSJAvailable() {
		return errors.New("greekhitrate: the dictionary index carries no LSJ entries; rebuild it with `make dictionary-index`")
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

// hitRateReport is the counts-only summary of one run. WithEntry counts
// identities the Wiktionary index resolves; LSJUnambiguous counts identities
// whose headword selects exactly one LSJ entry. Neither is Greek text.
type hitRateReport struct {
	Total          int
	WithEntry      int
	ByUPOS         []uposHitRate
	LSJUnambiguous int
	LSJByUPOS      []uposHitRate
}

type uposHitRate struct {
	UPOS      string
	Total     int
	WithEntry int
}

// summarize counts the identities that resolve to at least one Wiktionary entry
// and those with an unambiguous LSJ match, overall and per part of speech. The
// discriminator never reaches either lookup, so the two homographs of one lemma
// share whatever evidence the lemma and part of speech carry.
func summarize(ctx context.Context, identities []persistence.AncientGreekVocabularyIdentity, index dictionaryIndex) (hitRateReport, error) {
	report := hitRateReport{}
	buckets := make(map[string]*uposHitRate)
	lsjBuckets := make(map[string]*uposHitRate)
	lsjAvailable := index.AncientGreekLSJAvailable()
	for _, identity := range identities {
		_, found, err := index.Lookup(ctx, enrichment.LexicalLookupRequest{Language: "grc", CanonicalLemma: identity.CanonicalLemma, UPOS: identity.UPOS})
		if err != nil {
			return hitRateReport{}, err
		}
		report.Total++
		recordHit(buckets, identity.UPOS, found)
		if found {
			report.WithEntry++
		}
		if !lsjAvailable {
			continue
		}
		_, matched := index.LookupAncientGreekLSJ(ctx, identity.CanonicalLemma)
		recordHit(lsjBuckets, identity.UPOS, matched)
		if matched {
			report.LSJUnambiguous++
		}
	}
	report.ByUPOS = orderedUPOSHitRates(buckets)
	report.LSJByUPOS = orderedUPOSHitRates(lsjBuckets)
	return report, nil
}

// recordHit adds one identity to its part-of-speech bucket, counting whether it
// resolved an entry.
func recordHit(buckets map[string]*uposHitRate, upos string, hit bool) {
	bucket, ok := buckets[upos]
	if !ok {
		bucket = &uposHitRate{UPOS: upos}
		buckets[upos] = bucket
	}
	bucket.Total++
	if hit {
		bucket.WithEntry++
	}
}

// orderedUPOSHitRates returns the per-part-of-speech counts in a stable order.
func orderedUPOSHitRates(buckets map[string]*uposHitRate) []uposHitRate {
	ordered := make([]string, 0, len(buckets))
	for upos := range buckets {
		ordered = append(ordered, upos)
	}
	sort.Strings(ordered)
	result := make([]uposHitRate, 0, len(ordered))
	for _, upos := range ordered {
		result = append(result, *buckets[upos])
	}
	return result
}

// writeReport prints the summary and per-part-of-speech counts, never any
// stored text. The LSJ lines report the unambiguous-match counts the Ancient
// Greek card relies on.
func writeReport(output io.Writer, report hitRateReport) error {
	if _, err := fmt.Fprintf(output, "greek dictionary hit rate: identities=%d with_entry=%d without_entry=%d\n", report.Total, report.WithEntry, report.Total-report.WithEntry); err != nil {
		return err
	}
	for _, bucket := range report.ByUPOS {
		if _, err := fmt.Fprintf(output, "greek dictionary hit rate by upos: upos=%s identities=%d with_entry=%d\n", bucket.UPOS, bucket.Total, bucket.WithEntry); err != nil {
			return err
		}
	}
	if len(report.LSJByUPOS) == 0 {
		return nil
	}
	if _, err := fmt.Fprintf(output, "greek lsj match rate: identities=%d unambiguous=%d ambiguous_or_missing=%d\n", report.Total, report.LSJUnambiguous, report.Total-report.LSJUnambiguous); err != nil {
		return err
	}
	for _, bucket := range report.LSJByUPOS {
		if _, err := fmt.Fprintf(output, "greek lsj match rate by upos: upos=%s identities=%d unambiguous=%d\n", bucket.UPOS, bucket.Total, bucket.WithEntry); err != nil {
			return err
		}
	}
	return nil
}
