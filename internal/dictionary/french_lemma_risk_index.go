package dictionary

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/enrichment"
	"github.com/justin-hayes/mouseion/internal/lemmarisk"
)

// frenchLanguage is the language tag for the French evidence rule.
const frenchLanguage = "fr"

// ErrFrenchFormsUnavailable reports that the index predates the French form-of
// data lemma-risk review needs. French assessment treats it as unavailable, not
// as a clean verdict; German assessment does not depend on it.
var ErrFrenchFormsUnavailable = errors.New("dictionary: French form-of data requires a rebuilt index")

// indexFormatWithFrenchForms is the first non-LSJ index format carrying the
// French form-of links and headword membership (ticket #2244). LSJ indexes start
// at format 6. The table probe, not the version alone, is authoritative: a
// historical format-4 LSJ index has the number but not the tables.
const indexFormatWithFrenchForms = 4

// loadFrenchFormsAvailability records whether the index carries both French
// form-of tables. A missing table or an older format reports French lemma-risk
// assessment unavailable instead of failing the whole index.
func (i *Index) loadFrenchFormsAvailability(ctx context.Context) error {
	version, ok, err := i.indexFormat(ctx)
	if err != nil {
		return err
	}
	if !ok || version < indexFormatWithFrenchForms {
		return nil
	}
	var available bool
	err = i.db.QueryRowContext(ctx, `SELECT
		EXISTS(SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'french_forms')
		AND EXISTS(SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'french_headwords')`).Scan(&available)
	if err != nil {
		return fmt.Errorf("%w: French form schema: %w", ErrInvalidIndex, err)
	}
	i.frenchFormsAvailable = available
	return nil
}

// AssessmentAvailable reports whether the index can assess lemma risk for a
// language. A French index older than the form-of format is unavailable even
// when every occurrence is already reviewed and detection would otherwise have
// nothing to inspect.
func (i *Index) AssessmentAvailable(_ context.Context, language string) (bool, error) {
	switch canonicalization.NormalizeLanguage(language) {
	case "de":
		return true, nil
	case frenchLanguage:
		return i != nil && i.frenchFormsAvailable, nil
	default:
		return false, nil
	}
}

// frenchAlternatives implements the French evidence rule. A participial
// adjective whose analyzed lemma is the verb it is a recorded participle of is
// not a lemma error: the analyzer kept the verb's infinitive while the surface
// is the verb's own form, so no alternative is offered.
func (i *Index) frenchAlternatives(ctx context.Context, surface, lemma, upos string) ([]lemmarisk.Alternative, error) {
	if upos == "" {
		return nil, nil
	}
	if !i.frenchFormsAvailable {
		return nil, ErrFrenchFormsUnavailable
	}
	normalizedSurface := normalizeLemma(frenchLanguage, surface)
	if normalizedSurface == "" {
		return nil, nil
	}
	if upos == "ADJ" {
		participle, err := i.frenchParticipleFormOf(ctx, normalizedSurface, normalizeLemma(frenchLanguage, lemma))
		if err != nil {
			return nil, err
		}
		if participle {
			return nil, nil
		}
	}
	result := make([]lemmarisk.Alternative, 0, 2)
	headword, found, err := i.frenchSurfaceHeadword(ctx, normalizedSurface, upos)
	if err != nil {
		return nil, err
	}
	if found {
		result = append(result, headword)
	}
	formOf, err := i.frenchFormOfAlternatives(ctx, normalizedSurface, upos)
	if err != nil {
		return nil, err
	}
	return append(result, formOf...), nil
}

// frenchSurfaceHeadword returns the surface's own indexed headword when that
// entry has at least one non-form-of sense. An entry made only of form-of
// senses is not a headword.
func (i *Index) frenchSurfaceHeadword(ctx context.Context, surface, upos string) (lemmarisk.Alternative, bool, error) {
	isHeadword, err := i.frenchHeadwordExists(ctx, surface, upos)
	if err != nil || !isHeadword {
		return lemmarisk.Alternative{}, false, err
	}
	entry, found, err := i.Lookup(ctx, enrichment.LexicalLookupRequest{Language: frenchLanguage, CanonicalLemma: surface, UPOS: upos})
	if err != nil || !found {
		return lemmarisk.Alternative{}, false, err
	}
	for _, sense := range entry.CandidateSenses {
		if hasFormOfTag(sense.Tags) || sense.Source == "" || sense.Version == "" || sense.EvidenceID == "" {
			continue
		}
		return lemmarisk.Alternative{Lemma: surface, UPOS: upos, Source: sense.Source, Version: sense.Version, EvidenceID: sense.EvidenceID, ContextRelevant: true}, true, nil
	}
	return lemmarisk.Alternative{}, false, nil
}

// frenchHeadwordExists reports whether the index records at least one non-form-of
// sense for the exact identity.
func (i *Index) frenchHeadwordExists(ctx context.Context, lemma, upos string) (bool, error) {
	var isHeadword bool
	err := i.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM french_headwords WHERE lemma = ? AND upos = ?)`, lemma, upos).Scan(&isHeadword)
	if err != nil {
		return false, fmt.Errorf("dictionary French headword: %w", err)
	}
	return isHeadword, nil
}

// frenchFormOfAlternatives returns the indexed headwords the surface is a
// recorded form of, in derivation order, each carrying the form-of row's
// evidence.
func (i *Index) frenchFormOfAlternatives(ctx context.Context, surface, upos string) ([]lemmarisk.Alternative, error) {
	rows, err := i.db.QueryContext(ctx, `SELECT target, evidence_id FROM french_forms WHERE form = ? AND upos = ? ORDER BY rowid`, surface, upos)
	if err != nil {
		return nil, fmt.Errorf("dictionary French form-of: %w", err)
	}
	defer rows.Close() //nolint:errcheck // Rows.Err below reports actionable lookup failures.
	result := make([]lemmarisk.Alternative, 0, 2)
	seen := make(map[string]bool)
	for rows.Next() {
		var target, evidenceID string
		if err = rows.Scan(&target, &evidenceID); err != nil {
			return nil, fmt.Errorf("dictionary French form-of row: %w", err)
		}
		alternative, ok, err := i.frenchFormOfAlternative(ctx, target, evidenceID, upos, seen)
		if err != nil {
			return nil, err
		}
		if ok {
			result = append(result, alternative)
		}
	}
	if err = rows.Err(); err != nil {
		return nil, fmt.Errorf("dictionary French form-of rows: %w", err)
	}
	return result, nil
}

// frenchFormOfAlternative returns one form-of link's alternative when its target
// is an indexed headword with the occurrence's part of speech.
func (i *Index) frenchFormOfAlternative(ctx context.Context, target, evidenceID, upos string, seen map[string]bool) (lemmarisk.Alternative, bool, error) {
	if seen[target] || evidenceID == "" {
		return lemmarisk.Alternative{}, false, nil
	}
	exists, err := i.LemmaExists(ctx, frenchLanguage, target, upos)
	if err != nil || !exists {
		return lemmarisk.Alternative{}, false, err
	}
	isHeadword, err := i.frenchHeadwordExists(ctx, target, upos)
	if err != nil || !isHeadword {
		return lemmarisk.Alternative{}, false, err
	}
	seen[target] = true
	return lemmarisk.Alternative{Lemma: target, UPOS: upos, Source: wiktionarySource, Version: i.version, EvidenceID: evidenceID, ContextRelevant: true}, true, nil
}

// frenchParticipleFormOf reports whether the analyzed lemma is a verb headword
// and the surface is one of its recorded participle forms. The headword check
// matches the surface-headword rule: a form-of-only verb entry is not a lemma.
func (i *Index) frenchParticipleFormOf(ctx context.Context, surface, lemma string) (bool, error) {
	exists, err := i.LemmaExists(ctx, frenchLanguage, lemma, "VERB")
	if err != nil || !exists {
		return false, err
	}
	isHeadword, err := i.frenchHeadwordExists(ctx, lemma, "VERB")
	if err != nil || !isHeadword {
		return false, err
	}
	var found bool
	err = i.db.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM french_forms WHERE form = ? AND target = ? AND participle = 1)`, surface, lemma).Scan(&found)
	if err != nil {
		return false, fmt.Errorf("dictionary French participle form: %w", err)
	}
	return found, nil
}

func hasFormOfTag(tags []string) bool {
	for _, tag := range tags {
		if strings.EqualFold(tag, "form-of") {
			return true
		}
	}
	return false
}
