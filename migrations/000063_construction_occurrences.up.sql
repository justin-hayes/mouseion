-- ADR 0123: Construction occurrences are a derived reading of a native_agdt
-- run's stored syntax. They are rebuilt from corpus_tokens and
-- corpus_unprinted_syntax_nodes, never edited by the learner, and never an
-- input to lemma corrections or Occurrence exclusions.
--
--   * construction_occurrences holds one row per occurrence: identifiers only.
--     The note name, definition and citation are looked up at render, so no
--     text, name or version is stored per row. language is denormalised from the
--     run so Concordance can filter without a join.
--   * construction_run_markers records the detector version that built a run's
--     rows. A reader uses a run's rows only when the marker equals the current
--     detector version; no marker, or an older one, means show nothing.
--
-- Structural DDL only, with no backfill: existing runs get no rows and no
-- marker until the detector-version rebuild fills them (#2624). Forward-only in
-- production (ADR 0038, ADR 0070).
-- An extent is valid when it is non-empty, has no NULL element, is sorted, starts
-- at a non-negative ordinal and contains the anchor. The result is never NULL, so
-- a CHECK on it cannot pass a malformed array by evaluating to NULL.
CREATE FUNCTION public.construction_extent_valid(extent bigint[], anchor bigint) RETURNS boolean
    LANGUAGE sql IMMUTABLE
    AS $$
    SELECT COALESCE(
        cardinality(extent) > 0
        AND array_position(extent, NULL) IS NULL
        AND extent = ARRAY(SELECT x FROM unnest(extent) AS x ORDER BY x)
        AND extent[1] >= 0
        AND anchor = ANY (extent),
        false)
$$;

CREATE TABLE public.construction_occurrences (
    owner_id uuid NOT NULL,
    corpus_id uuid NOT NULL,
    analysis_run_id uuid NOT NULL,
    sentence_ordinal bigint NOT NULL,
    language text NOT NULL,
    construction_id text NOT NULL,
    -- NULL when the shipping gate does not label the subtype.
    subtype_id text,
    anchor_token_ordinal bigint NOT NULL,
    -- Sorted printed token ordinals, containing the anchor. A derived array, so
    -- its members carry no foreign key.
    extent_token_ordinals bigint[] NOT NULL,
    CONSTRAINT construction_occurrences_pkey PRIMARY KEY (analysis_run_id, sentence_ordinal, anchor_token_ordinal, construction_id),
    CONSTRAINT construction_occurrences_language_check CHECK (language <> ''),
    CONSTRAINT construction_occurrences_construction_id_check CHECK (construction_id <> ''),
    CONSTRAINT construction_occurrences_subtype_id_check CHECK (subtype_id IS NULL OR subtype_id <> ''),
    CONSTRAINT construction_occurrences_anchor_check CHECK (anchor_token_ordinal >= 0),
    CONSTRAINT construction_occurrences_extent_check CHECK (public.construction_extent_valid(extent_token_ordinals, anchor_token_ordinal)),
    -- Owner-scoped and cascading exactly like corpus_tokens.
    CONSTRAINT construction_occurrences_sentence_fkey
        FOREIGN KEY (owner_id, corpus_id, analysis_run_id, sentence_ordinal)
        REFERENCES public.corpus_sentences(owner_id, corpus_id, analysis_run_id, sentence_ordinal) ON DELETE CASCADE,
    CONSTRAINT construction_occurrences_anchor_fkey
        FOREIGN KEY (analysis_run_id, sentence_ordinal, anchor_token_ordinal)
        REFERENCES public.corpus_tokens(analysis_run_id, sentence_ordinal, token_ordinal) ON DELETE CASCADE
);

CREATE INDEX construction_occurrences_lookup_idx
    ON public.construction_occurrences (owner_id, language, construction_id, subtype_id);

CREATE TABLE public.construction_run_markers (
    owner_id uuid NOT NULL,
    corpus_id uuid NOT NULL,
    analysis_run_id uuid NOT NULL,
    detector_version integer NOT NULL,
    built_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT construction_run_markers_pkey PRIMARY KEY (analysis_run_id),
    CONSTRAINT construction_run_markers_detector_version_check CHECK (detector_version > 0),
    CONSTRAINT construction_run_markers_corpus_fkey
        FOREIGN KEY (owner_id, corpus_id, analysis_run_id)
        REFERENCES public.corpora(owner_id, id, analysis_run_id) ON DELETE CASCADE
);
