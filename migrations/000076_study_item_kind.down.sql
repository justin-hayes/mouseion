-- Reverse 000076_study_item_kind. The pre-kind schema cannot represent an
-- idiom item (it has no upos), so every row of an idiom and every row that
-- refers to one is deleted before the kind is dropped; word rows are kept
-- unchanged. As with every down migration here, this is not assumed safe for
-- destructive production rollback.

DELETE FROM public.study_fluency_response r
 USING public.study_fluency_context c
 WHERE r.owner_id = c.owner_id AND r.context_id = c.id AND c.item_kind = 'idiom';
DELETE FROM public.study_fluency_context WHERE item_kind = 'idiom';
DELETE FROM public.study_report WHERE item_kind = 'idiom';
DELETE FROM public.study_review_log l
 USING public.study_progress p
 WHERE l.owner_id = p.owner_id AND l.progress_id = p.id AND p.item_kind = 'idiom';
DELETE FROM public.study_progress WHERE item_kind = 'idiom';
DELETE FROM public.study_review_log l
 USING public.study_material m
 WHERE l.owner_id = m.owner_id AND l.material_id = m.id AND m.item_kind = 'idiom';
DELETE FROM public.study_material WHERE item_kind = 'idiom';

ALTER TABLE public.study_report
    DROP CONSTRAINT study_report_progress_fkey,
    ADD CONSTRAINT study_report_progress_fkey FOREIGN KEY (owner_id, progress_id) REFERENCES public.study_progress(owner_id, id) ON DELETE SET NULL (progress_id),
    DROP CONSTRAINT study_report_material_fkey,
    ADD CONSTRAINT study_report_material_fkey FOREIGN KEY (owner_id, material_id) REFERENCES public.study_material(owner_id, id);

ALTER TABLE public.study_fluency_context
    DROP CONSTRAINT study_fluency_context_progress_fkey,
    ADD CONSTRAINT study_fluency_context_progress_fkey FOREIGN KEY (owner_id, progress_id) REFERENCES public.study_progress(owner_id, id);

ALTER TABLE public.study_progress
    DROP CONSTRAINT study_progress_material_fkey,
    ADD CONSTRAINT study_progress_material_fkey FOREIGN KEY (owner_id, material_id) REFERENCES public.study_material(owner_id, id);

ALTER TABLE public.study_progress DROP CONSTRAINT study_progress_kind_key;
ALTER TABLE public.study_material DROP CONSTRAINT study_material_kind_key;

DROP INDEX public.study_fluency_context_occurrence_idx;
CREATE UNIQUE INDEX study_fluency_context_occurrence_idx
    ON public.study_fluency_context(owner_id, progress_id, analysis_run_id, sentence_ordinal, token_ordinal);

DROP INDEX public.study_report_open_wrong_word_idx;
CREATE UNIQUE INDEX study_report_open_wrong_word_idx
    ON public.study_report(owner_id, language, canonical_lemma, upos, lexical_discriminator)
    WHERE kind = 'wrong_word' AND ended_at IS NULL;

DROP INDEX public.study_progress_word_idx;
CREATE UNIQUE INDEX study_progress_word_idx
    ON public.study_progress(owner_id, language, canonical_lemma, upos, lexical_discriminator);

DROP INDEX public.study_material_word_evidence_idx;
CREATE UNIQUE INDEX study_material_word_evidence_idx
    ON public.study_material(owner_id, language, canonical_lemma, upos, lexical_discriminator, analysis_run_id, sentence_ordinal, token_ordinal)
    WHERE state = 'ready' AND superseded_at IS NULL;

DROP INDEX public.study_material_current_word_idx;
CREATE UNIQUE INDEX study_material_current_word_idx
    ON public.study_material(owner_id, language, canonical_lemma, upos, lexical_discriminator)
    WHERE superseded_at IS NULL;

ALTER TABLE public.study_fluency_context
    DROP CONSTRAINT study_fluency_context_item_kind_check,
    DROP COLUMN item_kind;

ALTER TABLE public.study_report
    DROP CONSTRAINT study_report_identity_check,
    DROP CONSTRAINT study_report_item_kind_check,
    DROP COLUMN item_kind,
    ADD CONSTRAINT study_report_identity_check CHECK ((btrim(language) <> ''::text AND btrim(canonical_lemma) <> ''::text AND btrim(upos) <> ''::text));

ALTER TABLE public.study_progress
    DROP CONSTRAINT study_progress_identity_check,
    DROP CONSTRAINT study_progress_item_kind_check,
    DROP COLUMN item_kind,
    ADD CONSTRAINT study_progress_identity_check CHECK ((btrim(language) <> ''::text AND btrim(canonical_lemma) <> ''::text AND btrim(upos) <> ''::text));

ALTER TABLE public.study_material
    DROP CONSTRAINT study_material_identity_check,
    DROP CONSTRAINT study_material_item_kind_check,
    DROP COLUMN item_kind,
    ADD CONSTRAINT study_material_identity_check CHECK ((btrim(language) <> ''::text AND btrim(canonical_lemma) <> ''::text AND btrim(upos) <> ''::text));
