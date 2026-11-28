package persistence

import (
	"context"
	"fmt"
	"strings"

	sqlcgen "github.com/justin-hayes/mouseion/gen/sqlc"
	"github.com/justin-hayes/mouseion/internal/analyzer"
	"github.com/justin-hayes/mouseion/internal/canonicalization"
	"github.com/justin-hayes/mouseion/internal/domain"
)

// StudyIdiomSentence is one sentence of the Corpus that contains every word of
// an Idiom headword, with what choosing it as the Idiom's study sentence needs.
type StudyIdiomSentence struct {
	BookID, AnalysisRunID, CorpusID string
	SentenceOrdinal                 int64
	// BookStartOffset is where the sentence starts in its Book.
	BookStartOffset int64
	// Sentence is the analysed sentence as persisted.
	Sentence analyzer.Sentence
	// Effective are the sentence's tokens with the learner's decisions applied:
	// a confirmed Lemma correction supplies the lemma, and an excluded
	// Occurrence matches nothing. They are the tokens the matcher reads.
	Effective []analyzer.Token
}

// ListStudyIdiomCandidates returns the Idioms of the language as Study
// candidates, unordered, with their Personal occurrences: each headword's
// candidate count across the Corpus. PendingBooks counts the Books whose counts
// are not ready yet at indexVersion, so a headword missing from the list may
// only be waiting. Page requests never call it: planning does.
func (s *PostgresStore) ListStudyIdiomCandidates(ctx context.Context, owner, language, indexVersion string) ([]domain.StudyCandidate, int64, error) {
	language = canonicalization.NormalizeLanguage(language)
	counts, err := s.ListCorpusIdiomCandidateCounts(ctx, owner, language, indexVersion)
	if err != nil {
		return nil, 0, err
	}
	return domain.IdiomStudyCandidates(language, counts.Counts), counts.PendingBooks, nil
}

// StudyIdiomSentenceRef names one candidate sentence of an Idiom: where it is
// and what is needed to load it. It carries no text.
type StudyIdiomSentenceRef struct {
	BookID, AnalysisRunID, CorpusID string
	SentenceOrdinal                 int64
	// BookStartOffset is where the sentence starts in its Book.
	BookStartOffset int64
	// source locates the sentence's unit for the speaker-label rule.
	sourceID, snapshotID, unitID string
	unitStart, unitEnd           int64
}

// ListStudyIdiomSentenceRefs returns, in order, the sentences of the owner's
// currently analyzed Books in My Books that contain every one of the headword's
// component words as a surface form or an effective lemma. Only the Books whose Idiom counts list the headword are
// read. It is a superset of the sentences the matcher accepts, which the
// caller decides; LoadStudyIdiomSentences reads them in batches.
func (s *PostgresStore) ListStudyIdiomSentenceRefs(ctx context.Context, owner, language, headword string, words []string) ([]StudyIdiomSentenceRef, error) {
	language = canonicalization.NormalizeLanguage(language)
	distinct := distinctLowerWords(words)
	if len(distinct) == 0 {
		return nil, nil
	}
	rows, err := sqlcgen.New(s.pool).ListStudyIdiomSentences(ctx, sqlcgen.ListStudyIdiomSentencesParams{Owner: owner, Language: language, Words: distinct, Headword: headword})
	if err != nil {
		return nil, fmt.Errorf("list Study Idiom sentences: %w", err)
	}
	refs := make([]StudyIdiomSentenceRef, len(rows))
	for i, row := range rows {
		refs[i] = StudyIdiomSentenceRef{
			BookID: row.BookID, AnalysisRunID: row.AnalysisRunID, CorpusID: row.CorpusID, SentenceOrdinal: row.SentenceOrdinal,
			BookStartOffset: row.BookStartOffset,
			sourceID:        row.SourceMaterialID, snapshotID: row.SnapshotID, unitID: row.UnitID, unitStart: row.UnitStartOffset, unitEnd: row.UnitEndOffset,
		}
	}
	return refs, nil
}

// LoadStudyIdiomSentences loads the referenced sentences with their effective
// tokens, in the refs' order. A sentence that overlaps a recorded speaker label
// supplies no Study context (ADR 0103) and is left out, as for a word.
func (s *PostgresStore) LoadStudyIdiomSentences(ctx context.Context, owner string, refs []StudyIdiomSentenceRef) ([]StudyIdiomSentence, error) {
	type scope struct{ book, run, corpus string }
	ordinals := map[scope][]int64{}
	for _, ref := range refs {
		key := scope{ref.BookID, ref.AnalysisRunID, ref.CorpusID}
		ordinals[key] = append(ordinals[key], ref.SentenceOrdinal)
	}
	loaded := make(map[scope]map[int64]analyzer.Sentence, len(ordinals))
	decisions := make(map[scope]map[[2]int64]sqlcgen.ListStudySentenceDecisionsRow, len(ordinals))
	for key, list := range ordinals {
		sentences, err := s.ListCorpusSentences(ctx, owner, key.corpus, list)
		if err != nil {
			return nil, err
		}
		loaded[key] = sentences
		byToken, err := s.studySentenceDecisions(ctx, owner, key.book, key.run, key.corpus, list)
		if err != nil {
			return nil, err
		}
		decisions[key] = byToken
	}
	structures := map[[2]string]domain.SnapshotStructure{}
	result := make([]StudyIdiomSentence, 0, len(refs))
	for _, ref := range refs {
		key := scope{ref.BookID, ref.AnalysisRunID, ref.CorpusID}
		sentence, ok := loaded[key][ref.SentenceOrdinal]
		if !ok {
			continue
		}
		structure, err := s.cachedSnapshotStructure(ctx, owner, structures, ref)
		if err != nil {
			return nil, err
		}
		if speakerLabelOverlap(structure, ref.unitID, ref.unitStart, ref.unitEnd) {
			continue
		}
		effective := make([]analyzer.Token, len(sentence.Tokens))
		for i, token := range sentence.Tokens {
			effective[i] = effectiveStudyToken(token, decisions[key], ref.SentenceOrdinal, int64(i))
		}
		result = append(result, StudyIdiomSentence{
			BookID: ref.BookID, AnalysisRunID: ref.AnalysisRunID, CorpusID: ref.CorpusID, SentenceOrdinal: ref.SentenceOrdinal,
			BookStartOffset: ref.BookStartOffset, Sentence: sentence, Effective: effective,
		})
	}
	return result, nil
}

// studySentenceDecisions are the Lemma corrections and Occurrence exclusions on
// the tokens of the sentences, by sentence and token ordinal.
func (s *PostgresStore) studySentenceDecisions(ctx context.Context, owner, book, run, corpus string, ordinals []int64) (map[[2]int64]sqlcgen.ListStudySentenceDecisionsRow, error) {
	found, err := sqlcgen.New(s.pool).ListStudySentenceDecisions(ctx, sqlcgen.ListStudySentenceDecisionsParams{
		Owner: owner, Book: book, AnalysisRun: run, Corpus: corpus, SentenceOrdinals: ordinals,
	})
	if err != nil {
		return nil, fmt.Errorf("list Study sentence decisions: %w", err)
	}
	byToken := make(map[[2]int64]sqlcgen.ListStudySentenceDecisionsRow, len(found))
	for _, row := range found {
		byToken[[2]int64{row.SentenceOrdinal, row.TokenOrdinal}] = row
	}
	return byToken, nil
}

// ListStudyEffectiveTokens returns the tokens of the sentences of one Book's
// analysis with the learner's decisions applied, by sentence ordinal: a
// confirmed Lemma correction supplies the lemma and an excluded Occurrence
// matches nothing. They are the tokens Idiom matching reads (ADR 0137).
func (s *PostgresStore) ListStudyEffectiveTokens(ctx context.Context, owner, book, run, corpus string, ordinals []int64) (map[int64][]analyzer.Token, error) {
	if !validUUID(book) || !validUUID(run) || !validUUID(corpus) {
		return map[int64][]analyzer.Token{}, nil
	}
	sentences, err := s.ListCorpusSentences(ctx, owner, corpus, ordinals)
	if err != nil {
		return nil, err
	}
	decisions, err := s.studySentenceDecisions(ctx, owner, book, run, corpus, ordinals)
	if err != nil {
		return nil, err
	}
	result := make(map[int64][]analyzer.Token, len(sentences))
	for ordinal, sentence := range sentences {
		effective := make([]analyzer.Token, len(sentence.Tokens))
		for i, token := range sentence.Tokens {
			effective[i] = effectiveStudyToken(token, decisions, ordinal, int64(i))
		}
		result[ordinal] = effective
	}
	return result, nil
}

// effectiveStudyToken applies the Book's decision on the token, if any: an
// excluded Occurrence keeps its position but matches nothing, and a confirmed
// correction replaces the lemma.
func effectiveStudyToken(token analyzer.Token, decisions map[[2]int64]sqlcgen.ListStudySentenceDecisionsRow, sentence, ordinal int64) analyzer.Token {
	decision, ok := decisions[[2]int64{sentence, ordinal}]
	if !ok {
		return token
	}
	if decision.DecisionExcluded {
		return analyzer.Token{}
	}
	if lemma := strings.TrimSpace(decision.DecisionLemma.String); lemma != "" {
		token.CanonicalLemma = lemma
	}
	return token
}

func distinctLowerWords(words []string) []string {
	seen := make(map[string]bool, len(words))
	result := make([]string, 0, len(words))
	for _, word := range words {
		word = strings.ToLower(strings.TrimSpace(word))
		if word != "" && !seen[word] {
			seen[word] = true
			result = append(result, word)
		}
	}
	return result
}

// cachedSnapshotStructure is the recorded structure of the sentence's snapshot,
// read once per snapshot.
func (s *PostgresStore) cachedSnapshotStructure(ctx context.Context, owner string, cache map[[2]string]domain.SnapshotStructure, ref StudyIdiomSentenceRef) (domain.SnapshotStructure, error) {
	key := [2]string{ref.sourceID, ref.snapshotID}
	if structure, ok := cache[key]; ok {
		return structure, nil
	}
	structure, err := LoadSnapshotStructure(ctx, s.pool, owner, ref.sourceID, ref.snapshotID)
	if err != nil {
		return domain.SnapshotStructure{}, err
	}
	cache[key] = structure
	return structure, nil
}
