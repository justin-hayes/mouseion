-- Study reports (ADR 0135, spec #2714). A learner reports, from Today, that a
-- word's Study material is wrong. Owned by learner and study language, and
-- keyed by the word's full vocabulary identity through its Study progress.
--
-- kind is wrong_word (the occurrence belongs to another vocabulary identity;
-- the word is held while the report is open) or unsuitable_context (the
-- identity is right but the context does not teach the word; the word is
-- re-introduced from another context). A report is open while ended_at is
-- null. Its end_reason says how it ended: corrected and excluded (a Lemma
-- correction or Occurrence exclusion was confirmed on the occurrence),
-- analysis_update (an Analysis update gave the occurrence another identity),
-- occurrence_gone (the occurrence no longer exists), dismissed (the learner
-- dismissed a Wrong-word report), undone (the learner undid an
-- Unsuitable-context report) and retired (the retired context is gone for
-- good). ended_at is set if and only if end_reason is.
--
-- The reported occurrence (Book, analysis run, corpus, sentence and token),
-- sentence, target word, Gloss and sentence translation are copied from the
-- reported Study material when the report is made. The source occurrence is
-- plain data, not a foreign key: a replaced analysis must not delete a
-- sentence the learner has been shown. The rejected context's Gloss and
-- translation are evidence, as Fluency keeps its disagreeing contexts.
--
-- An open Unsuitable-context report is also the word's retired
-- (word, occurrence) pair: that sentence is never used again as this word's
-- Study material or Fluency context. Its occurrence columns are therefore
-- always present.
--
-- created_at is when the learner made the report, on the application's clock,
-- and recorded_at is the database clock at insert. Undo refuses once any Study
-- action followed the report, and compares recorded_at with the times the
-- database stamps on later actions (review log, batch ledger, Fluency response
-- and report rows), so both sides of the comparison are the database's clock.
--
-- previous_material_id, previous_corrected_at and previous_learned_meaning
-- are the state Undo restores when an Unsuitable-context report re-introduced
-- the word: the material it had, when it was last corrected, and the meaning
-- it had learned. They are null when the report changed none of them.
CREATE TABLE public.study_report (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    owner_id uuid NOT NULL,
    language text NOT NULL,
    canonical_lemma text NOT NULL,
    upos text NOT NULL,
    lexical_discriminator text DEFAULT ''::text NOT NULL,
    progress_id uuid NOT NULL,
    material_id uuid NOT NULL,
    kind text NOT NULL,
    note text DEFAULT ''::text NOT NULL,
    book_id uuid NOT NULL,
    analysis_run_id uuid NOT NULL,
    corpus_id uuid NOT NULL,
    sentence_ordinal bigint NOT NULL,
    token_ordinal bigint NOT NULL,
    sentence text NOT NULL,
    target_word text NOT NULL,
    gloss text NOT NULL,
    sentence_translation text NOT NULL,
    previous_material_id uuid,
    previous_corrected_at timestamptz,
    previous_learned_meaning text,
    created_at timestamptz NOT NULL,
    recorded_at timestamptz DEFAULT now() NOT NULL,
    ended_at timestamptz,
    end_reason text,
    CONSTRAINT study_report_pkey PRIMARY KEY (owner_id, id),
    CONSTRAINT study_report_owner_fkey FOREIGN KEY (owner_id) REFERENCES public.users(id) ON DELETE CASCADE,
    CONSTRAINT study_report_progress_fkey FOREIGN KEY (owner_id, progress_id) REFERENCES public.study_progress(owner_id, id),
    CONSTRAINT study_report_material_fkey FOREIGN KEY (owner_id, material_id) REFERENCES public.study_material(owner_id, id),
    CONSTRAINT study_report_previous_material_fkey FOREIGN KEY (owner_id, previous_material_id) REFERENCES public.study_material(owner_id, id),
    CONSTRAINT study_report_kind_check CHECK ((kind = ANY (ARRAY['wrong_word'::text, 'unsuitable_context'::text]))),
    CONSTRAINT study_report_identity_check CHECK ((btrim(language) <> ''::text AND btrim(canonical_lemma) <> ''::text AND btrim(upos) <> ''::text)),
    CONSTRAINT study_report_note_check CHECK ((char_length(note) <= 500)),
    CONSTRAINT study_report_context_check CHECK ((btrim(sentence) <> ''::text AND btrim(target_word) <> ''::text AND btrim(gloss) <> ''::text AND btrim(sentence_translation) <> ''::text)),
    CONSTRAINT study_report_end_reason_check CHECK (((end_reason IS NULL) OR (end_reason = ANY (ARRAY['corrected'::text, 'excluded'::text, 'analysis_update'::text, 'occurrence_gone'::text, 'dismissed'::text, 'undone'::text, 'retired'::text])))),
    CONSTRAINT study_report_end_pairing_check CHECK (((ended_at IS NULL) = (end_reason IS NULL)))
);

-- A word has at most one open Wrong-word report: it is held once.
CREATE UNIQUE INDEX study_report_open_wrong_word_idx
    ON public.study_report(owner_id, progress_id)
    WHERE kind = 'wrong_word' AND ended_at IS NULL;

-- Reporting the same material twice while a report is open yields one report.
CREATE UNIQUE INDEX study_report_open_material_idx
    ON public.study_report(owner_id, material_id, kind)
    WHERE ended_at IS NULL;

-- Open reports of a learner's study language: Today and Reported words.
CREATE INDEX study_report_open_idx
    ON public.study_report(owner_id, language, created_at)
    WHERE ended_at IS NULL;

-- Open reports of a Book, for its Lemma review, and the retired occurrences of
-- a word.
CREATE INDEX study_report_open_book_idx
    ON public.study_report(owner_id, book_id)
    WHERE ended_at IS NULL;

CREATE INDEX study_report_progress_idx
    ON public.study_report(owner_id, progress_id, created_at);

-- withdrawn_reason says why a word was withdrawn, when it is: null for a
-- withdrawal by a correction, and 'no_suitable_context' for a word whose every
-- context was reported as unsuitable. Additive and nullable. Its progress is
-- kept either way.
ALTER TABLE public.study_progress ADD COLUMN withdrawn_reason text;

ALTER TABLE public.study_progress ADD CONSTRAINT study_progress_withdrawn_reason_check
    CHECK (((withdrawn_reason IS NULL) OR (withdrawn_at IS NOT NULL AND withdrawn_reason = 'no_suitable_context'::text)));

-- Forward-only in production (ADR 0038, ADR 0070). The down migration is a
-- development convenience, not a supported production rollback.
