package analysis

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log"
	"slices"

	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/ancientgreek"
	"github.com/justin-hayes/mouseion/internal/classical"
	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/justin-hayes/mouseion/internal/persistence"
)

// ClassicalConversionReport counts the outcomes of the one-time conversion of
// Books registered before the Classical catalog. It carries identifiers and
// counts only, never source text. Unpublished and Withdrawn are subsets of
// Converted that name the two recoverable shapes the catalog could not clear.
type ClassicalConversionReport struct {
	Converted       int
	EvidenceDiffers int
	// Unpublished counts converted Books that had no published analysis run.
	Unpublished int
	// Withdrawn counts converted Books whose Reading unit is withdrawn.
	Withdrawn int
	Skipped   int
}

// ConvertLegacyClassicalBooks converts every Book the shared legacy definition
// counts. It reproduces a published analysis under the current interpretation,
// then rewrites the source to unit-only content under the Reading-unit key. A
// matching reproduction also records the prepared definition; a differing one
// leaves the published evidence and that record alone so the ordinary Analysis
// update offer stands.
//
// A Book whose interrupted import left a whole-file source with no published
// analysis is converted the same way without a run to re-point; ordinary
// classical preparation publishes its analysis later. A Book whose Reading unit
// was withdrawn is converted to unit-only content under its key and records no
// prepared definition, so it behaves like any other Book with a withdrawn
// definition instead of blocking the installation. A refused or unreadable
// source stays whole-file and keeps the catalog fail-closed until a later start
// retries it.
func ConvertLegacyClassicalBooks(ctx context.Context, store *persistence.PostgresStore) (ClassicalConversionReport, error) {
	var report ClassicalConversionReport
	if store == nil {
		return report, errors.New("analysis: classical conversion store is required")
	}
	candidates, err := loadClassicalConversionCandidates(ctx, store)
	if err != nil {
		return report, err
	}
	for _, candidate := range candidates {
		converted, outcome := convertLegacyClassicalCandidate(ctx, store, candidate)
		switch {
		case converted:
			report.Converted++
			if candidate.AnalysisRunID == "" {
				report.Unpublished++
			}
			if candidate.Withdrawn {
				report.Withdrawn++
			}
		case outcome == conversionEvidenceDiffers:
			report.EvidenceDiffers++
		default:
			report.Skipped++
		}
	}
	return report, nil
}

// classicalConversionCandidate is one Book the shared legacy definition
// counts, with the whole-file source bytes conversion reads.
type classicalConversionCandidate struct {
	persistence.LegacyClassicalSourceMaterial
	content []byte
}

// loadClassicalConversionCandidates reads the shared legacy-Book definition the
// catalog check counts. Both cases the check caught but conversion used to skip
// are included: a source row with no published analysis, and a withdrawn unit.
func loadClassicalConversionCandidates(ctx context.Context, store *persistence.PostgresStore) ([]classicalConversionCandidate, error) {
	materials, err := store.ListLegacyClassicalSourceMaterials(ctx, classical.Language)
	if err != nil {
		return nil, fmt.Errorf("load classical conversion candidates: %w", err)
	}
	candidates := make([]classicalConversionCandidate, 0, len(materials))
	for _, material := range materials {
		source, err := store.GetSourceMaterial(ctx, material.OwnerID, material.SourceMaterialID)
		if err != nil {
			return nil, fmt.Errorf("load classical conversion source: %w", err)
		}
		candidates = append(candidates, classicalConversionCandidate{
			LegacyClassicalSourceMaterial: material,
			content:                       source.Content,
		})
	}
	return candidates, nil
}

type classicalConversionOutcome string

const (
	conversionConverted       classicalConversionOutcome = "converted"
	conversionEvidenceDiffers classicalConversionOutcome = "evidence-differs"
	conversionSkipped         classicalConversionOutcome = "skipped"
)

// convertLegacyClassicalCandidate reproduces one published Book's analysis and
// rewrites its source identity. A published evidence mismatch still rewrites
// the source but records no prepared definition, leaving the ordinary Analysis
// update outstanding.
func convertLegacyClassicalCandidate(ctx context.Context, store *persistence.PostgresStore, candidate classicalConversionCandidate) (bool, classicalConversionOutcome) {
	if int64(len(candidate.content)) != candidate.SourceFileSize || sha256Hex(candidate.content) != candidate.SourceFileSHA256 {
		logClassicalConversionFailure(candidate, "source_not_whole_file")
		return false, conversionSkipped
	}
	// The retired operator registration stored no source format. A unit the
	// first manifest withdraws before ever reconciling it therefore keeps an
	// empty format even though its whole-file source is the OGA CoNLL-U the old
	// path required. Treat that withdrawn-only empty value as OGA; anything
	// else is a format this conversion cannot reproduce.
	legacyWithdrawnFormat := candidate.Withdrawn && candidate.SourceFormat == ""
	if candidate.SourceFormat != classical.SourceFormatOGACoNLLU && !legacyWithdrawnFormat {
		logClassicalConversionFailure(candidate, "source_format_unknown")
		return false, conversionSkipped
	}
	parsed, err := ancientgreek.ParseOgaSource(candidate.content)
	if err != nil {
		logClassicalConversionFailure(candidate, "source_malformed")
		return false, conversionSkipped
	}
	unit, err := parsed.CutUnit(conversionDefinition(candidate))
	if err != nil {
		logClassicalConversionFailure(candidate, "interpretation_refused")
		return false, conversionSkipped
	}
	conversion := legacyClassicalConversion(candidate, unit)
	// A withdrawn unit has no preparation to recover, and a source with no
	// published analysis has no evidence to reproduce. Both finish the
	// source-identity rewrite only, so the Book stops blocking the catalog and
	// ordinary preparation (or the withdrawn state) takes over. A withdrawn
	// Book cannot gain a prepared definition, so its published run, if any, is
	// re-pointed like every other conversion without an evidence comparison
	// changing the outcome.
	if candidate.Withdrawn || candidate.AnalysisRunID == "" {
		outcome := writeLegacyClassicalConversion(ctx, store, conversion, conversionConverted)
		return outcome == conversionConverted, outcome
	}
	sentences, tokens, err := loadPublishedConversionEvidence(ctx, store, candidate)
	if err != nil {
		log.Printf("mouseion: classical conversion evidence unavailable owner=%s book=%s unit=%s: %v",
			candidate.OwnerID, candidate.BookID, candidate.ReadingUnitKey, err)
		return false, conversionSkipped
	}
	if !publishedEvidenceMatches(unit.Result, sentences, tokens) {
		log.Printf("mouseion: classical conversion evidence differs owner=%s book=%s unit=%s revision=%d",
			candidate.OwnerID, candidate.BookID, candidate.ReadingUnitKey, candidate.DefinitionRevision)
		conversion.EvidenceDiffers = true
		return false, writeLegacyClassicalConversion(ctx, store, conversion, conversionEvidenceDiffers)
	}
	outcome := writeLegacyClassicalConversion(ctx, store, conversion, conversionConverted)
	return outcome == conversionConverted, outcome
}

// writeLegacyClassicalConversion applies one source-identity rewrite and
// reports the startup outcome. success names the outcome a committed write
// produces, so the caller can keep the matched and differing cases distinct.
func writeLegacyClassicalConversion(ctx context.Context, store *persistence.PostgresStore, conversion domain.ClassicalSourceConversion, success classicalConversionOutcome) classicalConversionOutcome {
	converted, err := store.ConvertLegacyClassicalSource(ctx, conversion)
	if errors.Is(err, persistence.ErrClassicalConversionChanged) {
		log.Printf("mouseion: classical conversion evidence changed owner=%s book=%s unit=%s",
			conversion.OwnerID, conversion.BookID, conversion.ReadingUnitKey)
		return conversionSkipped
	}
	if err != nil {
		log.Printf("mouseion: classical conversion failed owner=%s book=%s unit=%s: %v",
			conversion.OwnerID, conversion.BookID, conversion.ReadingUnitKey, err)
		return conversionSkipped
	}
	if !converted {
		return conversionSkipped
	}
	if success == conversionEvidenceDiffers {
		log.Printf("mouseion: classical conversion source complete owner=%s book=%s unit=%s revision=%d analysis_update=pending",
			conversion.OwnerID, conversion.BookID, conversion.ReadingUnitKey, conversion.DefinitionRevision)
	} else {
		log.Printf("mouseion: classical conversion complete owner=%s book=%s unit=%s revision=%d",
			conversion.OwnerID, conversion.BookID, conversion.ReadingUnitKey, conversion.DefinitionRevision)
	}
	return success
}

// legacyClassicalConversion builds the source-identity rewrite both conversion
// outcomes share.
func legacyClassicalConversion(candidate classicalConversionCandidate, unit ancientgreek.PreparedUnit) domain.ClassicalSourceConversion {
	return domain.ClassicalSourceConversion{
		OwnerID:                   candidate.OwnerID,
		BookID:                    candidate.BookID,
		SourceMaterialID:          candidate.SourceMaterialID,
		ReadingUnitKey:            candidate.ReadingUnitKey,
		UnitID:                    unit.UnitID,
		UnitTitle:                 candidate.WorkTitle,
		UnitText:                  unit.FullText,
		SourceFileName:            candidate.SourceFileName,
		DefinitionRevision:        candidate.DefinitionRevision,
		SourceFileSHA256:          candidate.SourceFileSHA256,
		InterpretationVersion:     candidate.InterpretationVersion,
		AnalysisRunID:             candidate.AnalysisRunID,
		ExpectedContentRevisionID: candidate.ContentRevisionID,
		ExpectedSnapshotID:        candidate.SnapshotID,
		Withdrawn:                 candidate.Withdrawn,
	}
}

// conversionDefinition is the adapter definition the Book's reconciled Reading
// unit names.
func conversionDefinition(candidate classicalConversionCandidate) domain.ReadingUnitDefinition {
	return domain.ReadingUnitDefinition{
		Key:                   candidate.ReadingUnitKey,
		Language:              classical.Language,
		WorkTitle:             candidate.WorkTitle,
		WorkAuthor:            candidate.WorkAuthor,
		EditionLabel:          candidate.EditionLabel,
		CitationStart:         candidate.CitationStart,
		CitationEnd:           candidate.CitationEnd,
		SourceFileName:        candidate.SourceFileName,
		SourceFileSize:        candidate.SourceFileSize,
		SourceFileSHA256:      candidate.SourceFileSHA256,
		InterpretationVersion: candidate.InterpretationVersion,
	}
}

type conversionSentence struct {
	text      string
	citations []string
}

type conversionToken struct {
	ordinal              int64
	surface              string
	rawLemma             string
	canonicalLemma       string
	upos                 string
	headword             string
	lexicalDiscriminator string
}

// loadPublishedConversionEvidence reads the sentence text, citations and token
// evidence the Book's published run currently stores.
func loadPublishedConversionEvidence(ctx context.Context, store *persistence.PostgresStore, candidate classicalConversionCandidate) ([]conversionSentence, map[int64][]conversionToken, error) {
	sentences, err := loadConversionSentences(ctx, store, candidate)
	if err != nil {
		return nil, nil, err
	}
	tokens, err := loadConversionTokens(ctx, store, candidate)
	if err != nil {
		return nil, nil, err
	}
	return sentences, tokens, nil
}

func loadConversionSentences(ctx context.Context, store *persistence.PostgresStore, candidate classicalConversionCandidate) ([]conversionSentence, error) {
	rows, err := store.Pool().Query(ctx, `
		SELECT sentence_text, citations
		FROM corpus_sentences
		WHERE owner_id = $1 AND analysis_run_id = $2 AND corpus_id = $3
		ORDER BY sentence_ordinal`, candidate.OwnerID, candidate.AnalysisRunID, candidate.CorpusID)
	if err != nil {
		return nil, fmt.Errorf("load published conversion sentences: %w", err)
	}
	defer rows.Close()
	sentences := []conversionSentence{}
	for rows.Next() {
		var sentence conversionSentence
		if err := rows.Scan(&sentence.text, &sentence.citations); err != nil {
			return nil, fmt.Errorf("read published conversion sentence: %w", err)
		}
		sentences = append(sentences, sentence)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read published conversion sentences: %w", err)
	}
	return sentences, nil
}

func loadConversionTokens(ctx context.Context, store *persistence.PostgresStore, candidate classicalConversionCandidate) (map[int64][]conversionToken, error) {
	rows, err := store.Pool().Query(ctx, `
		SELECT sentence_ordinal, token_ordinal, surface, raw_lemma, canonical_lemma,
		       upos, headword, lexical_discriminator
		FROM corpus_tokens
		WHERE owner_id = $1 AND analysis_run_id = $2 AND corpus_id = $3
		ORDER BY sentence_ordinal, token_ordinal`, candidate.OwnerID, candidate.AnalysisRunID, candidate.CorpusID)
	if err != nil {
		return nil, fmt.Errorf("load published conversion tokens: %w", err)
	}
	defer rows.Close()
	tokens := map[int64][]conversionToken{}
	for rows.Next() {
		var sentenceOrdinal int64
		var token conversionToken
		if err := rows.Scan(&sentenceOrdinal, &token.ordinal, &token.surface, &token.rawLemma,
			&token.canonicalLemma, &token.upos, &token.headword, &token.lexicalDiscriminator); err != nil {
			return nil, fmt.Errorf("read published conversion token: %w", err)
		}
		tokens[sentenceOrdinal] = append(tokens[sentenceOrdinal], token)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read published conversion tokens: %w", err)
	}
	return tokens, nil
}

// publishedEvidenceMatches compares the reproduced interpretation with the
// stored sentence text, tokens, lemmas, UPOS, headword, discriminator and
// citations.
func publishedEvidenceMatches(result analyzer.Result, sentences []conversionSentence, stored map[int64][]conversionToken) bool {
	if len(result.Sentences) != len(sentences) {
		return false
	}
	for index, sentence := range result.Sentences {
		if sentence.Text != sentences[index].text || !slices.Equal(sentence.Citations, sentences[index].citations) {
			return false
		}
		tokens := stored[int64(index)]
		if len(tokens) != len(sentence.Tokens) {
			return false
		}
		for tokenIndex, token := range sentence.Tokens {
			storedToken := tokens[tokenIndex]
			if storedToken.ordinal != int64(tokenIndex) ||
				storedToken.surface != token.Surface ||
				storedToken.rawLemma != token.RawLemma ||
				storedToken.canonicalLemma != token.CanonicalLemma ||
				storedToken.upos != token.UPOS ||
				storedToken.headword != token.Headword ||
				storedToken.lexicalDiscriminator != token.LexicalDiscriminator {
				return false
			}
		}
	}
	return true
}

// sha256Hex returns the lowercase hexadecimal SHA-256 of content.
func sha256Hex(content []byte) string {
	sum := sha256.Sum256(content)
	return hex.EncodeToString(sum[:])
}

// logClassicalConversionFailure records a stable code only.
func logClassicalConversionFailure(candidate classicalConversionCandidate, code string) {
	log.Printf("mouseion: classical conversion skipped owner=%s book=%s unit=%s code=%s",
		candidate.OwnerID, candidate.BookID, candidate.ReadingUnitKey, code)
}
