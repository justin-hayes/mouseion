package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"

	"github.com/justin-hayes/mouseion/internal/persistence"
)

// idiomSampleSize is the reviewed sample size from ADR 0114. A shorter sample
// cannot pass the rollout gate.
const idiomSampleSize = 30

// idiomSampleReport pins what the sample drew and how it was chosen, so a rerun
// with the same Book and seed selects the same cards.
type idiomSampleReport struct {
	Requested bool
	BookID    string
	Language  string
	Seed      string
	Limit     int
	Cards     []persistence.ConfirmedIdiomCard
}

// selectIdiomSample chooses up to limit distinct confirmed idiom cards
// deterministically for a Book and seed. Ordering is by SHA-256 over the Book,
// seed and card identity, so it does not depend on database row order or the Go
// random implementation. A repeated headword in the same sentence is one card.
func selectIdiomSample(cards []persistence.ConfirmedIdiomCard, bookID, seed string, limit int) []persistence.ConfirmedIdiomCard {
	if limit <= 0 || len(cards) == 0 {
		return nil
	}
	unique := make([]persistence.ConfirmedIdiomCard, 0, len(cards))
	seen := make(map[string]struct{}, len(cards))
	for _, card := range cards {
		key := sampleIdentity(card)
		if _, duplicate := seen[key]; duplicate {
			continue
		}
		seen[key] = struct{}{}
		unique = append(unique, card)
	}
	slices.SortStableFunc(unique, func(left, right persistence.ConfirmedIdiomCard) int {
		leftKey := sampleHash(bookID, seed, left)
		rightKey := sampleHash(bookID, seed, right)
		if leftKey != rightKey {
			return strings.Compare(leftKey, rightKey)
		}
		return strings.Compare(sampleIdentity(left), sampleIdentity(right))
	})
	if len(unique) > limit {
		unique = unique[:limit]
	}
	return unique
}

func sampleHash(bookID, seed string, card persistence.ConfirmedIdiomCard) string {
	sum := sha256.Sum256([]byte(bookID + "\x00" + seed + "\x00" + sampleIdentity(card)))
	return hex.EncodeToString(sum[:])
}

func sampleIdentity(card persistence.ConfirmedIdiomCard) string {
	return fmt.Sprintf("%s\x00%s\x00%s", card.Language, card.Headword, card.Sentence)
}
