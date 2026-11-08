package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"

	"github.com/justin-hayes/mouseion/internal/cardexport"
	"github.com/justin-hayes/mouseion/internal/enrichment"
)

// idiomSampleSize is the reviewed sample size from ADR 0114. A shorter sample
// cannot pass the rollout gate.
const idiomSampleSize = 30

// idiomSampleResult is one sampled idiom item and the provider's answer to the
// request a prepared deck would send for it.
type idiomSampleResult struct {
	Item     cardexport.IdiomWorkItem
	Response enrichment.TranslationResponse
	Err      error
	Outcome  idiomOutcomeKind
}

// idiomSampleReport pins what the sample drew and how it was chosen, so a rerun
// with the same Book and seed sends the same items in the same order.
type idiomSampleReport struct {
	Requested bool
	BookID    string
	Language  string
	Seed      string
	Limit     int
	// Found is the number of distinct idiom items the in-memory freeze matched.
	Found int
	// Cards are the confirmed items, at most Limit. Unconfirmed are the items
	// sent before Limit was reached that the provider did not confirm.
	Cards       []idiomSampleResult
	Unconfirmed []idiomSampleResult
	// Ranking is the per-deck bound's order of every matched idiom. The report
	// lists the ones a prepared deck keeps; no provider call is made for it.
	Ranking []cardexport.IdiomRank
}

// Sent is the number of items sent to the provider.
func (r idiomSampleReport) Sent() int {
	return len(r.Cards) + len(r.Unconfirmed)
}

// runIdiomSample sends the ordered idiom items to the provider, one at a time,
// until idiomSampleSize are confirmed or the items run out. It stops when ctx is
// cancelled, so an interrupted run does not list the unsent items as provider
// errors; the caller then writes no report.
func runIdiomSample(ctx context.Context, client translationClient, options idiomSampleOptions) idiomSampleReport {
	report := idiomSampleReport{
		Requested: options.Requested,
		BookID:    options.BookID,
		Language:  options.Language,
		Seed:      options.Seed,
		Limit:     idiomSampleSize,
		Ranking:   options.Ranking,
	}
	ordered := orderIdiomSample(options.Items, options.BookID, options.Seed)
	report.Found = len(ordered)
	for _, item := range ordered {
		if len(report.Cards) >= report.Limit || ctx.Err() != nil {
			break
		}
		result := idiomSampleResult{Item: item}
		result.Response, result.Err = client.Translate(ctx, item.Work.Request)
		result.Outcome = classifyIdiomOutcome(idiomCaseResult{Response: result.Response, Err: result.Err})
		if result.Outcome == idiomOutcomeConfirmed {
			report.Cards = append(report.Cards, result)
		} else {
			report.Unconfirmed = append(report.Unconfirmed, result)
		}
	}
	return report
}

// orderIdiomSample orders distinct idiom items deterministically for a Book and
// seed. Ordering is by SHA-256 over the Book, seed and item identity, so it does
// not depend on database row order or the Go random implementation. A repeated
// headword in the same sentence is one item.
func orderIdiomSample(items []cardexport.IdiomWorkItem, bookID, seed string) []cardexport.IdiomWorkItem {
	unique := make([]cardexport.IdiomWorkItem, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for _, item := range items {
		key := sampleIdentity(item)
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, item)
	}
	slices.SortStableFunc(unique, func(left, right cardexport.IdiomWorkItem) int {
		leftKey := sampleHash(bookID, seed, left)
		rightKey := sampleHash(bookID, seed, right)
		if leftKey != rightKey {
			return strings.Compare(leftKey, rightKey)
		}
		return strings.Compare(sampleIdentity(left), sampleIdentity(right))
	})
	return unique
}

func sampleHash(bookID, seed string, item cardexport.IdiomWorkItem) string {
	sum := sha256.Sum256([]byte(bookID + "\x00" + seed + "\x00" + sampleIdentity(item)))
	return hex.EncodeToString(sum[:])
}

func sampleIdentity(item cardexport.IdiomWorkItem) string {
	request := item.Work.Request
	return fmt.Sprintf("%s\x00%s\x00%s", request.Language, item.Headword, request.ExampleSentence)
}
