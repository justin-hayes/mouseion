-- Study for source-evidence languages (ADR 0138, spec #2759, ticket #2761):
-- Ancient Greek (and later Latin) Study material cites the dictionary senses its
-- Gloss rests on, and the Learned meaning keeps those citations and the
-- dictionary pin they were made against, so a later pin change never
-- re-introduces a word. Every column is additive with an empty default: the
-- citation columns are empty for modern-language material, whose Learned
-- meaning stays a Gloss alone, and the passage columns are empty when no
-- passage applies. No row is rewritten.
--
-- The citation columns are the source's native sense IDs, the native entry IDs
-- they belong to and the pinned dictionary revision. Sense IDs and the pin are
-- set together: a row with one has the other, and a row with entry IDs has
-- sense IDs. The database cannot see a language's card policy, so whether a
-- ready source-evidence row must carry them is enforced where it is recorded.
-- What the database can enforce is the shape of a ready row that does carry a
-- pin: at least one cited sense, and the entries those senses belong to (one
-- per entry, never more than the senses).

ALTER TABLE public.study_material
    ADD COLUMN cited_sense_ids text[] DEFAULT '{}'::text[] NOT NULL,
    ADD COLUMN cited_entry_ids text[] DEFAULT '{}'::text[] NOT NULL,
    ADD COLUMN dictionary_pin text DEFAULT ''::text NOT NULL,
    ADD COLUMN passage_context text DEFAULT ''::text NOT NULL,
    ADD COLUMN passage_citation text DEFAULT ''::text NOT NULL,
    ADD COLUMN passage_translation_pin text DEFAULT ''::text NOT NULL;

ALTER TABLE public.study_material
    ADD CONSTRAINT study_material_citations_check CHECK (((cardinality(cited_sense_ids) > 0) = (btrim(dictionary_pin) <> ''::text) AND (cardinality(cited_entry_ids) = 0 OR cardinality(cited_sense_ids) > 0))),
    ADD CONSTRAINT study_material_ready_citations_check CHECK ((state <> 'ready'::text OR btrim(dictionary_pin) = ''::text OR (cardinality(cited_sense_ids) > 0 AND cardinality(cited_entry_ids) > 0 AND cardinality(cited_entry_ids) <= cardinality(cited_sense_ids)))),
    ADD CONSTRAINT study_material_passage_check CHECK (((btrim(passage_context) <> ''::text) = (btrim(passage_citation) <> ''::text) AND (btrim(passage_context) <> ''::text) = (btrim(passage_translation_pin) <> ''::text)));

-- The Learned meaning's citations, copied from the material at introduction and
-- at every re-anchoring.
ALTER TABLE public.study_progress
    ADD COLUMN learned_sense_ids text[] DEFAULT '{}'::text[] NOT NULL,
    ADD COLUMN learned_entry_ids text[] DEFAULT '{}'::text[] NOT NULL,
    ADD COLUMN learned_dictionary_pin text DEFAULT ''::text NOT NULL;

ALTER TABLE public.study_progress
    ADD CONSTRAINT study_progress_learned_citations_check CHECK (((cardinality(learned_sense_ids) > 0) = (btrim(learned_dictionary_pin) <> ''::text) AND (cardinality(learned_entry_ids) = 0 OR cardinality(learned_sense_ids) > 0)));

-- What Undo restores of the Learned meaning's citations beside
-- previous_learned_meaning: null when the report changed none of them.
ALTER TABLE public.study_report
    ADD COLUMN previous_learned_sense_ids text[],
    ADD COLUMN previous_learned_entry_ids text[],
    ADD COLUMN previous_learned_dictionary_pin text;

-- A Fluency context's citations. Fluency fills them (ticket #2763); until then
-- they are empty.
ALTER TABLE public.study_fluency_context
    ADD COLUMN cited_sense_ids text[] DEFAULT '{}'::text[] NOT NULL,
    ADD COLUMN cited_entry_ids text[] DEFAULT '{}'::text[] NOT NULL,
    ADD COLUMN dictionary_pin text DEFAULT ''::text NOT NULL;

ALTER TABLE public.study_fluency_context
    ADD CONSTRAINT study_fluency_context_citations_check CHECK (((cardinality(cited_sense_ids) > 0) = (btrim(dictionary_pin) <> ''::text) AND (cardinality(cited_entry_ids) = 0 OR cardinality(cited_sense_ids) > 0)));

-- Forward-only in production (ADR 0038, ADR 0070). The down migration is a
-- development convenience, not a supported production rollback.
