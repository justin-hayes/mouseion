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
// counts only, never source text.
type ClassicalConversionReport struct {
	Converted       int
	EvidenceDiffers int
	Skipped         int
}

// ConvertLegacyClassicalBooks converts every published Book whose source
// material still holds a pinned whole work file. It reproduces the Book's
// existing published analysis under the current interpretation and rewrites the
// source only when the evidence matches exactly. A differing reproduction is
// left alone so the ordinary Analysis update offer stands; a refused or
// unreadable source stays whole-file and keeps the catalog fail-closed.
func ConvertLegacyClassicalBooks(ctx context.Context, store *persistence.PostgresStore, definitions []classical.Definition) (ClassicalConversionReport, error) {
	var report ClassicalConversionReport
	if store == nil {
		return report, errors.New("analysis: classical conversion store is required")
	}
	byKey := make(map[string]classical.Definition, len(definitions))
	for _, definition := range definitions {
		byKey[definition.Key] = definition
	}
	candidates, err := loadClassicalConversionCandidates(ctx, store)
	if err != nil {
		return report, err
	}
	for _, candidate := range candidates {
		definition, ok := byKey[candidate.readingUnitKey]
		if !ok || definition.Withdrawn {
			report.Skipped++
			continue
		}
		converted, outcome := convertLegacyClassicalCandidate(ctx, store, candidate)
		switch {
		case converted:
			report.Converted++
		case outcome == conversionEvidenceDiffers:
			report.EvidenceDiffers++
		default:
			report.Skipped++
		}
	}
	return report, nil
}

// classicalConversionCandidate is one published Book whose source identifier
// still names the whole work file.
type classicalConversionCandidate struct {
	ownerID, bookID, sourceID, sourceIdentifier string
	content                                     []byte
	contentRevisionID, snapshotID               string
	analysisRunID, corpusID                     string
	readingUnitKey                              string
	citationStart, citationEnd                  string
	sourceFileName                              string
	sourceFileSize                              int64
	sourceFileSHA256                            string
	sourceFormat                                string
	interpretationVersion                       string
	definitionRevision                          int
	workTitle, workAuthor, editionLabel         string
}

// loadClassicalConversionCandidates reads every published Book whose current
// source is still keyed by the whole work file rather than its Reading unit.
func loadClassicalConversionCandidates(ctx context.Context, store *persistence.PostgresStore) ([]classicalConversionCandidate, error) {
	rows, err := store.Pool().Query(ctx, `
		SELECT sm.owner_id::text, sm.book_id::text, sm.id::text, sm.source_identifier,
		       COALESCE(r.content, sm.content),
		       sm.current_content_revision_id::text, sm.current_snapshot_id::text,
		       cai.analysis_run_id::text, cai.corpus_id::text,
		       ru.definition_key, ru.citation_start, ru.citation_end,
		       ru.source_file_name, ru.source_file_size, ru.source_file_sha256,
		       ru.source_format, ru.interpretation_version, ru.revision,
		       w.title, w.author, e.label
		FROM source_materials sm
		JOIN current_analysis_identity cai
		  ON cai.owner_id = sm.owner_id AND cai.source_material_id = sm.id AND cai.book_id = sm.book_id
		JOIN source_content_revisions r
		  ON r.owner_id = sm.owner_id AND r.revision_id = cai.content_revision_id
		JOIN book_reading_units bru
		  ON bru.owner_id = sm.owner_id AND bru.book_id = sm.book_id
		JOIN reading_units ru ON ru.id = bru.reading_unit_id
		JOIN editions e ON e.id = ru.edition_id
		JOIN works w ON w.id = e.work_id
		LEFT JOIN classical_prepared_definitions p
		  ON p.owner_id = sm.owner_id AND p.book_id = sm.book_id
		WHERE sm.language = $1
		  AND sm.media_type = $2
		  AND sm.book_id IS NOT NULL
		  AND sm.source_identifier <> ru.definition_key
		  AND ru.withdrawn = false
		  AND p.book_id IS NULL
		ORDER BY sm.owner_id, sm.id`, classical.Language, domain.ImportedSourceMediaType)
	if err != nil {
		return nil, fmt.Errorf("load classical conversion candidates: %w", err)
	}
	defer rows.Close()
	candidates := []classicalConversionCandidate{}
	for rows.Next() {
		var candidate classicalConversionCandidate
		if err := rows.Scan(&candidate.ownerID, &candidate.bookID, &candidate.sourceID, &candidate.sourceIdentifier,
			&candidate.content, &candidate.contentRevisionID, &candidate.snapshotID,
			&candidate.analysisRunID, &candidate.corpusID,
			&candidate.readingUnitKey, &candidate.citationStart, &candidate.citationEnd,
			&candidate.sourceFileName, &candidate.sourceFileSize, &candidate.sourceFileSHA256,
			&candidate.sourceFormat, &candidate.interpretationVersion, &candidate.definitionRevision,
			&candidate.workTitle, &candidate.workAuthor, &candidate.editionLabel); err != nil {
			return nil, fmt.Errorf("read classical conversion candidate: %w", err)
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read classical conversion candidates: %w", err)
	}
	return candidates, nil
}

type classicalConversionOutcome string

const (
	conversionConverted       classicalConversionOutcome = "converted"
	conversionEvidenceDiffers classicalConversionOutcome = "evidence-differs"
	conversionSkipped         classicalConversionOutcome = "skipped"
)

// convertLegacyClassicalCandidate reproduces one Book's published analysis and
// converts its source only when every compared fact matches.
func convertLegacyClassicalCandidate(ctx context.Context, store *persistence.PostgresStore, candidate classicalConversionCandidate) (bool, classicalConversionOutcome) {
	if int64(len(candidate.content)) != candidate.sourceFileSize || sha256Hex(candidate.content) != candidate.sourceFileSHA256 {
		logClassicalConversionFailure(candidate, "source_not_whole_file")
		return false, conversionSkipped
	}
	if candidate.sourceFormat != classical.SourceFormatOGACoNLLU {
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
	sentences, tokens, err := loadPublishedConversionEvidence(ctx, store, candidate)
	if err != nil {
		log.Printf("mouseion: classical conversion evidence unavailable owner=%s book=%s unit=%s: %v",
			candidate.ownerID, candidate.bookID, candidate.readingUnitKey, err)
		return false, conversionSkipped
	}
	if !publishedEvidenceMatches(unit.Result, sentences, tokens) {
		log.Printf("mouseion: classical conversion evidence differs owner=%s book=%s unit=%s revision=%d",
			candidate.ownerID, candidate.bookID, candidate.readingUnitKey, candidate.definitionRevision)
		return false, conversionEvidenceDiffers
	}
	converted, err := store.ConvertLegacyClassicalSource(ctx, domain.ClassicalSourceConversion{
		OwnerID:                   candidate.ownerID,
		BookID:                    candidate.bookID,
		SourceMaterialID:          candidate.sourceID,
		ReadingUnitKey:            candidate.readingUnitKey,
		UnitID:                    unit.UnitID,
		UnitTitle:                 candidate.workTitle,
		UnitText:                  unit.FullText,
		SourceFileName:            candidate.sourceFileName,
		DefinitionRevision:        candidate.definitionRevision,
		SourceFileSHA256:          candidate.sourceFileSHA256,
		InterpretationVersion:     candidate.interpretationVersion,
		AnalysisRunID:             candidate.analysisRunID,
		ExpectedContentRevisionID: candidate.contentRevisionID,
		ExpectedSnapshotID:        candidate.snapshotID,
	})
	if errors.Is(err, persistence.ErrClassicalConversionChanged) {
		log.Printf("mouseion: classical conversion evidence changed owner=%s book=%s unit=%s",
			candidate.ownerID, candidate.bookID, candidate.readingUnitKey)
		return false, conversionSkipped
	}
	if err != nil {
		log.Printf("mouseion: classical conversion failed owner=%s book=%s unit=%s: %v",
			candidate.ownerID, candidate.bookID, candidate.readingUnitKey, err)
		return false, conversionSkipped
	}
	if !converted {
		return false, conversionSkipped
	}
	log.Printf("mouseion: classical conversion complete owner=%s book=%s unit=%s revision=%d",
		candidate.ownerID, candidate.bookID, candidate.readingUnitKey, candidate.definitionRevision)
	return true, conversionConverted
}

// conversionDefinition is the adapter definition the Book's reconciled Reading
// unit names.
func conversionDefinition(candidate classicalConversionCandidate) domain.ReadingUnitDefinition {
	return domain.ReadingUnitDefinition{
		Key:                   candidate.readingUnitKey,
		Language:              classical.Language,
		WorkTitle:             candidate.workTitle,
		WorkAuthor:            candidate.workAuthor,
		EditionLabel:          candidate.editionLabel,
		CitationStart:         candidate.citationStart,
		CitationEnd:           candidate.citationEnd,
		SourceFileName:        candidate.sourceFileName,
		SourceFileSize:        candidate.sourceFileSize,
		SourceFileSHA256:      candidate.sourceFileSHA256,
		InterpretationVersion: candidate.interpretationVersion,
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
		ORDER BY sentence_ordinal`, candidate.ownerID, candidate.analysisRunID, candidate.corpusID)
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
		ORDER BY sentence_ordinal, token_ordinal`, candidate.ownerID, candidate.analysisRunID, candidate.corpusID)
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
		candidate.ownerID, candidate.bookID, candidate.readingUnitKey, code)
}
