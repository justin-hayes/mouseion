//go:build integration

package persistence

import (
	"context"
	"testing"

	"github.com/justin-hayes/mouseion/internal/domain"
	"github.com/stretchr/testify/require"
)

// TestStudyItemKindKeepsAWordAndAnIdiomApart holds that a word and an Idiom
// with the same key text never collide in a uniqueness rule, and that the
// tables refuse a row whose kind disagrees with its key or with the row it
// refers to (ADR 0137 §1, ticket #2749). The Idiom rows are written through SQL
// because only word items are created until a later ticket.
func TestStudyItemKindKeepsAWordAndAnIdiomApart(t *testing.T) {
	ctx := context.Background()
	store := openIntegrationStore(t, ctx, integrationDatabase(t, ctx))
	owner, err := store.CreateUser(ctx, "study-item-kind", false)
	require.NoError(t, err)

	word := domain.WordItem("de", "haus", "NOUN", "")
	idiom := domain.IdiomItem("de", "haus")

	wordMaterial, created, err := store.ReserveStudyMaterial(ctx, owner.ID, word)
	require.NoError(t, err)
	require.True(t, created)
	idiomMaterial, created, err := store.ReserveStudyMaterial(ctx, owner.ID, idiom)
	require.NoError(t, err)
	require.True(t, created, "an Idiom is not the word with the same text")
	require.NotEqual(t, wordMaterial, idiomMaterial)

	_, created, err = store.ReserveStudyMaterial(ctx, owner.ID, word)
	require.NoError(t, err)
	require.False(t, created, "a word still has one current material")
	_, created, err = store.ReserveStudyMaterial(ctx, owner.ID, idiom)
	require.NoError(t, err)
	require.False(t, created, "an Idiom still has one current material")

	kinds := map[string]domain.StudyItemKind{}
	material, err := store.ListCurrentStudyMaterial(ctx, owner.ID, "de")
	require.NoError(t, err)
	for _, item := range material {
		kinds[item.ID] = item.ItemKind
	}
	require.Equal(t, map[string]domain.StudyItemKind{wordMaterial: domain.StudyItemWord, idiomMaterial: domain.StudyItemIdiom}, kinds)

	_, _, err = store.ReserveStudyMaterial(ctx, owner.ID, domain.StudyItem{ItemKind: domain.StudyItemIdiom, Language: "de", CanonicalLemma: "haus", UPOS: "NOUN"})
	require.ErrorIs(t, err, domain.ErrInvalidStudyMaterial, "an Idiom has no part of speech")
	_, _, err = store.ReserveStudyMaterial(ctx, owner.ID, domain.WordItem("de", "haus", "", ""))
	require.ErrorIs(t, err, domain.ErrInvalidStudyMaterial, "a word needs a part of speech")

	exec := func(sql string, args ...any) error {
		_, err := store.Pool().Exec(ctx, sql, args...)
		return err
	}
	const insertMaterial = `INSERT INTO study_material (owner_id, item_kind, language, canonical_lemma, upos, lexical_discriminator, state) VALUES ($1,$2,'de','x',$3,$4,'pending')`
	require.Error(t, exec(insertMaterial, owner.ID, "idiom", "NOUN", ""), "an Idiom row has an empty upos")
	require.Error(t, exec(insertMaterial, owner.ID, "idiom", "", "d"), "an Idiom row has an empty discriminator")
	require.Error(t, exec(insertMaterial, owner.ID, "word", "", ""), "a word row has a upos")
	require.Error(t, exec(insertMaterial, owner.ID, "phrase", "NOUN", ""), "the kind is word or idiom")

	const insertProgress = `INSERT INTO study_progress (owner_id, item_kind, language, canonical_lemma, upos, lexical_discriminator, material_id, introduced_at, learned_meaning, due)
		VALUES ($1,$2,'de','haus',$3,'',$4,now(),'house',now())`
	require.NoError(t, exec(insertProgress, owner.ID, "word", "NOUN", wordMaterial))
	require.NoError(t, exec(insertProgress, owner.ID, "idiom", "", idiomMaterial), "an Idiom with the word's key text has its own progress")
	require.Error(t, exec(insertProgress, owner.ID, "word", "NOUN", wordMaterial), "a word has one progress row")
	require.Error(t, exec(insertProgress, owner.ID, "idiom", "", idiomMaterial), "an Idiom has one progress row")

	require.Error(t, exec(`INSERT INTO study_progress (owner_id, item_kind, language, canonical_lemma, upos, material_id, introduced_at, learned_meaning, due)
		VALUES ($1,'idiom','de','other','',$2,now(),'x',now())`, owner.ID, wordMaterial), "progress carries its material's kind")

	var wordProgress, idiomProgress string
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT id::text FROM study_progress WHERE owner_id=$1 AND item_kind='word'`, owner.ID).Scan(&wordProgress))
	require.NoError(t, store.Pool().QueryRow(ctx, `SELECT id::text FROM study_progress WHERE owner_id=$1 AND item_kind='idiom'`, owner.ID).Scan(&idiomProgress))

	book, source := createConcordanceBook(t, ctx, store, owner.ID, "Kinds", "study-item-kind", false, []domain.ExtractedUnit{
		concordanceUnit(0, "chapter", "Das Haus steht.", 0, 15),
	})
	insertConcordanceAnalysis(t, ctx, store, source, true, []concordanceSentence{
		{UnitID: "epub-unit-v1:0:chapter", Ordinal: 0, Text: "Das Haus steht.", Start: 0, End: 15, Tokens: []concordanceToken{
			{Surface: "Haus", Lemma: "haus", Upos: "NOUN", Dependency: "root", Head: 0, Morphology: map[string]string{}, Start: 4, End: 8},
		}},
	})
	corpus, run := currentSentenceStudyAnalysis(t, ctx, store, owner.ID, book.ID)

	const insertContext = `INSERT INTO study_fluency_context (owner_id, progress_id, item_kind, agreement, reason, book_id, analysis_run_id, corpus_id, sentence_ordinal, token_ordinal, sentence, target_word, gloss, sentence_translation)
		VALUES ($1,$2,$3,'agrees','same-meaning',$4,$5,$6,0,0,'Das Haus steht.','Haus','house','The house stands.')`
	require.NoError(t, exec(insertContext, owner.ID, wordProgress, "word", book.ID, run, corpus))
	require.Error(t, exec(insertContext, owner.ID, wordProgress, "word", book.ID, run, corpus), "an occurrence is assessed once per item")
	require.Error(t, exec(insertContext, owner.ID, wordProgress, "idiom", book.ID, run, corpus), "a context carries its progress's kind")
	require.NoError(t, exec(insertContext, owner.ID, idiomProgress, "idiom", book.ID, run, corpus))

	const insertReport = `INSERT INTO study_report (owner_id, item_kind, language, canonical_lemma, upos, lexical_discriminator, progress_id, material_id, kind,
			book_id, analysis_run_id, corpus_id, sentence_ordinal, token_ordinal, sentence, target_word, gloss, sentence_translation, created_at)
		VALUES ($1,$2,'de','haus',$3,'',$4,$5,'wrong_word',$6,$7,$8,0,0,'Das Haus steht.','Haus','house','The house stands.',now())`
	require.NoError(t, exec(insertReport, owner.ID, "word", "NOUN", wordProgress, wordMaterial, book.ID, run, corpus))
	require.Error(t, exec(`INSERT INTO study_report (owner_id, item_kind, language, canonical_lemma, upos, progress_id, material_id, kind,
			book_id, analysis_run_id, corpus_id, sentence_ordinal, token_ordinal, sentence, target_word, gloss, sentence_translation, created_at)
		VALUES ($1,'word','de','other','NOUN',NULL,$2,'unsuitable_context',$3,$4,$5,0,0,'s','t','g','x',now())`, owner.ID, idiomMaterial, book.ID, run, corpus), "a report carries its material's kind")
	require.Error(t, exec(insertReport, owner.ID, "idiom", "", wordProgress, idiomMaterial, book.ID, run, corpus), "a report carries its progress's kind")
	require.NoError(t, exec(insertReport, owner.ID, "idiom", "", idiomProgress, idiomMaterial, book.ID, run, corpus), "an Idiom with the word's key text is held on its own")
}
