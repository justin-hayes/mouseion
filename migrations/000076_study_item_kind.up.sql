-- Study item kind (ADR 0137 §1, spec #2748, ticket #2749). Study's identity
-- becomes a Study item with two kinds: a word, keyed by language, canonical
-- lemma, UPOS and lexical discriminator, and an idiom, keyed by language and
-- headword. Study progress, Study material, Fluency contexts and Study reports
-- all carry the kind, and every uniqueness rule that applied per identity now
-- applies per kind and key, so a word and an idiom with the same text can never
-- collide.
--
-- An idiom row keeps its headword in canonical_lemma and leaves upos and
-- lexical_discriminator empty. The identity checks become kind-aware: a word
-- still requires a non-empty upos, an idiom requires both to be empty.
--
-- This migration is structural DDL only; ADR 0038's separate-backfill rule has
-- nothing to separate. Every existing row is a word, which is the constant
-- column default, so Postgres fills it without a rewrite and no data backfill
-- exists. The migration is idempotent in effect (a second run fails on the
-- existing column and changes nothing), and the schema adds no idiom
-- behaviour: only word items are created until a later ticket.

ALTER TABLE public.study_material ADD COLUMN item_kind text DEFAULT 'word'::text NOT NULL;
ALTER TABLE public.study_progress ADD COLUMN item_kind text DEFAULT 'word'::text NOT NULL;
ALTER TABLE public.study_fluency_context ADD COLUMN item_kind text DEFAULT 'word'::text NOT NULL;
ALTER TABLE public.study_report ADD COLUMN item_kind text DEFAULT 'word'::text NOT NULL;

ALTER TABLE public.study_material
    ADD CONSTRAINT study_material_item_kind_check CHECK ((item_kind = ANY (ARRAY['word'::text, 'idiom'::text]))),
    DROP CONSTRAINT study_material_identity_check,
    ADD CONSTRAINT study_material_identity_check CHECK ((btrim(language) <> ''::text AND btrim(canonical_lemma) <> ''::text AND ((item_kind = 'word'::text AND btrim(upos) <> ''::text) OR (item_kind = 'idiom'::text AND upos = ''::text AND lexical_discriminator = ''::text))));

ALTER TABLE public.study_progress
    ADD CONSTRAINT study_progress_item_kind_check CHECK ((item_kind = ANY (ARRAY['word'::text, 'idiom'::text]))),
    DROP CONSTRAINT study_progress_identity_check,
    ADD CONSTRAINT study_progress_identity_check CHECK ((btrim(language) <> ''::text AND btrim(canonical_lemma) <> ''::text AND ((item_kind = 'word'::text AND btrim(upos) <> ''::text) OR (item_kind = 'idiom'::text AND upos = ''::text AND lexical_discriminator = ''::text))));

ALTER TABLE public.study_report
    ADD CONSTRAINT study_report_item_kind_check CHECK ((item_kind = ANY (ARRAY['word'::text, 'idiom'::text]))),
    DROP CONSTRAINT study_report_identity_check,
    ADD CONSTRAINT study_report_identity_check CHECK ((btrim(language) <> ''::text AND btrim(canonical_lemma) <> ''::text AND ((item_kind = 'word'::text AND btrim(upos) <> ''::text) OR (item_kind = 'idiom'::text AND upos = ''::text AND lexical_discriminator = ''::text))));

ALTER TABLE public.study_fluency_context
    ADD CONSTRAINT study_fluency_context_item_kind_check CHECK ((item_kind = ANY (ARRAY['word'::text, 'idiom'::text])));

-- The uniqueness rules keyed by identity gain the kind.
DROP INDEX public.study_material_current_word_idx;
CREATE UNIQUE INDEX study_material_current_word_idx
    ON public.study_material(owner_id, item_kind, language, canonical_lemma, upos, lexical_discriminator)
    WHERE superseded_at IS NULL;

DROP INDEX public.study_material_word_evidence_idx;
CREATE UNIQUE INDEX study_material_word_evidence_idx
    ON public.study_material(owner_id, item_kind, language, canonical_lemma, upos, lexical_discriminator, analysis_run_id, sentence_ordinal, token_ordinal)
    WHERE state = 'ready' AND superseded_at IS NULL;

DROP INDEX public.study_progress_word_idx;
CREATE UNIQUE INDEX study_progress_word_idx
    ON public.study_progress(owner_id, item_kind, language, canonical_lemma, upos, lexical_discriminator);

DROP INDEX public.study_report_open_wrong_word_idx;
CREATE UNIQUE INDEX study_report_open_wrong_word_idx
    ON public.study_report(owner_id, item_kind, language, canonical_lemma, upos, lexical_discriminator)
    WHERE kind = 'wrong_word' AND ended_at IS NULL;

DROP INDEX public.study_fluency_context_occurrence_idx;
CREATE UNIQUE INDEX study_fluency_context_occurrence_idx
    ON public.study_fluency_context(owner_id, progress_id, item_kind, analysis_run_id, sentence_ordinal, token_ordinal);

-- A row that refers to material or progress carries that row's kind: the
-- references below include the kind, so the two can never disagree.
ALTER TABLE public.study_material ADD CONSTRAINT study_material_kind_key UNIQUE (owner_id, id, item_kind);
ALTER TABLE public.study_progress ADD CONSTRAINT study_progress_kind_key UNIQUE (owner_id, id, item_kind);

ALTER TABLE public.study_progress
    DROP CONSTRAINT study_progress_material_fkey,
    ADD CONSTRAINT study_progress_material_fkey FOREIGN KEY (owner_id, material_id, item_kind) REFERENCES public.study_material(owner_id, id, item_kind);

ALTER TABLE public.study_fluency_context
    DROP CONSTRAINT study_fluency_context_progress_fkey,
    ADD CONSTRAINT study_fluency_context_progress_fkey FOREIGN KEY (owner_id, progress_id, item_kind) REFERENCES public.study_progress(owner_id, id, item_kind);

ALTER TABLE public.study_report
    DROP CONSTRAINT study_report_material_fkey,
    ADD CONSTRAINT study_report_material_fkey FOREIGN KEY (owner_id, material_id, item_kind) REFERENCES public.study_material(owner_id, id, item_kind),
    DROP CONSTRAINT study_report_progress_fkey,
    ADD CONSTRAINT study_report_progress_fkey FOREIGN KEY (owner_id, progress_id, item_kind) REFERENCES public.study_progress(owner_id, id, item_kind) ON DELETE SET NULL (progress_id);

COMMENT ON COLUMN public.study_material.item_kind IS 'Study item kind: word keyed by lemma, UPOS and discriminator, idiom keyed by headword in canonical_lemma';
COMMENT ON COLUMN public.study_progress.item_kind IS 'Study item kind: word keyed by lemma, UPOS and discriminator, idiom keyed by headword in canonical_lemma';
COMMENT ON COLUMN public.study_fluency_context.item_kind IS 'Kind of the Study item the context belongs to; equals the progress row''s kind';
COMMENT ON COLUMN public.study_report.item_kind IS 'Study item kind: word keyed by lemma, UPOS and discriminator, idiom keyed by headword in canonical_lemma';
