--
-- PostgreSQL database dump
--

-- Current-state baseline captured at pre-baseline commit
-- e6cae4a8753fcc446f1743a770d7a71b544fc3db, as specified by ADR 0070.
-- The active migration history starts here; retired history is preserved at
-- the migrations/pre-baseline Git tag.


-- Dumped from database version 16.15
-- Dumped by pg_dump version 16.15

SET statement_timeout = 0;
SET lock_timeout = 0;
SET idle_in_transaction_session_timeout = 0;
SET client_encoding = 'UTF8';
SET standard_conforming_strings = on;
SELECT pg_catalog.set_config('search_path', '', false);
SET check_function_bodies = false;
SET xmloption = content;
SET client_min_messages = warning;
SET row_security = off;

--
-- Name: pgcrypto; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS pgcrypto WITH SCHEMA public;


--
-- Name: EXTENSION pgcrypto; Type: COMMENT; Schema: -; Owner: -
--

COMMENT ON EXTENSION pgcrypto IS 'cryptographic functions';


--
-- Name: reject_deck_preparation_batch_chunk_identity_mutation(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.reject_deck_preparation_batch_chunk_identity_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF NEW.owner_id <> OLD.owner_id OR NEW.preparation_id <> OLD.preparation_id OR
    NEW.run_id <> OLD.run_id OR NEW.chunk_index <> OLD.chunk_index OR
    NEW.generation <> OLD.generation OR NEW.model <> OLD.model OR
    NEW.endpoint <> OLD.endpoint OR NEW.split_reason <> OLD.split_reason OR
    NEW.first_ordinal <> OLD.first_ordinal OR NEW.last_ordinal <> OLD.last_ordinal OR
    NEW.input_digest <> OLD.input_digest OR NEW.request_count <> OLD.request_count OR
    NEW.input_bytes <> OLD.input_bytes OR NEW.estimated_prompt_tokens <> OLD.estimated_prompt_tokens THEN
  RAISE EXCEPTION 'prepared-deck Batch chunk identity is immutable';
 END IF;
 RETURN NEW;
END;
$$;


--
-- Name: reject_deck_preparation_manifest_mutation(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.reject_deck_preparation_manifest_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 RAISE EXCEPTION 'prepared-deck manifests are immutable';
END;
$$;


--
-- Name: reject_deck_preparation_run_identity_mutation(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.reject_deck_preparation_run_identity_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF NEW.execution_mode <> OLD.execution_mode OR NEW.target_language <> OLD.target_language THEN
  RAISE EXCEPTION 'prepared-deck run execution identity is immutable';
 END IF;
 RETURN NEW;
END;
$$;


--
-- Name: reject_enrichment_cache_mutation(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.reject_enrichment_cache_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$ BEGIN RAISE EXCEPTION 'enrichment cache rows are immutable'; END; $$;


--
-- Name: reject_reviewed_scope_mutation(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.reject_reviewed_scope_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 RAISE EXCEPTION 'reviewed scope revisions are immutable';
END;
$$;


--
-- Name: reject_source_content_revision_mutation(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.reject_source_content_revision_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 RAISE EXCEPTION 'source content revisions are immutable';
END;
$$;


--
-- Name: reject_source_material_content_replacement(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.reject_source_material_content_replacement() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 IF OLD.content IS DISTINCT FROM NEW.content
    OR OLD.full_text IS DISTINCT FROM NEW.full_text
    OR OLD.content_hash IS DISTINCT FROM NEW.content_hash THEN
  RAISE EXCEPTION 'source content is immutable; create a new content revision';
 END IF;
 RETURN NEW;
END;
$$;


SET default_tablespace = '';

SET default_table_access_method = heap;

--
-- Name: analysis_jobs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.analysis_jobs (
    river_job_id bigint NOT NULL,
    owner_id uuid NOT NULL,
    source_material_id uuid NOT NULL,
    content_hash text NOT NULL,
    corpus_id uuid,
    progress smallint DEFAULT 0 NOT NULL,
    error text DEFAULT ''::text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    display_number bigint NOT NULL,
    analysis_run_id uuid,
    analysis_identity text,
    CONSTRAINT analysis_jobs_progress_check CHECK (((progress >= 0) AND (progress <= 100)))
);


--
-- Name: analysis_run_attempts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.analysis_run_attempts (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    run_id uuid NOT NULL,
    owner_id uuid NOT NULL,
    source_material_id uuid NOT NULL,
    attempt_number integer NOT NULL,
    river_job_id bigint NOT NULL,
    state text NOT NULL,
    error text DEFAULT ''::text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    started_at timestamptz,
    finalized_at timestamptz,
    CONSTRAINT analysis_run_attempts_attempt_number_check CHECK ((attempt_number > 0)),
    CONSTRAINT analysis_run_attempts_state_check CHECK ((state = ANY (ARRAY['queued'::text, 'running'::text, 'completed'::text, 'failed'::text, 'cancelled'::text])))
);


--
-- Name: analysis_runs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.analysis_runs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    owner_id uuid NOT NULL,
    source_material_id uuid NOT NULL,
    content_revision_id uuid NOT NULL,
    snapshot_id uuid NOT NULL,
    analyzer_name text NOT NULL,
    analyzer_version text NOT NULL,
    config_identity text NOT NULL,
    state text NOT NULL,
    attempt_count integer DEFAULT 0 NOT NULL,
    last_error text DEFAULT ''::text NOT NULL,
    corpus_id uuid,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    started_at timestamptz,
    completed_at timestamptz,
    CONSTRAINT analysis_runs_analyzer_name_check CHECK ((btrim(analyzer_name) <> ''::text)),
    CONSTRAINT analysis_runs_analyzer_version_check CHECK ((btrim(analyzer_version) <> ''::text)),
    CONSTRAINT analysis_runs_attempt_count_check CHECK ((attempt_count >= 0)),
    CONSTRAINT analysis_runs_config_identity_check CHECK ((btrim(config_identity) <> ''::text)),
    CONSTRAINT analysis_runs_state_check CHECK ((state = ANY (ARRAY['queued'::text, 'running'::text, 'completed'::text, 'failed'::text, 'cancelled'::text])))
);


--
-- Name: book_aliases; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.book_aliases (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    owner_id uuid NOT NULL,
    book_id uuid NOT NULL,
    alias_type text NOT NULL,
    namespace text NOT NULL,
    value text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    connection_id uuid,
    CONSTRAINT book_aliases_alias_type_check CHECK ((alias_type = ANY (ARRAY['catalog_entry'::text, 'strong_bibliographic'::text])))
);


--
-- Name: book_current_analyses; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.book_current_analyses (
    owner_id uuid NOT NULL,
    book_id uuid NOT NULL,
    source_material_id uuid NOT NULL,
    analysis_run_id uuid NOT NULL,
    promoted_at timestamptz DEFAULT now() NOT NULL
);


--
-- Name: book_membership; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.book_membership (
    owner_id uuid NOT NULL,
    book_id uuid NOT NULL,
    state text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    activated_at timestamptz,
    removed_at timestamptz,
    CONSTRAINT book_membership_check CHECK (((state = 'active'::text) = (removed_at IS NULL))),
    CONSTRAINT book_membership_state_check CHECK ((state = ANY (ARRAY['active'::text, 'removed'::text])))
);


--
-- Name: books; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.books (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    owner_id uuid NOT NULL,
    title text NOT NULL,
    metadata_provenance text NOT NULL,
    language_state text NOT NULL,
    language_tag text,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT books_check CHECK ((((language_state = 'chosen'::text) AND (language_tag IS NOT NULL) AND (language_tag <> ''::text)) OR ((language_state = 'unknown'::text) AND (language_tag IS NULL)))),
    CONSTRAINT books_language_state_check CHECK ((language_state = ANY (ARRAY['unknown'::text, 'chosen'::text]))),
    CONSTRAINT books_title_check CHECK ((title <> ''::text))
);


--
-- Name: cards; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cards (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    owner_id uuid NOT NULL,
    deck_id uuid NOT NULL,
    dedup_key text NOT NULL,
    canonical_lemma text NOT NULL,
    upos text NOT NULL,
    front text NOT NULL,
    back text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL
);


--
-- Name: catalogue_sync_status; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.catalogue_sync_status (
    owner_id uuid NOT NULL,
    connection_id uuid NOT NULL,
    state text NOT NULL,
    last_synced_at timestamptz,
    last_upserted_count integer DEFAULT 0 NOT NULL,
    last_error text DEFAULT ''::text NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT catalogue_sync_status_state_check CHECK ((state = ANY (ARRAY['syncing'::text, 'synced'::text, 'failed'::text])))
);


--
-- Name: COLUMN catalogue_sync_status.last_upserted_count; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.catalogue_sync_status.last_upserted_count IS 'Books added or updated by the most recent successful sync; zero means the collection was already current.';


--
-- Name: corpora; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.corpora (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    owner_id uuid NOT NULL,
    source_material_id uuid NOT NULL,
    artifact_hash text NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    analyzable_token_count bigint,
    distinct_lemma_count bigint,
    sentence_count bigint,
    normalized_token_count bigint,
    empty_sentence_count bigint,
    median_sentence_token_count double precision,
    p90_sentence_token_count bigint,
    long_sentence_count bigint,
    analysis_run_id uuid,
    CONSTRAINT corpora_analysis_statistics_complete CHECK (((analyzable_token_count IS NULL) = (distinct_lemma_count IS NULL))),
    CONSTRAINT corpora_analyzable_token_count_check CHECK ((analyzable_token_count >= 0)),
    CONSTRAINT corpora_distinct_lemma_count_check CHECK ((distinct_lemma_count >= 0)),
    CONSTRAINT corpora_empty_sentence_count_check CHECK ((empty_sentence_count >= 0)),
    CONSTRAINT corpora_long_sentence_count_check CHECK ((long_sentence_count >= 0)),
    CONSTRAINT corpora_median_sentence_token_count_check CHECK ((median_sentence_token_count >= (0)::double precision)),
    CONSTRAINT corpora_normalized_token_count_check CHECK ((normalized_token_count >= 0)),
    CONSTRAINT corpora_p90_sentence_token_count_check CHECK ((p90_sentence_token_count >= 0)),
    CONSTRAINT corpora_sentence_count_check CHECK ((sentence_count >= 0)),
    CONSTRAINT corpora_structural_statistics_complete CHECK ((((sentence_count IS NULL) = (normalized_token_count IS NULL)) AND ((sentence_count IS NULL) = (empty_sentence_count IS NULL)) AND ((sentence_count IS NULL) = (median_sentence_token_count IS NULL)) AND ((sentence_count IS NULL) = (p90_sentence_token_count IS NULL)) AND ((sentence_count IS NULL) = (long_sentence_count IS NULL)) AND ((sentence_count IS NULL) OR (empty_sentence_count <= sentence_count)) AND ((sentence_count IS NULL) OR (long_sentence_count <= sentence_count))))
);


--
-- Name: corpus_sentences; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.corpus_sentences (
    owner_id uuid NOT NULL,
    analysis_run_id uuid NOT NULL,
    corpus_id uuid NOT NULL,
    unit_id text NOT NULL,
    sentence_ordinal bigint NOT NULL,
    sentence_text text NOT NULL,
    start_offset bigint NOT NULL,
    end_offset bigint NOT NULL,
    CONSTRAINT corpus_sentences_check CHECK ((end_offset >= start_offset)),
    CONSTRAINT corpus_sentences_sentence_ordinal_check CHECK ((sentence_ordinal >= 0)),
    CONSTRAINT corpus_sentences_start_offset_check CHECK ((start_offset >= 0)),
    CONSTRAINT corpus_sentences_unit_id_check CHECK ((unit_id <> ''::text))
);


--
-- Name: corpus_tokens; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.corpus_tokens (
    owner_id uuid NOT NULL,
    language text NOT NULL,
    analysis_run_id uuid NOT NULL,
    corpus_id uuid NOT NULL,
    sentence_ordinal bigint NOT NULL,
    token_ordinal bigint NOT NULL,
    surface text NOT NULL,
    raw_lemma text NOT NULL,
    canonical_lemma text NOT NULL,
    upos text NOT NULL,
    morphology jsonb DEFAULT '{}'::jsonb NOT NULL,
    named_entity text,
    start_offset bigint NOT NULL,
    end_offset bigint NOT NULL,
    dependency text NOT NULL,
    head bigint NOT NULL,
    CONSTRAINT corpus_tokens_check CHECK ((end_offset >= start_offset)),
    CONSTRAINT corpus_tokens_dependency_check CHECK ((dependency <> ''::text)),
    CONSTRAINT corpus_tokens_head_check CHECK ((head >= 0)),
    CONSTRAINT corpus_tokens_language_check CHECK ((language <> ''::text)),
    CONSTRAINT corpus_tokens_morphology_check CHECK ((jsonb_typeof(morphology) = 'object'::text)),
    CONSTRAINT corpus_tokens_sentence_ordinal_check CHECK ((sentence_ordinal >= 0)),
    CONSTRAINT corpus_tokens_start_offset_check CHECK ((start_offset >= 0)),
    CONSTRAINT corpus_tokens_token_ordinal_check CHECK ((token_ordinal >= 0))
);


--
-- Name: source_materials; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.source_materials (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    owner_id uuid NOT NULL,
    language text NOT NULL,
    source_identifier text NOT NULL,
    title text NOT NULL,
    media_type text NOT NULL,
    content_hash text NOT NULL,
    content bytea NOT NULL,
    full_text text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    current_content_revision_id uuid,
    current_snapshot_id uuid,
    book_id uuid
);


--
-- Name: current_analysis_identity; Type: VIEW; Schema: public; Owner: -
--

CREATE VIEW public.current_analysis_identity AS
 SELECT p.owner_id,
    p.book_id,
    p.source_material_id,
    p.analysis_run_id,
    r.content_revision_id,
    r.snapshot_id,
    c.id AS corpus_id
   FROM (((public.book_current_analyses p
     JOIN public.source_materials source ON (((source.owner_id = p.owner_id) AND (source.id = p.source_material_id) AND (source.book_id = p.book_id))))
     JOIN public.analysis_runs r ON (((r.owner_id = p.owner_id) AND (r.id = p.analysis_run_id) AND (r.source_material_id = p.source_material_id) AND (r.state = 'completed'::text))))
     JOIN public.corpora c ON (((c.owner_id = r.owner_id) AND (c.id = r.corpus_id) AND (c.source_material_id = r.source_material_id) AND (c.analysis_run_id = r.id) AND (c.status = 'complete'::text))))
  WHERE ((source.current_content_revision_id = r.content_revision_id) AND (source.current_snapshot_id = r.snapshot_id));


--
-- Name: reading_journey_membership; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reading_journey_membership (
    owner_id uuid NOT NULL,
    book_id uuid NOT NULL,
    "position" integer NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    language text NOT NULL,
    CONSTRAINT reading_journey_membership_language_nonempty CHECK ((btrim(language) <> ''::text)),
    CONSTRAINT reading_journey_membership_position_check CHECK (("position" >= 1))
);


--
-- Name: source_material_units; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.source_material_units (
    owner_id uuid NOT NULL,
    source_material_id uuid NOT NULL,
    unit_id text NOT NULL,
    unit_order bigint NOT NULL,
    spine_index bigint NOT NULL,
    title text NOT NULL,
    title_source text NOT NULL,
    text text NOT NULL,
    start_offset bigint NOT NULL,
    end_offset bigint NOT NULL,
    package_path text NOT NULL,
    manifest_id text NOT NULL,
    source_href text NOT NULL,
    resolved_href text NOT NULL,
    media_type text NOT NULL,
    properties jsonb DEFAULT '[]'::jsonb NOT NULL,
    linear boolean NOT NULL,
    navigation_labels jsonb DEFAULT '[]'::jsonb NOT NULL,
    landmark_types jsonb DEFAULT '[]'::jsonb NOT NULL,
    selected boolean DEFAULT true NOT NULL,
    snapshot_id uuid NOT NULL,
    CONSTRAINT source_material_units_check CHECK ((end_offset >= start_offset)),
    CONSTRAINT source_material_units_spine_index_check CHECK ((spine_index >= 0)),
    CONSTRAINT source_material_units_start_offset_check CHECK ((start_offset >= 0)),
    CONSTRAINT source_material_units_unit_order_check CHECK ((unit_order >= 0))
);


--
-- Name: concordance_occurrences; Type: VIEW; Schema: public; Owner: -
--

CREATE VIEW public.concordance_occurrences AS
 SELECT t.owner_id,
    t.language,
    t.surface,
    t.canonical_lemma,
    t.upos,
    t.dependency,
    t.head AS head_ordinal,
    head.surface AS head_surface,
    s.sentence_text,
    (t.start_offset - s.start_offset) AS sentence_start_offset,
    (t.end_offset - s.start_offset) AS sentence_end_offset,
    t.start_offset AS unit_start_offset,
    t.end_offset AS unit_end_offset,
    (u.start_offset + t.start_offset) AS book_start_offset,
    (u.start_offset + t.end_offset) AS book_end_offset,
    (ca.book_id)::text AS book_id,
    b.title AS book_title,
    (ca.source_material_id)::text AS source_material_id,
    (ca.analysis_run_id)::text AS analysis_run_id,
    (ca.corpus_id)::text AS corpus_id,
    u.unit_id,
    u.title AS chapter_title,
    u.unit_order,
    s.sentence_ordinal,
    t.token_ordinal,
    jm."position" AS book_position,
    jm.created_at AS book_position_created_at
   FROM ((((((public.corpus_tokens t
     JOIN public.corpus_sentences s ON (((s.owner_id = t.owner_id) AND (s.analysis_run_id = t.analysis_run_id) AND (s.corpus_id = t.corpus_id) AND (s.sentence_ordinal = t.sentence_ordinal))))
     LEFT JOIN public.corpus_tokens head ON (((head.owner_id = t.owner_id) AND (head.language = t.language) AND (head.analysis_run_id = t.analysis_run_id) AND (head.corpus_id = t.corpus_id) AND (head.sentence_ordinal = t.sentence_ordinal) AND (head.token_ordinal = t.head))))
     JOIN public.current_analysis_identity ca ON (((ca.owner_id = t.owner_id) AND (ca.analysis_run_id = t.analysis_run_id) AND (ca.corpus_id = t.corpus_id))))
     JOIN public.books b ON (((b.owner_id = ca.owner_id) AND (b.id = ca.book_id) AND (b.language_state = 'chosen'::text) AND (b.language_tag = t.language))))
     JOIN public.source_material_units u ON (((u.owner_id = ca.owner_id) AND (u.source_material_id = ca.source_material_id) AND (u.snapshot_id = ca.snapshot_id) AND (u.unit_id = s.unit_id))))
     LEFT JOIN public.reading_journey_membership jm ON (((jm.owner_id = ca.owner_id) AND (jm.language = t.language) AND (jm.book_id = ca.book_id))));


--
-- Name: curated_sentences; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.curated_sentences (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    owner_id uuid NOT NULL,
    example_sentence_id uuid NOT NULL,
    language text NOT NULL,
    canonical_lemma text NOT NULL,
    upos text NOT NULL,
    notes text DEFAULT ''::text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL
);


--
-- Name: deck_preparation_batch_chunk_items; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.deck_preparation_batch_chunk_items (
    owner_id uuid NOT NULL,
    preparation_id uuid NOT NULL,
    run_id uuid NOT NULL,
    chunk_id uuid NOT NULL,
    generation integer NOT NULL,
    "position" integer NOT NULL,
    ordinal integer NOT NULL,
    candidate_digest text NOT NULL,
    CONSTRAINT deck_preparation_batch_chunk_items_candidate_digest_check CHECK ((candidate_digest ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT deck_preparation_batch_chunk_items_generation_check CHECK ((generation > 0)),
    CONSTRAINT deck_preparation_batch_chunk_items_ordinal_check CHECK ((ordinal >= 0)),
    CONSTRAINT deck_preparation_batch_chunk_items_position_check CHECK (("position" >= 0))
);


--
-- Name: deck_preparation_batch_chunks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.deck_preparation_batch_chunks (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    owner_id uuid NOT NULL,
    preparation_id uuid NOT NULL,
    run_id uuid NOT NULL,
    chunk_index integer NOT NULL,
    generation integer NOT NULL,
    state text DEFAULT 'pending'::text NOT NULL,
    provider_status text,
    model text NOT NULL,
    endpoint text NOT NULL,
    split_reason text NOT NULL,
    first_ordinal integer NOT NULL,
    last_ordinal integer NOT NULL,
    input_digest text NOT NULL,
    request_count integer NOT NULL,
    input_bytes bigint NOT NULL,
    estimated_prompt_tokens bigint NOT NULL,
    completed_count integer DEFAULT 0 NOT NULL,
    failed_count integer DEFAULT 0 NOT NULL,
    expired_count integer DEFAULT 0 NOT NULL,
    input_file_id text,
    batch_id text,
    output_file_id text,
    error_file_id text,
    submission_job_id bigint,
    submission_generation integer DEFAULT 0 NOT NULL,
    submission_claim_token uuid,
    submission_claimed_at timestamptz,
    submission_lease_expires_at timestamptz,
    reconciliation_job_id bigint,
    reconciliation_generation integer DEFAULT 0 NOT NULL,
    reconciliation_claim_token uuid,
    reconciliation_claimed_at timestamptz,
    reconciliation_lease_expires_at timestamptz,
    error_class text DEFAULT ''::text NOT NULL,
    error_code text DEFAULT ''::text NOT NULL,
    input_tokens bigint DEFAULT 0 NOT NULL,
    output_tokens bigint DEFAULT 0 NOT NULL,
    total_tokens bigint DEFAULT 0 NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    submitted_at timestamptz,
    last_polled_at timestamptz,
    provider_completed_at timestamptz,
    reconciled_at timestamptz,
    input_file_cleanup_state text DEFAULT 'pending'::text NOT NULL,
    output_file_cleanup_state text DEFAULT 'pending'::text NOT NULL,
    error_file_cleanup_state text DEFAULT 'pending'::text NOT NULL,
    input_file_cleanup_attempts integer DEFAULT 0 NOT NULL,
    output_file_cleanup_attempts integer DEFAULT 0 NOT NULL,
    error_file_cleanup_attempts integer DEFAULT 0 NOT NULL,
    cleanup_error_class text DEFAULT ''::text NOT NULL,
    cleanup_error_code text DEFAULT ''::text NOT NULL,
    cleanup_claim_token uuid,
    cleanup_claimed_at timestamptz,
    cleanup_lease_expires_at timestamptz,
    cleanup_completed_at timestamptz,
    CONSTRAINT deck_preparation_batch_chunks_batch_id_check CHECK (((batch_id IS NULL) OR (batch_id <> ''::text))),
    CONSTRAINT deck_preparation_batch_chunks_check CHECK ((last_ordinal >= first_ordinal)),
    CONSTRAINT deck_preparation_batch_chunks_check1 CHECK (((total_tokens >= 0) AND (total_tokens = (input_tokens + output_tokens)))),
    CONSTRAINT deck_preparation_batch_chunks_check2 CHECK ((((completed_count + failed_count) + expired_count) <= request_count)),
    CONSTRAINT deck_preparation_batch_chunks_check3 CHECK ((((submission_claim_token IS NULL) AND (submission_claimed_at IS NULL) AND (submission_lease_expires_at IS NULL)) OR ((submission_claim_token IS NOT NULL) AND (submission_claimed_at IS NOT NULL) AND (submission_lease_expires_at > submission_claimed_at)))),
    CONSTRAINT deck_preparation_batch_chunks_check4 CHECK ((((reconciliation_claim_token IS NULL) AND (reconciliation_claimed_at IS NULL) AND (reconciliation_lease_expires_at IS NULL)) OR ((reconciliation_claim_token IS NOT NULL) AND (reconciliation_claimed_at IS NOT NULL) AND (reconciliation_lease_expires_at > reconciliation_claimed_at)))),
    CONSTRAINT deck_preparation_batch_chunks_chunk_index_check CHECK ((chunk_index >= 0)),
    CONSTRAINT deck_preparation_batch_chunks_cleanup_attempts_check CHECK (((input_file_cleanup_attempts >= 0) AND (output_file_cleanup_attempts >= 0) AND (error_file_cleanup_attempts >= 0))),
    CONSTRAINT deck_preparation_batch_chunks_cleanup_claim_check CHECK ((((cleanup_claim_token IS NULL) AND (cleanup_claimed_at IS NULL) AND (cleanup_lease_expires_at IS NULL)) OR ((cleanup_claim_token IS NOT NULL) AND (cleanup_claimed_at IS NOT NULL) AND (cleanup_lease_expires_at > cleanup_claimed_at)))),
    CONSTRAINT deck_preparation_batch_chunks_cleanup_error_check CHECK ((cleanup_error_code ~ '^[a-z0-9_.:-]{0,80}$'::text)),
    CONSTRAINT deck_preparation_batch_chunks_cleanup_states_check CHECK (((input_file_cleanup_state = ANY (ARRAY['pending'::text, 'deleted'::text, 'failed'::text, 'not_needed'::text])) AND (output_file_cleanup_state = ANY (ARRAY['pending'::text, 'deleted'::text, 'failed'::text, 'not_needed'::text])) AND (error_file_cleanup_state = ANY (ARRAY['pending'::text, 'deleted'::text, 'failed'::text, 'not_needed'::text])))),
    CONSTRAINT deck_preparation_batch_chunks_completed_count_check CHECK ((completed_count >= 0)),
    CONSTRAINT deck_preparation_batch_chunks_endpoint_check CHECK ((endpoint = '/v1/chat/completions'::text)),
    CONSTRAINT deck_preparation_batch_chunks_error_class_check CHECK ((error_class = ANY (ARRAY[''::text, 'configuration'::text, 'upload'::text, 'submission'::text, 'ambiguous_submission'::text, 'provider'::text, 'unsupported_model'::text, 'expired'::text, 'cancelled'::text, 'poll'::text, 'reconciliation'::text, 'malformed_result'::text, 'missing_result'::text, 'duplicate_result'::text, 'unknown_result'::text, 'validation'::text]))),
    CONSTRAINT deck_preparation_batch_chunks_error_code_check CHECK ((error_code ~ '^[a-z0-9_.:-]{0,80}$'::text)),
    CONSTRAINT deck_preparation_batch_chunks_error_file_id_check CHECK (((error_file_id IS NULL) OR (error_file_id <> ''::text))),
    CONSTRAINT deck_preparation_batch_chunks_estimated_prompt_tokens_check CHECK ((estimated_prompt_tokens >= 0)),
    CONSTRAINT deck_preparation_batch_chunks_expired_count_check CHECK ((expired_count >= 0)),
    CONSTRAINT deck_preparation_batch_chunks_failed_count_check CHECK ((failed_count >= 0)),
    CONSTRAINT deck_preparation_batch_chunks_first_ordinal_check CHECK ((first_ordinal >= 0)),
    CONSTRAINT deck_preparation_batch_chunks_generation_check CHECK ((generation > 0)),
    CONSTRAINT deck_preparation_batch_chunks_input_bytes_check CHECK (((input_bytes > 0) AND (input_bytes <= 209715200))),
    CONSTRAINT deck_preparation_batch_chunks_input_digest_check CHECK ((input_digest ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT deck_preparation_batch_chunks_input_file_id_check CHECK (((input_file_id IS NULL) OR (input_file_id <> ''::text))),
    CONSTRAINT deck_preparation_batch_chunks_input_tokens_check CHECK ((input_tokens >= 0)),
    CONSTRAINT deck_preparation_batch_chunks_model_check CHECK ((model <> ''::text)),
    CONSTRAINT deck_preparation_batch_chunks_output_file_id_check CHECK (((output_file_id IS NULL) OR (output_file_id <> ''::text))),
    CONSTRAINT deck_preparation_batch_chunks_output_tokens_check CHECK ((output_tokens >= 0)),
    CONSTRAINT deck_preparation_batch_chunks_provider_status_check CHECK (((provider_status IS NULL) OR (provider_status = ANY (ARRAY['validating'::text, 'in_progress'::text, 'finalizing'::text, 'completed'::text, 'failed'::text, 'expired'::text, 'cancelling'::text, 'cancelled'::text])))),
    CONSTRAINT deck_preparation_batch_chunks_reconciliation_generation_check CHECK ((reconciliation_generation >= 0)),
    CONSTRAINT deck_preparation_batch_chunks_request_count_check CHECK (((request_count > 0) AND (request_count <= 50000))),
    CONSTRAINT deck_preparation_batch_chunks_split_reason_check CHECK ((split_reason = ANY (ARRAY['run'::text, 'request_limit'::text, 'byte_limit'::text, 'token_limit'::text, 'retry'::text]))),
    CONSTRAINT deck_preparation_batch_chunks_state_check CHECK ((state = ANY (ARRAY['pending'::text, 'submitting'::text, 'submitted'::text, 'polling'::text, 'reconciling'::text, 'completed'::text, 'failed'::text, 'cancelled'::text, 'ambiguous'::text]))),
    CONSTRAINT deck_preparation_batch_chunks_submission_generation_check CHECK ((submission_generation >= 0))
);


--
-- Name: deck_preparation_manifest_items; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.deck_preparation_manifest_items (
    owner_id uuid NOT NULL,
    preparation_id uuid NOT NULL,
    run_id uuid NOT NULL,
    ordinal integer NOT NULL,
    disposition text NOT NULL,
    language text NOT NULL,
    canonical_lemma text NOT NULL,
    upos text NOT NULL,
    source_sentence text NOT NULL,
    tested_target text NOT NULL,
    first_encounter bigint NOT NULL,
    quality_score integer NOT NULL,
    quality_reasons text[] DEFAULT '{}'::text[] NOT NULL,
    render_payload jsonb NOT NULL,
    provider text,
    provider_version text,
    sentence_hash text,
    candidate_digest text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    target_language text DEFAULT 'en'::text NOT NULL,
    quality_gdex_score double precision DEFAULT 0 NOT NULL,
    CONSTRAINT deck_preparation_manifest_items_candidate_digest_check CHECK ((candidate_digest ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT deck_preparation_manifest_items_canonical_lemma_check CHECK ((canonical_lemma <> ''::text)),
    CONSTRAINT deck_preparation_manifest_items_check CHECK ((((provider IS NULL) AND (provider_version IS NULL) AND (sentence_hash IS NULL)) OR ((provider IS NOT NULL) AND (provider <> ''::text) AND (provider_version IS NOT NULL) AND (provider_version <> ''::text) AND (sentence_hash IS NOT NULL) AND ((sentence_hash = ''::text) OR (sentence_hash ~ '^[0-9a-f]{64}$'::text))))),
    CONSTRAINT deck_preparation_manifest_items_check1 CHECK (((disposition = 'accepted'::text) OR ((disposition = 'quality_omitted'::text) AND (provider IS NULL) AND (provider_version IS NULL) AND (sentence_hash IS NULL)))),
    CONSTRAINT deck_preparation_manifest_items_disposition_check CHECK ((disposition = ANY (ARRAY['accepted'::text, 'quality_omitted'::text]))),
    CONSTRAINT deck_preparation_manifest_items_language_check CHECK ((language <> ''::text)),
    CONSTRAINT deck_preparation_manifest_items_ordinal_check CHECK ((ordinal >= 0)),
    CONSTRAINT deck_preparation_manifest_items_quality_gdex_score_check CHECK (((quality_gdex_score >= (0)::double precision) AND (quality_gdex_score <= (1)::double precision))),
    CONSTRAINT deck_preparation_manifest_items_quality_reasons_check CHECK (((cardinality(quality_reasons) <= 16) AND (array_position(quality_reasons, NULL::text) IS NULL) AND (quality_reasons <@ ARRAY['too short or fragmented'::text, 'too long'::text, 'usable length'::text, 'useful context window'::text, 'target not present as a word'::text, 'target present'::text, 'invalid source location'::text, 'valid source location'::text, 'complete sentence boundaries'::text, 'incomplete sentence boundaries'::text, 'structural noise or boilerplate'::text, 'no obvious structural noise'::text, 'no finite verb and subject'::text, 'target in subordinate clause'::text, 'deictic context'::text, 'named-entity density'::text, 'optimal length'::text, 'outside optimal length'::text]))),
    CONSTRAINT deck_preparation_manifest_items_quality_score_check CHECK (((quality_score >= 0) AND (quality_score <= 110))),
    CONSTRAINT deck_preparation_manifest_items_render_payload_check CHECK ((jsonb_typeof(render_payload) = 'object'::text)),
    CONSTRAINT deck_preparation_manifest_items_target_language_check CHECK ((target_language <> ''::text)),
    CONSTRAINT deck_preparation_manifest_items_upos_check CHECK (((upos <> ''::text) AND (upos = upper(upos))))
);


--
-- Name: COLUMN deck_preparation_manifest_items.target_language; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.deck_preparation_manifest_items.target_language IS 'Frozen target language for this manifest item cache identity';


--
-- Name: deck_preparation_manifests; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.deck_preparation_manifests (
    owner_id uuid NOT NULL,
    preparation_id uuid NOT NULL,
    run_id uuid NOT NULL,
    schema_version integer NOT NULL,
    manifest_digest text NOT NULL,
    deck_name text NOT NULL,
    filename text NOT NULL,
    selected_count integer NOT NULL,
    accepted_count integer NOT NULL,
    omitted_count integer NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT deck_preparation_manifests_accepted_count_check CHECK ((accepted_count >= 0)),
    CONSTRAINT deck_preparation_manifests_check CHECK ((selected_count = (accepted_count + omitted_count))),
    CONSTRAINT deck_preparation_manifests_deck_name_check CHECK ((deck_name <> ''::text)),
    CONSTRAINT deck_preparation_manifests_filename_check CHECK ((filename <> ''::text)),
    CONSTRAINT deck_preparation_manifests_manifest_digest_check CHECK ((manifest_digest ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT deck_preparation_manifests_omitted_count_check CHECK ((omitted_count >= 0)),
    CONSTRAINT deck_preparation_manifests_schema_version_check CHECK ((schema_version > 0)),
    CONSTRAINT deck_preparation_manifests_selected_count_check CHECK ((selected_count >= 0))
);


--
-- Name: deck_preparation_runs; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.deck_preparation_runs (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    owner_id uuid NOT NULL,
    preparation_id uuid NOT NULL,
    run_number integer NOT NULL,
    state text NOT NULL,
    translation_state text NOT NULL,
    external_translation_consent boolean NOT NULL,
    external_translation_configured boolean NOT NULL,
    context_mode text,
    provider text,
    provider_version text,
    endpoint text,
    model text,
    manifest_schema_version integer NOT NULL,
    retry_policy_version integer NOT NULL,
    max_provider_attempts integer NOT NULL,
    max_batch_generations integer NOT NULL,
    batch_max_requests integer NOT NULL,
    batch_max_bytes bigint NOT NULL,
    candidate_count integer NOT NULL,
    completed_count integer DEFAULT 0 NOT NULL,
    failed_count integer DEFAULT 0 NOT NULL,
    finalization_dispatch_generation integer DEFAULT 0 NOT NULL,
    finalization_dispatch_count integer DEFAULT 0 NOT NULL,
    finalization_job_id bigint,
    finalization_claim_token uuid,
    finalization_claimed_at timestamptz,
    finalization_lease_expires_at timestamptz,
    error_class text DEFAULT ''::text NOT NULL,
    error_code text DEFAULT ''::text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    translation_completed_at timestamptz,
    completed_at timestamptz,
    execution_mode text DEFAULT 'batch'::text NOT NULL,
    target_language text DEFAULT 'en'::text NOT NULL,
    CONSTRAINT deck_preparation_runs_batch_max_bytes_check CHECK (((batch_max_bytes > 0) AND (batch_max_bytes <= 209715200))),
    CONSTRAINT deck_preparation_runs_batch_max_requests_check CHECK (((batch_max_requests > 0) AND (batch_max_requests <= 50000))),
    CONSTRAINT deck_preparation_runs_candidate_count_check CHECK ((candidate_count >= 0)),
    CONSTRAINT deck_preparation_runs_check CHECK (((completed_count + failed_count) <= candidate_count)),
    CONSTRAINT deck_preparation_runs_check1 CHECK (((external_translation_consent AND external_translation_configured AND (context_mode IS NOT NULL) AND (context_mode = ANY (ARRAY['sentence'::text, 'lemma_only'::text])) AND (provider IS NOT NULL) AND (provider <> ''::text) AND (provider_version IS NOT NULL) AND (provider_version <> ''::text) AND (endpoint IS NOT NULL) AND (endpoint = '/v1/chat/completions'::text) AND (model IS NOT NULL) AND (model <> ''::text)) OR ((NOT (external_translation_consent AND external_translation_configured)) AND (context_mode IS NULL) AND (provider IS NULL) AND (provider_version IS NULL) AND (endpoint IS NULL) AND (model IS NULL)))),
    CONSTRAINT deck_preparation_runs_check2 CHECK ((((finalization_claim_token IS NULL) AND (finalization_claimed_at IS NULL) AND (finalization_lease_expires_at IS NULL)) OR ((finalization_claim_token IS NOT NULL) AND (finalization_claimed_at IS NOT NULL) AND (finalization_lease_expires_at > finalization_claimed_at)))),
    CONSTRAINT deck_preparation_runs_check3 CHECK ((((state = ANY (ARRAY['completed'::text, 'failed'::text, 'cancelled'::text])) AND (completed_at IS NOT NULL)) OR ((state = ANY (ARRAY['translating'::text, 'finalizing'::text])) AND (completed_at IS NULL)))),
    CONSTRAINT deck_preparation_runs_check4 CHECK ((((translation_state = 'completed'::text) AND (translation_completed_at IS NOT NULL)) OR (translation_state <> 'completed'::text))),
    CONSTRAINT deck_preparation_runs_completed_count_check CHECK ((completed_count >= 0)),
    CONSTRAINT deck_preparation_runs_error_class_check CHECK ((error_class = ANY (ARRAY[''::text, 'configuration'::text, 'identity'::text, 'orchestration'::text, 'persistence'::text, 'render'::text, 'cache'::text, 'provider'::text, 'validation'::text, 'retry_exhausted'::text, 'ambiguous_submission'::text, 'reconciliation'::text]))),
    CONSTRAINT deck_preparation_runs_error_code_check CHECK ((error_code ~ '^[a-z0-9_.:-]{0,80}$'::text)),
    CONSTRAINT deck_preparation_runs_execution_mode_check CHECK ((execution_mode = ANY (ARRAY['standard'::text, 'batch'::text]))),
    CONSTRAINT deck_preparation_runs_failed_count_check CHECK ((failed_count >= 0)),
    CONSTRAINT deck_preparation_runs_finalization_dispatch_count_check CHECK ((finalization_dispatch_count >= 0)),
    CONSTRAINT deck_preparation_runs_finalization_dispatch_generation_check CHECK ((finalization_dispatch_generation >= 0)),
    CONSTRAINT deck_preparation_runs_manifest_schema_version_check CHECK ((manifest_schema_version > 0)),
    CONSTRAINT deck_preparation_runs_max_batch_generations_check CHECK ((max_batch_generations > 0)),
    CONSTRAINT deck_preparation_runs_max_provider_attempts_check CHECK ((max_provider_attempts > 0)),
    CONSTRAINT deck_preparation_runs_retry_policy_version_check CHECK ((retry_policy_version > 0)),
    CONSTRAINT deck_preparation_runs_run_number_check CHECK ((run_number > 0)),
    CONSTRAINT deck_preparation_runs_state_check CHECK ((state = ANY (ARRAY['translating'::text, 'finalizing'::text, 'completed'::text, 'failed'::text, 'cancelled'::text]))),
    CONSTRAINT deck_preparation_runs_target_language_check CHECK ((target_language <> ''::text)),
    CONSTRAINT deck_preparation_runs_translation_state_check CHECK ((translation_state = ANY (ARRAY['pending'::text, 'running'::text, 'completed'::text, 'failed'::text, 'cancelled'::text])))
);


--
-- Name: COLUMN deck_preparation_runs.execution_mode; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.deck_preparation_runs.execution_mode IS 'Frozen prepared-deck executor mode: standard or batch';


--
-- Name: COLUMN deck_preparation_runs.target_language; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.deck_preparation_runs.target_language IS 'Frozen prepared-deck translation target language';


--
-- Name: deck_preparation_translation_outcomes; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.deck_preparation_translation_outcomes (
    owner_id uuid NOT NULL,
    preparation_id uuid NOT NULL,
    run_id uuid NOT NULL,
    ordinal integer NOT NULL,
    state text NOT NULL,
    dispatch_count integer DEFAULT 0 NOT NULL,
    provider_attempt_count integer DEFAULT 0 NOT NULL,
    max_provider_attempts integer NOT NULL,
    next_attempt_at timestamptz DEFAULT now() NOT NULL,
    dispatch_generation integer DEFAULT 0 NOT NULL,
    river_job_id bigint,
    claim_token uuid,
    claimed_at timestamptz,
    lease_expires_at timestamptz,
    terminal_at timestamptz,
    error_class text DEFAULT ''::text NOT NULL,
    error_code text DEFAULT ''::text NOT NULL,
    cache_hit_count integer DEFAULT 0 NOT NULL,
    provider_call_count integer DEFAULT 0 NOT NULL,
    cache_latency_ms bigint DEFAULT 0 NOT NULL,
    provider_latency_ms bigint DEFAULT 0 NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT deck_preparation_translation_outco_provider_attempt_count_check CHECK ((provider_attempt_count >= 0)),
    CONSTRAINT deck_preparation_translation_outcom_max_provider_attempts_check CHECK ((max_provider_attempts > 0)),
    CONSTRAINT deck_preparation_translation_outcomes_cache_hit_count_check CHECK ((cache_hit_count >= 0)),
    CONSTRAINT deck_preparation_translation_outcomes_cache_latency_ms_check CHECK ((cache_latency_ms >= 0)),
    CONSTRAINT deck_preparation_translation_outcomes_check CHECK ((provider_attempt_count <= max_provider_attempts)),
    CONSTRAINT deck_preparation_translation_outcomes_check1 CHECK ((((state = 'running'::text) AND (claim_token IS NOT NULL) AND (claimed_at IS NOT NULL) AND (lease_expires_at > claimed_at) AND (terminal_at IS NULL)) OR ((state = 'pending'::text) AND (claim_token IS NULL) AND (claimed_at IS NULL) AND (lease_expires_at IS NULL) AND (terminal_at IS NULL)) OR ((state = ANY (ARRAY['completed'::text, 'failed'::text, 'cancelled'::text])) AND (claim_token IS NULL) AND (claimed_at IS NULL) AND (lease_expires_at IS NULL) AND (terminal_at IS NOT NULL)))),
    CONSTRAINT deck_preparation_translation_outcomes_dispatch_count_check CHECK ((dispatch_count >= 0)),
    CONSTRAINT deck_preparation_translation_outcomes_dispatch_generation_check CHECK ((dispatch_generation >= 0)),
    CONSTRAINT deck_preparation_translation_outcomes_error_class_check CHECK ((error_class = ANY (ARRAY[''::text, 'cache'::text, 'provider'::text, 'rate_limit'::text, 'provider_5xx'::text, 'timeout'::text, 'cancellation'::text, 'validation'::text, 'identity'::text, 'retry_exhausted'::text, 'malformed_result'::text, 'missing_result'::text, 'duplicate_result'::text, 'unknown_result'::text, 'expired'::text, 'orchestration'::text]))),
    CONSTRAINT deck_preparation_translation_outcomes_error_code_check CHECK ((error_code ~ '^[a-z0-9_.:-]{0,80}$'::text)),
    CONSTRAINT deck_preparation_translation_outcomes_provider_call_count_check CHECK ((provider_call_count >= 0)),
    CONSTRAINT deck_preparation_translation_outcomes_provider_latency_ms_check CHECK ((provider_latency_ms >= 0)),
    CONSTRAINT deck_preparation_translation_outcomes_state_check CHECK ((state = ANY (ARRAY['pending'::text, 'running'::text, 'completed'::text, 'failed'::text, 'cancelled'::text])))
);


--
-- Name: deck_preparation_vocabulary; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.deck_preparation_vocabulary (
    owner_id uuid NOT NULL,
    deck_preparation_id uuid NOT NULL,
    language text NOT NULL,
    canonical_lemma text NOT NULL,
    upos text NOT NULL,
    generated_at timestamptz NOT NULL,
    graduated_at timestamptz
);


--
-- Name: deck_preparations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.deck_preparations (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    owner_id uuid NOT NULL,
    source_material_id uuid NOT NULL,
    state text DEFAULT 'queued'::text NOT NULL,
    artifact bytea,
    filename text NOT NULL,
    deck_name text NOT NULL,
    content_hash text NOT NULL,
    total_cards integer DEFAULT 0 NOT NULL,
    cards_with_english integer DEFAULT 0 NOT NULL,
    cards_with_contextual_sentence_translations integer DEFAULT 0 NOT NULL,
    quality_omissions integer DEFAULT 0 NOT NULL,
    error text DEFAULT ''::text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    started_at timestamptz,
    completed_at timestamptz,
    analysis_run_id uuid,
    current_run_id uuid,
    studying_at timestamptz,
    reviewed_at timestamptz,
    graduated_at timestamptz,
    released_at timestamptz,
    book_id uuid,
    retired_at timestamptz,
    cards_with_fallback_gloss integer DEFAULT 0 NOT NULL,
    CONSTRAINT deck_preparations_cards_with_contextual_sentence_translat_check CHECK ((cards_with_contextual_sentence_translations >= 0)),
    CONSTRAINT deck_preparations_cards_with_english_check CHECK ((cards_with_english >= 0)),
    CONSTRAINT deck_preparations_cards_with_fallback_gloss_check CHECK ((cards_with_fallback_gloss <= total_cards)),
    CONSTRAINT deck_preparations_cards_with_fallback_gloss_nonnegative_check CHECK ((cards_with_fallback_gloss >= 0)),
    CONSTRAINT deck_preparations_check CHECK ((((state = 'ready'::text) AND (artifact IS NOT NULL) AND (octet_length(artifact) > 0) AND (completed_at IS NOT NULL)) OR ((state <> 'ready'::text) AND (artifact IS NULL)))),
    CONSTRAINT deck_preparations_check1 CHECK ((cards_with_english <= total_cards)),
    CONSTRAINT deck_preparations_check2 CHECK ((cards_with_contextual_sentence_translations <= total_cards)),
    CONSTRAINT deck_preparations_quality_omissions_check CHECK ((quality_omissions >= 0)),
    CONSTRAINT deck_preparations_state_check CHECK ((state = ANY (ARRAY['queued'::text, 'preparing'::text, 'ready'::text, 'failed'::text, 'cancelled'::text]))),
    CONSTRAINT deck_preparations_total_cards_check CHECK ((total_cards >= 0))
);


--
-- Name: decks; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.decks (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    owner_id uuid NOT NULL,
    language text NOT NULL,
    name text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL
);


--
-- Name: enrichment_cache; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.enrichment_cache (
    language text NOT NULL,
    canonical_lemma text NOT NULL,
    upos text NOT NULL,
    provider text NOT NULL,
    provider_version text NOT NULL,
    translation text DEFAULT ''::text NOT NULL,
    cached_at timestamptz DEFAULT now() NOT NULL,
    sentence_hash text DEFAULT ''::text NOT NULL,
    sentence_translation text DEFAULT ''::text NOT NULL,
    sentence_translation_target text DEFAULT ''::text NOT NULL,
    target_language text DEFAULT 'en'::text NOT NULL,
    dictionary_provider_version text DEFAULT ''::text NOT NULL,
    fallback_gloss text DEFAULT ''::text NOT NULL,
    sense_selection jsonb DEFAULT '[]'::jsonb NOT NULL,
    CONSTRAINT enrichment_cache_check CHECK (((language <> ''::text) AND (canonical_lemma <> ''::text) AND (upos <> ''::text) AND (provider <> ''::text) AND (provider_version <> ''::text))),
    CONSTRAINT enrichment_cache_sense_selection_array CHECK ((jsonb_typeof(sense_selection) = 'array'::text)),
    CONSTRAINT enrichment_cache_sentence_hash_format CHECK (((sentence_hash = ''::text) OR (sentence_hash ~ '^[0-9a-f]{64}$'::text))),
    CONSTRAINT enrichment_cache_target_language_nonempty CHECK ((target_language <> ''::text)),
    CONSTRAINT enrichment_cache_translation_required CHECK ((translation <> ''::text))
);


--
-- Name: TABLE enrichment_cache; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.enrichment_cache IS 'Shared, language-scoped, immutable external enrichment results; contains no owner or source metadata';


--
-- Name: COLUMN enrichment_cache.sentence_hash; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.enrichment_cache.sentence_hash IS 'Lowercase hex SHA-256 of conservatively normalized approved sentence text; empty for lemma-only legacy entries';


--
-- Name: COLUMN enrichment_cache.sentence_translation; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.enrichment_cache.sentence_translation IS 'Natural translation of the complete approved example sentence; empty when unavailable';


--
-- Name: COLUMN enrichment_cache.sentence_translation_target; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.enrichment_cache.sentence_translation_target IS 'Optional plain-text English word or phrase corresponding to the target in sentence_translation; provider markup is never stored';


--
-- Name: COLUMN enrichment_cache.target_language; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.enrichment_cache.target_language IS 'Explicit target language component of the immutable shared enrichment identity';


--
-- Name: COLUMN enrichment_cache.dictionary_provider_version; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.enrichment_cache.dictionary_provider_version IS 'Dictionary index identity used to produce the frozen card meaning candidates';


--
-- Name: COLUMN enrichment_cache.fallback_gloss; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.enrichment_cache.fallback_gloss IS 'Consent-gated external gloss used only when the frozen dictionary meaning is unavailable';


--
-- Name: COLUMN enrichment_cache.sense_selection; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.enrichment_cache.sense_selection IS 'Reserved durable ordered dictionary sense indices for the consented selection phase';


--
-- Name: example_sentences; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.example_sentences (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    owner_id uuid NOT NULL,
    corpus_id uuid NOT NULL,
    sentence_key text NOT NULL,
    sentence_text text NOT NULL,
    source_location jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    language text,
    canonical_lemma text,
    upos text,
    selection_rank integer,
    selection_score integer,
    selection_reasons jsonb,
    is_chosen boolean DEFAULT false NOT NULL,
    CONSTRAINT example_sentences_selection_metadata_check CHECK ((((language IS NULL) AND (canonical_lemma IS NULL) AND (upos IS NULL) AND (selection_rank IS NULL) AND (selection_score IS NULL) AND (selection_reasons IS NULL)) OR ((language <> ''::text) AND (canonical_lemma <> ''::text) AND (upos <> ''::text) AND (selection_rank >= 1) AND (selection_reasons IS NOT NULL))))
);


--
-- Name: frequency_datasets; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.frequency_datasets (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    language text NOT NULL,
    name text NOT NULL,
    version text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    source_url text DEFAULT ''::text NOT NULL,
    license text DEFAULT ''::text NOT NULL,
    attribution text DEFAULT ''::text NOT NULL,
    active boolean DEFAULT false NOT NULL
);


--
-- Name: frequency_entries; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.frequency_entries (
    dataset_id uuid NOT NULL,
    language text NOT NULL,
    canonical_lemma text NOT NULL,
    upos text NOT NULL,
    rank bigint NOT NULL,
    frequency double precision NOT NULL,
    frequency_class smallint DEFAULT 0 NOT NULL,
    CONSTRAINT frequency_entries_frequency_check CHECK ((frequency >= (0)::double precision)),
    CONSTRAINT frequency_entries_frequency_class_check CHECK (((frequency_class >= 0) AND (frequency_class <= 6))),
    CONSTRAINT frequency_entries_rank_check CHECK ((rank > 0))
);


--
-- Name: generated_vocabulary; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.generated_vocabulary (
    owner_id uuid NOT NULL,
    language text NOT NULL,
    canonical_lemma text NOT NULL,
    upos text NOT NULL,
    first_deck_id uuid NOT NULL,
    first_source_material_id uuid,
    first_generated_at timestamptz DEFAULT now() NOT NULL
);


--
-- Name: known_vocabulary; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.known_vocabulary (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    owner_id uuid NOT NULL,
    language text NOT NULL,
    canonical_lemma text NOT NULL,
    upos text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL
);


--
-- Name: source_content_revisions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.source_content_revisions (
    revision_id uuid DEFAULT gen_random_uuid() NOT NULL,
    owner_id uuid NOT NULL,
    source_material_id uuid NOT NULL,
    digest_version integer NOT NULL,
    content_digest text NOT NULL,
    content bytea NOT NULL,
    full_text text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT source_content_revisions_check CHECK (((digest_version = 0) OR (content_digest = ('sha256:'::text || encode(public.digest(content, 'sha256'::text), 'hex'::text))))),
    CONSTRAINT source_content_revisions_content_digest_check CHECK ((content_digest ~ '^(sha256:[0-9a-f]{64}|legacy:[^[:space:]]+)$'::text)),
    CONSTRAINT source_content_revisions_digest_version_check CHECK ((digest_version = ANY (ARRAY[0, 1])))
);


--
-- Name: source_material_evidence; Type: VIEW; Schema: public; Owner: -
--

CREATE VIEW public.source_material_evidence AS
 SELECT (s.id)::text AS source_id,
    (s.owner_id)::text AS source_owner_id,
    s.language AS source_language,
    s.source_identifier,
    s.title AS source_title,
    s.media_type AS source_media_type,
    COALESCE((s.book_id)::text, ''::text) AS book_id,
    COALESCE(b.title, ''::text) AS book_title,
        CASE
            WHEN (r.digest_version = 1) THEN r.content_digest
            ELSE s.content_hash
        END AS content_hash,
    COALESCE(r.content_digest, ''::text) AS content_digest,
    COALESCE((r.revision_id)::text, ''::text) AS content_revision_id,
    COALESCE((s.current_snapshot_id)::text, ''::text) AS content_snapshot_id,
    COALESCE(r.digest_version, 0) AS digest_version,
    s.created_at AS source_created_at,
        CASE
            WHEN (ar.state = ANY (ARRAY['queued'::text, 'running'::text])) THEN 'analyzing'::text
            WHEN (ar.state = 'failed'::text) THEN 'analysis failed'::text
            WHEN (ar.state = 'cancelled'::text) THEN 'analysis cancelled'::text
            WHEN ((p.source_material_id IS NOT NULL) AND (ca.analysis_run_id IS NULL)) THEN 'stale'::text
            WHEN (ca.analysis_run_id IS NOT NULL) THEN 'analyzed'::text
            WHEN ((j.river_job_id IS NOT NULL) AND (j.error = ''::text)) THEN 'analyzing'::text
            ELSE 'not analyzed'::text
        END AS analysis_status,
        CASE
            WHEN (ar.state IS NOT NULL) THEN ar.state
            WHEN (ca.analysis_run_id IS NOT NULL) THEN 'completed'::text
            WHEN ((j.river_job_id IS NOT NULL) AND (j.error <> ''::text)) THEN 'failed'::text
            WHEN (j.river_job_id IS NOT NULL) THEN 'queued'::text
            ELSE ''::text
        END AS analysis_state,
    COALESCE((ca.analysis_run_id)::text, ''::text) AS analysis_run_id,
    COALESCE((ca.corpus_id)::text, ''::text) AS corpus_id,
    COALESCE(j.river_job_id, (0)::bigint) AS analysis_job_id,
    (p.source_material_id IS NOT NULL) AS is_current_analysis
   FROM ((((((public.source_materials s
     LEFT JOIN public.books b ON (((b.owner_id = s.owner_id) AND (b.id = s.book_id))))
     LEFT JOIN public.source_content_revisions r ON (((r.owner_id = s.owner_id) AND (r.revision_id = s.current_content_revision_id))))
     LEFT JOIN public.book_current_analyses p ON (((p.owner_id = s.owner_id) AND (p.source_material_id = s.id))))
     LEFT JOIN public.current_analysis_identity ca ON (((ca.owner_id = p.owner_id) AND (ca.source_material_id = p.source_material_id))))
     LEFT JOIN LATERAL ( SELECT analysis_jobs.river_job_id,
            analysis_jobs.error,
            analysis_jobs.analysis_run_id
           FROM public.analysis_jobs
          WHERE ((analysis_jobs.owner_id = s.owner_id) AND (analysis_jobs.source_material_id = s.id))
          ORDER BY analysis_jobs.created_at DESC, analysis_jobs.river_job_id DESC
         LIMIT 1) j ON (true))
     LEFT JOIN public.analysis_runs ar ON (((ar.owner_id = s.owner_id) AND (ar.id = j.analysis_run_id))));


--
-- Name: my_books_evidence; Type: VIEW; Schema: public; Owner: -
--

CREATE VIEW public.my_books_evidence AS
 SELECT (b.id)::text AS book_id,
    (b.owner_id)::text AS book_owner_id,
    b.title AS book_title,
    b.metadata_provenance AS book_metadata_provenance,
    b.language_state AS book_language_state,
    COALESCE(b.language_tag, ''::text) AS book_language_tag,
    b.created_at AS book_created_at,
    b.updated_at AS book_updated_at,
    COALESCE(sme.source_id, ''::text) AS source_id,
    COALESCE(sme.source_owner_id, ''::text) AS source_owner_id,
    COALESCE(sme.source_language, ''::text) AS source_language,
    COALESCE(sme.source_identifier, ''::text) AS source_identifier,
    COALESCE(sme.source_title, ''::text) AS source_title,
    COALESCE(sme.source_media_type, ''::text) AS source_media_type,
    COALESCE(sme.content_hash, ''::text) AS source_content_hash,
    COALESCE(sme.content_digest, ''::text) AS source_content_digest,
    COALESCE(sme.content_revision_id, ''::text) AS source_content_revision_id,
    COALESCE(sme.content_snapshot_id, ''::text) AS source_content_snapshot_id,
    COALESCE(sme.digest_version, 0) AS source_digest_version,
    sme.source_created_at,
    (sme.source_id IS NOT NULL) AS acquired,
    COALESCE(sme.analysis_status, ''::text) AS analysis_status,
    COALESCE(sme.analysis_state, ''::text) AS analysis_state,
    COALESCE(sme.analysis_run_id, ''::text) AS analysis_run_id,
    COALESCE(sme.corpus_id, ''::text) AS corpus_id,
    COALESCE(sme.analysis_job_id, (0)::bigint) AS analysis_job_id
   FROM ((public.books b
     JOIN public.book_membership m ON (((m.owner_id = b.owner_id) AND (m.book_id = b.id) AND (m.state = 'active'::text))))
     LEFT JOIN ( SELECT source_material_evidence.source_id,
            source_material_evidence.source_owner_id,
            source_material_evidence.source_language,
            source_material_evidence.source_identifier,
            source_material_evidence.source_title,
            source_material_evidence.source_media_type,
            source_material_evidence.book_id,
            source_material_evidence.book_title,
            source_material_evidence.content_hash,
            source_material_evidence.content_digest,
            source_material_evidence.content_revision_id,
            source_material_evidence.content_snapshot_id,
            source_material_evidence.digest_version,
            source_material_evidence.source_created_at,
            source_material_evidence.analysis_status,
            source_material_evidence.analysis_state,
            source_material_evidence.analysis_run_id,
            source_material_evidence.corpus_id,
            source_material_evidence.analysis_job_id,
            source_material_evidence.is_current_analysis,
            row_number() OVER (PARTITION BY source_material_evidence.source_owner_id, source_material_evidence.book_id ORDER BY source_material_evidence.is_current_analysis DESC, source_material_evidence.source_created_at DESC, source_material_evidence.source_id DESC) AS source_rn
           FROM public.source_material_evidence) sme ON (((sme.source_owner_id = (b.owner_id)::text) AND (sme.book_id = (b.id)::text) AND (sme.source_rn = 1))));


--
-- Name: normalized_corpus_artifacts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.normalized_corpus_artifacts (
    content_hash text NOT NULL,
    language text NOT NULL,
    schema_version text NOT NULL,
    normalization_profile text NOT NULL,
    normalization_version text NOT NULL,
    analyzer_name text NOT NULL,
    analyzer_version text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL
);


--
-- Name: opds_connections; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.opds_connections (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    name text NOT NULL,
    url text NOT NULL,
    username text DEFAULT ''::text NOT NULL,
    password_encrypted bytea DEFAULT '\x'::bytea NOT NULL,
    language text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    owner_id uuid
);


--
-- Name: COLUMN opds_connections.password_encrypted; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.opds_connections.password_encrypted IS 'AES-256-GCM ciphertext (nonce prefixed), encrypted by MOUSEION_SECRET';


--
-- Name: primary_goals; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.primary_goals (
    owner_id uuid NOT NULL,
    book_id uuid NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    reading_finished_at timestamptz,
    language text NOT NULL,
    CONSTRAINT primary_goals_language_nonempty CHECK ((btrim(language) <> ''::text))
);


--
-- Name: processing_history; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.processing_history (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    owner_id uuid NOT NULL,
    corpus_id uuid,
    operation text NOT NULL,
    status text NOT NULL,
    details jsonb DEFAULT '{}'::jsonb NOT NULL,
    started_at timestamptz DEFAULT now() NOT NULL,
    completed_at timestamptz
);


--
-- Name: reading_journeys; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.reading_journeys (
    owner_id uuid NOT NULL,
    revision bigint DEFAULT 0 NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    language text NOT NULL,
    CONSTRAINT reading_journeys_language_nonempty CHECK ((btrim(language) <> ''::text))
);


--
-- Name: selection_candidates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.selection_candidates (
    owner_id uuid NOT NULL,
    corpus_id text NOT NULL,
    language text NOT NULL,
    canonical_lemma text NOT NULL,
    upos text NOT NULL,
    occurrence_count integer NOT NULL,
    observed_forms jsonb NOT NULL,
    eligible_sentence_refs jsonb NOT NULL,
    provenance jsonb NOT NULL,
    selected_at timestamptz DEFAULT now() NOT NULL,
    ranking_global_pct double precision,
    ranking_corpus_pct double precision,
    ranking_priority boolean,
    ranking_cross_text integer,
    ranking_score double precision,
    ranked_at timestamptz,
    CONSTRAINT selection_candidates_occurrence_count_check CHECK ((occurrence_count > 0)),
    CONSTRAINT selection_candidates_ranking_corpus_pct_check CHECK (((ranking_corpus_pct >= (0)::double precision) AND (ranking_corpus_pct <= (1)::double precision))),
    CONSTRAINT selection_candidates_ranking_cross_text_check CHECK ((ranking_cross_text >= 1)),
    CONSTRAINT selection_candidates_ranking_global_pct_check CHECK (((ranking_global_pct >= (0)::double precision) AND (ranking_global_pct <= (1)::double precision)))
);


--
-- Name: sessions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.sessions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    token_hash text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    expires_at timestamptz NOT NULL,
    last_seen_at timestamptz DEFAULT now() NOT NULL
);


--
-- Name: shared_lemmas; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.shared_lemmas (
    id bigint NOT NULL,
    content_hash text NOT NULL,
    language text NOT NULL,
    canonical_lemma text NOT NULL,
    upos text NOT NULL,
    morphology jsonb DEFAULT '{}'::jsonb NOT NULL,
    frequency bigint NOT NULL,
    CONSTRAINT shared_lemmas_frequency_check CHECK ((frequency >= 0))
);


--
-- Name: shared_lemmas_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

ALTER TABLE public.shared_lemmas ALTER COLUMN id ADD GENERATED ALWAYS AS IDENTITY (
    SEQUENCE NAME public.shared_lemmas_id_seq
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1
);


--
-- Name: source_material_unit_snapshots; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.source_material_unit_snapshots (
    owner_id uuid NOT NULL,
    source_material_id uuid NOT NULL,
    schema_version integer NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    snapshot_id uuid DEFAULT gen_random_uuid() NOT NULL,
    content_revision_id uuid NOT NULL,
    CONSTRAINT source_material_unit_snapshots_schema_version_check CHECK ((schema_version > 0))
);


--
-- Name: supported_languages; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.supported_languages (
    language text NOT NULL,
    display_name text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL
);


--
-- Name: users; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.users (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    username text NOT NULL,
    is_admin boolean DEFAULT false NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    password_hash text,
    active_study_language text
);


--
-- Name: vocabulary_states; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.vocabulary_states (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    owner_id uuid NOT NULL,
    language text NOT NULL,
    canonical_lemma text NOT NULL,
    upos text NOT NULL,
    state text NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT vocabulary_states_state_check CHECK ((state = ANY (ARRAY['candidate'::text, 'accepted'::text, 'generated'::text, 'ignored'::text, 'known'::text])))
);


--
-- Name: analysis_jobs analysis_jobs_owner_display_number_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.analysis_jobs
    ADD CONSTRAINT analysis_jobs_owner_display_number_key UNIQUE (owner_id, display_number);


--
-- Name: analysis_jobs analysis_jobs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.analysis_jobs
    ADD CONSTRAINT analysis_jobs_pkey PRIMARY KEY (river_job_id);


--
-- Name: analysis_run_attempts analysis_run_attempts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.analysis_run_attempts
    ADD CONSTRAINT analysis_run_attempts_pkey PRIMARY KEY (id);


--
-- Name: analysis_run_attempts analysis_run_attempts_river_job_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.analysis_run_attempts
    ADD CONSTRAINT analysis_run_attempts_river_job_id_key UNIQUE (river_job_id);


--
-- Name: analysis_run_attempts analysis_run_attempts_run_id_attempt_number_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.analysis_run_attempts
    ADD CONSTRAINT analysis_run_attempts_run_id_attempt_number_key UNIQUE (run_id, attempt_number);


--
-- Name: analysis_runs analysis_runs_owner_id_id_source_material_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.analysis_runs
    ADD CONSTRAINT analysis_runs_owner_id_id_source_material_id_key UNIQUE (owner_id, id, source_material_id);


--
-- Name: analysis_runs analysis_runs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.analysis_runs
    ADD CONSTRAINT analysis_runs_pkey PRIMARY KEY (id);


--
-- Name: book_aliases book_aliases_connection_contract; Type: CHECK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE public.book_aliases
    ADD CONSTRAINT book_aliases_connection_contract CHECK ((((alias_type = 'catalog_entry'::text) AND (connection_id IS NOT NULL)) OR ((alias_type = 'strong_bibliographic'::text) AND (connection_id IS NULL)))) NOT VALID;


--
-- Name: book_aliases book_aliases_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.book_aliases
    ADD CONSTRAINT book_aliases_pkey PRIMARY KEY (id);


--
-- Name: book_current_analyses book_current_analyses_owner_id_analysis_run_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.book_current_analyses
    ADD CONSTRAINT book_current_analyses_owner_id_analysis_run_id_key UNIQUE (owner_id, analysis_run_id);


--
-- Name: book_current_analyses book_current_analyses_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.book_current_analyses
    ADD CONSTRAINT book_current_analyses_pkey PRIMARY KEY (owner_id, book_id);


--
-- Name: book_membership book_membership_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.book_membership
    ADD CONSTRAINT book_membership_pkey PRIMARY KEY (owner_id, book_id);


--
-- Name: books books_owner_id_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.books
    ADD CONSTRAINT books_owner_id_id_key UNIQUE (owner_id, id);


--
-- Name: books books_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.books
    ADD CONSTRAINT books_pkey PRIMARY KEY (id);


--
-- Name: cards cards_owner_id_dedup_key_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cards
    ADD CONSTRAINT cards_owner_id_dedup_key_key UNIQUE (owner_id, dedup_key);


--
-- Name: cards cards_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cards
    ADD CONSTRAINT cards_pkey PRIMARY KEY (id);


--
-- Name: catalogue_sync_status catalogue_sync_status_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.catalogue_sync_status
    ADD CONSTRAINT catalogue_sync_status_pkey PRIMARY KEY (owner_id, connection_id);


--
-- Name: corpora corpora_owner_id_id_analysis_run_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.corpora
    ADD CONSTRAINT corpora_owner_id_id_analysis_run_id_key UNIQUE (owner_id, id, analysis_run_id);


--
-- Name: corpora corpora_owner_id_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.corpora
    ADD CONSTRAINT corpora_owner_id_id_key UNIQUE (owner_id, id);


--
-- Name: corpora corpora_owner_id_id_source_material_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.corpora
    ADD CONSTRAINT corpora_owner_id_id_source_material_id_key UNIQUE (owner_id, id, source_material_id);


--
-- Name: corpora corpora_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.corpora
    ADD CONSTRAINT corpora_pkey PRIMARY KEY (id);


--
-- Name: corpus_sentences corpus_sentences_owner_id_corpus_id_analysis_run_id_sentenc_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.corpus_sentences
    ADD CONSTRAINT corpus_sentences_owner_id_corpus_id_analysis_run_id_sentenc_key UNIQUE (owner_id, corpus_id, analysis_run_id, sentence_ordinal);


--
-- Name: corpus_sentences corpus_sentences_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.corpus_sentences
    ADD CONSTRAINT corpus_sentences_pkey PRIMARY KEY (analysis_run_id, sentence_ordinal);


--
-- Name: corpus_tokens corpus_tokens_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.corpus_tokens
    ADD CONSTRAINT corpus_tokens_pkey PRIMARY KEY (analysis_run_id, sentence_ordinal, token_ordinal);


--
-- Name: curated_sentences curated_sentences_owner_id_language_canonical_lemma_upos_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.curated_sentences
    ADD CONSTRAINT curated_sentences_owner_id_language_canonical_lemma_upos_key UNIQUE (owner_id, language, canonical_lemma, upos);


--
-- Name: curated_sentences curated_sentences_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.curated_sentences
    ADD CONSTRAINT curated_sentences_pkey PRIMARY KEY (id);


--
-- Name: deck_preparation_batch_chunk_items deck_preparation_batch_chunk_item_run_id_generation_ordinal_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deck_preparation_batch_chunk_items
    ADD CONSTRAINT deck_preparation_batch_chunk_item_run_id_generation_ordinal_key UNIQUE (run_id, generation, ordinal);


--
-- Name: deck_preparation_batch_chunk_items deck_preparation_batch_chunk_items_chunk_id_ordinal_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deck_preparation_batch_chunk_items
    ADD CONSTRAINT deck_preparation_batch_chunk_items_chunk_id_ordinal_key UNIQUE (chunk_id, ordinal);


--
-- Name: deck_preparation_batch_chunk_items deck_preparation_batch_chunk_items_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deck_preparation_batch_chunk_items
    ADD CONSTRAINT deck_preparation_batch_chunk_items_pkey PRIMARY KEY (chunk_id, "position");


--
-- Name: deck_preparation_batch_chunks deck_preparation_batch_chunks_owner_id_preparation_id_run__key1; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deck_preparation_batch_chunks
    ADD CONSTRAINT deck_preparation_batch_chunks_owner_id_preparation_id_run__key1 UNIQUE (owner_id, preparation_id, run_id, id, generation);


--
-- Name: deck_preparation_batch_chunks deck_preparation_batch_chunks_owner_id_preparation_id_run_i_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deck_preparation_batch_chunks
    ADD CONSTRAINT deck_preparation_batch_chunks_owner_id_preparation_id_run_i_key UNIQUE (owner_id, preparation_id, run_id, id);


--
-- Name: deck_preparation_batch_chunks deck_preparation_batch_chunks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deck_preparation_batch_chunks
    ADD CONSTRAINT deck_preparation_batch_chunks_pkey PRIMARY KEY (id);


--
-- Name: deck_preparation_batch_chunks deck_preparation_batch_chunks_run_id_generation_chunk_index_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deck_preparation_batch_chunks
    ADD CONSTRAINT deck_preparation_batch_chunks_run_id_generation_chunk_index_key UNIQUE (run_id, generation, chunk_index);


--
-- Name: deck_preparation_manifest_items deck_preparation_manifest_ite_owner_id_preparation_id_run__key1; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deck_preparation_manifest_items
    ADD CONSTRAINT deck_preparation_manifest_ite_owner_id_preparation_id_run__key1 UNIQUE (owner_id, preparation_id, run_id, ordinal, candidate_digest);


--
-- Name: deck_preparation_manifest_items deck_preparation_manifest_ite_owner_id_preparation_id_run_i_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deck_preparation_manifest_items
    ADD CONSTRAINT deck_preparation_manifest_ite_owner_id_preparation_id_run_i_key UNIQUE (owner_id, preparation_id, run_id, ordinal);


--
-- Name: deck_preparation_manifest_items deck_preparation_manifest_items_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deck_preparation_manifest_items
    ADD CONSTRAINT deck_preparation_manifest_items_pkey PRIMARY KEY (run_id, ordinal);


--
-- Name: deck_preparation_manifest_items deck_preparation_manifest_items_run_id_candidate_digest_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deck_preparation_manifest_items
    ADD CONSTRAINT deck_preparation_manifest_items_run_id_candidate_digest_key UNIQUE (run_id, candidate_digest);


--
-- Name: deck_preparation_manifests deck_preparation_manifests_owner_id_preparation_id_run_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deck_preparation_manifests
    ADD CONSTRAINT deck_preparation_manifests_owner_id_preparation_id_run_id_key UNIQUE (owner_id, preparation_id, run_id);


--
-- Name: deck_preparation_manifests deck_preparation_manifests_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deck_preparation_manifests
    ADD CONSTRAINT deck_preparation_manifests_pkey PRIMARY KEY (run_id);


--
-- Name: deck_preparation_runs deck_preparation_runs_owner_id_preparation_id_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deck_preparation_runs
    ADD CONSTRAINT deck_preparation_runs_owner_id_preparation_id_id_key UNIQUE (owner_id, preparation_id, id);


--
-- Name: deck_preparation_runs deck_preparation_runs_owner_id_preparation_id_run_number_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deck_preparation_runs
    ADD CONSTRAINT deck_preparation_runs_owner_id_preparation_id_run_number_key UNIQUE (owner_id, preparation_id, run_number);


--
-- Name: deck_preparation_runs deck_preparation_runs_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deck_preparation_runs
    ADD CONSTRAINT deck_preparation_runs_pkey PRIMARY KEY (id);


--
-- Name: deck_preparation_translation_outcomes deck_preparation_translation__owner_id_preparation_id_run_i_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deck_preparation_translation_outcomes
    ADD CONSTRAINT deck_preparation_translation__owner_id_preparation_id_run_i_key UNIQUE (owner_id, preparation_id, run_id, ordinal);


--
-- Name: deck_preparation_translation_outcomes deck_preparation_translation_outcomes_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deck_preparation_translation_outcomes
    ADD CONSTRAINT deck_preparation_translation_outcomes_pkey PRIMARY KEY (run_id, ordinal);


--
-- Name: deck_preparation_vocabulary deck_preparation_vocabulary_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deck_preparation_vocabulary
    ADD CONSTRAINT deck_preparation_vocabulary_pkey PRIMARY KEY (owner_id, deck_preparation_id, language, canonical_lemma, upos);


--
-- Name: deck_preparations deck_preparations_owner_id_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deck_preparations
    ADD CONSTRAINT deck_preparations_owner_id_id_key UNIQUE (owner_id, id);


--
-- Name: deck_preparations deck_preparations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deck_preparations
    ADD CONSTRAINT deck_preparations_pkey PRIMARY KEY (id);


--
-- Name: decks decks_owner_id_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.decks
    ADD CONSTRAINT decks_owner_id_id_key UNIQUE (owner_id, id);


--
-- Name: decks decks_owner_id_language_name_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.decks
    ADD CONSTRAINT decks_owner_id_language_name_key UNIQUE (owner_id, language, name);


--
-- Name: decks decks_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.decks
    ADD CONSTRAINT decks_pkey PRIMARY KEY (id);


--
-- Name: enrichment_cache enrichment_cache_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.enrichment_cache
    ADD CONSTRAINT enrichment_cache_pkey PRIMARY KEY (language, canonical_lemma, upos, provider, provider_version, sentence_hash, target_language, dictionary_provider_version);


--
-- Name: example_sentences example_sentences_owner_id_corpus_id_sentence_key_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.example_sentences
    ADD CONSTRAINT example_sentences_owner_id_corpus_id_sentence_key_key UNIQUE (owner_id, corpus_id, sentence_key);


--
-- Name: example_sentences example_sentences_owner_id_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.example_sentences
    ADD CONSTRAINT example_sentences_owner_id_id_key UNIQUE (owner_id, id);


--
-- Name: example_sentences example_sentences_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.example_sentences
    ADD CONSTRAINT example_sentences_pkey PRIMARY KEY (id);


--
-- Name: frequency_datasets frequency_datasets_language_name_version_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.frequency_datasets
    ADD CONSTRAINT frequency_datasets_language_name_version_key UNIQUE (language, name, version);


--
-- Name: frequency_datasets frequency_datasets_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.frequency_datasets
    ADD CONSTRAINT frequency_datasets_pkey PRIMARY KEY (id);


--
-- Name: frequency_entries frequency_entries_dataset_id_rank_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.frequency_entries
    ADD CONSTRAINT frequency_entries_dataset_id_rank_key UNIQUE (dataset_id, rank);


--
-- Name: frequency_entries frequency_entries_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.frequency_entries
    ADD CONSTRAINT frequency_entries_pkey PRIMARY KEY (dataset_id, canonical_lemma, upos);


--
-- Name: generated_vocabulary generated_vocabulary_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.generated_vocabulary
    ADD CONSTRAINT generated_vocabulary_pkey PRIMARY KEY (owner_id, language, canonical_lemma, upos);


--
-- Name: known_vocabulary known_vocabulary_owner_id_language_canonical_lemma_upos_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.known_vocabulary
    ADD CONSTRAINT known_vocabulary_owner_id_language_canonical_lemma_upos_key UNIQUE (owner_id, language, canonical_lemma, upos);


--
-- Name: known_vocabulary known_vocabulary_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.known_vocabulary
    ADD CONSTRAINT known_vocabulary_pkey PRIMARY KEY (id);


--
-- Name: normalized_corpus_artifacts normalized_corpus_artifacts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.normalized_corpus_artifacts
    ADD CONSTRAINT normalized_corpus_artifacts_pkey PRIMARY KEY (content_hash);


--
-- Name: opds_connections opds_connections_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.opds_connections
    ADD CONSTRAINT opds_connections_pkey PRIMARY KEY (id);


--
-- Name: primary_goals primary_goals_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.primary_goals
    ADD CONSTRAINT primary_goals_pkey PRIMARY KEY (owner_id, language);


--
-- Name: processing_history processing_history_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.processing_history
    ADD CONSTRAINT processing_history_pkey PRIMARY KEY (id);


--
-- Name: reading_journey_membership reading_journey_membership_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reading_journey_membership
    ADD CONSTRAINT reading_journey_membership_pkey PRIMARY KEY (owner_id, language, book_id);


--
-- Name: reading_journeys reading_journeys_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reading_journeys
    ADD CONSTRAINT reading_journeys_pkey PRIMARY KEY (owner_id, language);


--
-- Name: selection_candidates selection_candidates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.selection_candidates
    ADD CONSTRAINT selection_candidates_pkey PRIMARY KEY (owner_id, corpus_id, language, canonical_lemma, upos);


--
-- Name: sessions sessions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sessions
    ADD CONSTRAINT sessions_pkey PRIMARY KEY (id);


--
-- Name: sessions sessions_token_hash_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sessions
    ADD CONSTRAINT sessions_token_hash_key UNIQUE (token_hash);


--
-- Name: shared_lemmas shared_lemmas_content_hash_canonical_lemma_upos_morphology_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.shared_lemmas
    ADD CONSTRAINT shared_lemmas_content_hash_canonical_lemma_upos_morphology_key UNIQUE (content_hash, canonical_lemma, upos, morphology);


--
-- Name: shared_lemmas shared_lemmas_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.shared_lemmas
    ADD CONSTRAINT shared_lemmas_pkey PRIMARY KEY (id);


--
-- Name: source_content_revisions source_content_revisions_owner_id_revision_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.source_content_revisions
    ADD CONSTRAINT source_content_revisions_owner_id_revision_id_key UNIQUE (owner_id, revision_id);


--
-- Name: source_content_revisions source_content_revisions_owner_id_source_material_id_conten_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.source_content_revisions
    ADD CONSTRAINT source_content_revisions_owner_id_source_material_id_conten_key UNIQUE (owner_id, source_material_id, content_digest);


--
-- Name: source_content_revisions source_content_revisions_owner_id_source_material_id_revisi_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.source_content_revisions
    ADD CONSTRAINT source_content_revisions_owner_id_source_material_id_revisi_key UNIQUE (owner_id, source_material_id, revision_id);


--
-- Name: source_content_revisions source_content_revisions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.source_content_revisions
    ADD CONSTRAINT source_content_revisions_pkey PRIMARY KEY (revision_id);


--
-- Name: source_material_unit_snapshots source_material_unit_snapshots_identity; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.source_material_unit_snapshots
    ADD CONSTRAINT source_material_unit_snapshots_identity UNIQUE (owner_id, source_material_id, snapshot_id);


--
-- Name: source_material_unit_snapshots source_material_unit_snapshots_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.source_material_unit_snapshots
    ADD CONSTRAINT source_material_unit_snapshots_pkey PRIMARY KEY (owner_id, source_material_id, snapshot_id);


--
-- Name: source_material_units source_material_units_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.source_material_units
    ADD CONSTRAINT source_material_units_pkey PRIMARY KEY (owner_id, source_material_id, snapshot_id, unit_id);


--
-- Name: source_material_units source_material_units_snapshot_order_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.source_material_units
    ADD CONSTRAINT source_material_units_snapshot_order_key UNIQUE (owner_id, source_material_id, snapshot_id, unit_order);


--
-- Name: source_materials source_materials_owner_id_id_book_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.source_materials
    ADD CONSTRAINT source_materials_owner_id_id_book_id_key UNIQUE (owner_id, id, book_id);


--
-- Name: source_materials source_materials_owner_id_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.source_materials
    ADD CONSTRAINT source_materials_owner_id_id_key UNIQUE (owner_id, id);


--
-- Name: source_materials source_materials_owner_id_source_identifier_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.source_materials
    ADD CONSTRAINT source_materials_owner_id_source_identifier_key UNIQUE (owner_id, source_identifier);


--
-- Name: source_materials source_materials_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.source_materials
    ADD CONSTRAINT source_materials_pkey PRIMARY KEY (id);


--
-- Name: supported_languages supported_languages_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.supported_languages
    ADD CONSTRAINT supported_languages_pkey PRIMARY KEY (language);


--
-- Name: users users_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);


--
-- Name: users users_username_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_username_key UNIQUE (username);


--
-- Name: vocabulary_states vocabulary_states_owner_id_language_canonical_lemma_upos_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vocabulary_states
    ADD CONSTRAINT vocabulary_states_owner_id_language_canonical_lemma_upos_key UNIQUE (owner_id, language, canonical_lemma, upos);


--
-- Name: vocabulary_states vocabulary_states_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vocabulary_states
    ADD CONSTRAINT vocabulary_states_pkey PRIMARY KEY (id);


--
-- Name: analysis_jobs_owner_created_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX analysis_jobs_owner_created_idx ON public.analysis_jobs USING btree (owner_id, created_at DESC);


--
-- Name: analysis_jobs_run_identity; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX analysis_jobs_run_identity ON public.analysis_jobs USING btree (owner_id, analysis_run_id) WHERE (analysis_run_id IS NOT NULL);


--
-- Name: book_aliases_catalogue_entry_identity; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX book_aliases_catalogue_entry_identity ON public.book_aliases USING btree (owner_id, connection_id, namespace, value) WHERE (connection_id IS NOT NULL);


--
-- Name: book_aliases_strong_bibliographic_identity; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX book_aliases_strong_bibliographic_identity ON public.book_aliases USING btree (owner_id, namespace, value) WHERE (connection_id IS NULL);


--
-- Name: corpora_run_identity; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX corpora_run_identity ON public.corpora USING btree (owner_id, analysis_run_id) WHERE (analysis_run_id IS NOT NULL);


--
-- Name: corpus_tokens_owner_language_canonical_lemma_upos_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX corpus_tokens_owner_language_canonical_lemma_upos_idx ON public.corpus_tokens USING btree (owner_id, language, canonical_lemma, upos);


--
-- Name: corpus_tokens_owner_language_dependency_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX corpus_tokens_owner_language_dependency_idx ON public.corpus_tokens USING btree (owner_id, language, dependency);


--
-- Name: corpus_tokens_owner_language_surface_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX corpus_tokens_owner_language_surface_idx ON public.corpus_tokens USING btree (owner_id, language, surface);


--
-- Name: deck_preparation_batch_chunks_batch_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX deck_preparation_batch_chunks_batch_id_idx ON public.deck_preparation_batch_chunks USING btree (batch_id) WHERE (batch_id IS NOT NULL);


--
-- Name: deck_preparation_batch_chunks_cleanup_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX deck_preparation_batch_chunks_cleanup_idx ON public.deck_preparation_batch_chunks USING btree (cleanup_completed_at, updated_at) WHERE ((state = ANY (ARRAY['completed'::text, 'cancelled'::text])) AND ((input_file_cleanup_state = ANY (ARRAY['pending'::text, 'failed'::text])) OR (output_file_cleanup_state = ANY (ARRAY['pending'::text, 'failed'::text])) OR (error_file_cleanup_state = ANY (ARRAY['pending'::text, 'failed'::text]))));


--
-- Name: deck_preparation_batch_chunks_input_file_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX deck_preparation_batch_chunks_input_file_id_idx ON public.deck_preparation_batch_chunks USING btree (input_file_id) WHERE (input_file_id IS NOT NULL);


--
-- Name: deck_preparation_batch_chunks_reconciliation_lease_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX deck_preparation_batch_chunks_reconciliation_lease_idx ON public.deck_preparation_batch_chunks USING btree (reconciliation_lease_expires_at) WHERE (reconciliation_claim_token IS NOT NULL);


--
-- Name: deck_preparation_batch_chunks_run_state_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX deck_preparation_batch_chunks_run_state_idx ON public.deck_preparation_batch_chunks USING btree (run_id, state, generation, chunk_index);


--
-- Name: deck_preparation_batch_chunks_submission_lease_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX deck_preparation_batch_chunks_submission_lease_idx ON public.deck_preparation_batch_chunks USING btree (submission_lease_expires_at) WHERE (submission_claim_token IS NOT NULL);


--
-- Name: deck_preparation_outcomes_pending_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX deck_preparation_outcomes_pending_idx ON public.deck_preparation_translation_outcomes USING btree (next_attempt_at) WHERE (state = 'pending'::text);


--
-- Name: deck_preparation_outcomes_river_job_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX deck_preparation_outcomes_river_job_idx ON public.deck_preparation_translation_outcomes USING btree (river_job_id) WHERE (river_job_id IS NOT NULL);


--
-- Name: deck_preparation_outcomes_run_state_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX deck_preparation_outcomes_run_state_idx ON public.deck_preparation_translation_outcomes USING btree (run_id, state);


--
-- Name: deck_preparation_outcomes_running_lease_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX deck_preparation_outcomes_running_lease_idx ON public.deck_preparation_translation_outcomes USING btree (lease_expires_at) WHERE (state = 'running'::text);


--
-- Name: deck_preparation_runs_finalizer_lease_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX deck_preparation_runs_finalizer_lease_idx ON public.deck_preparation_runs USING btree (finalization_lease_expires_at) WHERE (finalization_claim_token IS NOT NULL);


--
-- Name: deck_preparation_runs_owner_history_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX deck_preparation_runs_owner_history_idx ON public.deck_preparation_runs USING btree (owner_id, preparation_id, run_number DESC);


--
-- Name: deck_preparation_runs_stuck_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX deck_preparation_runs_stuck_idx ON public.deck_preparation_runs USING btree (state, updated_at) WHERE (state = ANY (ARRAY['translating'::text, 'finalizing'::text]));


--
-- Name: deck_preparation_vocabulary_owner_identity_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX deck_preparation_vocabulary_owner_identity_idx ON public.deck_preparation_vocabulary USING btree (owner_id, language, canonical_lemma, upos);


--
-- Name: deck_preparations_analysis_identity; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX deck_preparations_analysis_identity ON public.deck_preparations USING btree (owner_id, source_material_id, analysis_run_id) WHERE (analysis_run_id IS NOT NULL);


--
-- Name: deck_preparations_legacy_identity; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX deck_preparations_legacy_identity ON public.deck_preparations USING btree (owner_id, source_material_id, content_hash) WHERE ((book_id IS NULL) AND (analysis_run_id IS NULL));


--
-- Name: deck_preparations_one_current_per_book; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX deck_preparations_one_current_per_book ON public.deck_preparations USING btree (owner_id, book_id) WHERE ((book_id IS NOT NULL) AND (retired_at IS NULL));


--
-- Name: deck_preparations_one_studying_per_owner; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX deck_preparations_one_studying_per_owner ON public.deck_preparations USING btree (owner_id) WHERE (studying_at IS NOT NULL);


--
-- Name: deck_preparations_owner_updated_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX deck_preparations_owner_updated_idx ON public.deck_preparations USING btree (owner_id, updated_at DESC);


--
-- Name: example_sentences_identity_chosen_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX example_sentences_identity_chosen_idx ON public.example_sentences USING btree (owner_id, corpus_id, language, canonical_lemma, upos) WHERE is_chosen;


--
-- Name: example_sentences_identity_rank_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX example_sentences_identity_rank_idx ON public.example_sentences USING btree (owner_id, corpus_id, language, canonical_lemma, upos, selection_rank) WHERE (language IS NOT NULL);


--
-- Name: frequency_datasets_one_active_per_language; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX frequency_datasets_one_active_per_language ON public.frequency_datasets USING btree (language) WHERE active;


--
-- Name: frequency_entries_lookup_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX frequency_entries_lookup_idx ON public.frequency_entries USING btree (language, canonical_lemma, upos);


--
-- Name: opds_connections_owner_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX opds_connections_owner_id_idx ON public.opds_connections USING btree (owner_id);


--
-- Name: opds_connections_owner_name_key; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX opds_connections_owner_name_key ON public.opds_connections USING btree (owner_id, name) WHERE (owner_id IS NOT NULL);


--
-- Name: primary_goals_book_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX primary_goals_book_idx ON public.primary_goals USING btree (book_id);


--
-- Name: processing_history_owner_started_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX processing_history_owner_started_idx ON public.processing_history USING btree (owner_id, started_at DESC);


--
-- Name: reading_journey_membership_order_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX reading_journey_membership_order_idx ON public.reading_journey_membership USING btree (owner_id, language, "position", created_at, book_id);


--
-- Name: selection_candidates_owner_identity_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX selection_candidates_owner_identity_idx ON public.selection_candidates USING btree (owner_id, language, canonical_lemma, upos);


--
-- Name: sessions_expires_at_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX sessions_expires_at_idx ON public.sessions USING btree (expires_at);


--
-- Name: sessions_user_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX sessions_user_id_idx ON public.sessions USING btree (user_id);


--
-- Name: shared_lemmas_identity_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX shared_lemmas_identity_idx ON public.shared_lemmas USING btree (language, canonical_lemma, upos);


--
-- Name: source_materials_owner_book_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX source_materials_owner_book_idx ON public.source_materials USING btree (owner_id, book_id);


--
-- Name: deck_preparation_batch_chunk_items deck_preparation_batch_chunk_items_immutable; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER deck_preparation_batch_chunk_items_immutable BEFORE UPDATE ON public.deck_preparation_batch_chunk_items FOR EACH ROW EXECUTE FUNCTION public.reject_deck_preparation_manifest_mutation();


--
-- Name: deck_preparation_batch_chunks deck_preparation_batch_chunks_identity_immutable; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER deck_preparation_batch_chunks_identity_immutable BEFORE UPDATE ON public.deck_preparation_batch_chunks FOR EACH ROW EXECUTE FUNCTION public.reject_deck_preparation_batch_chunk_identity_mutation();


--
-- Name: deck_preparation_manifest_items deck_preparation_manifest_items_immutable; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER deck_preparation_manifest_items_immutable BEFORE UPDATE ON public.deck_preparation_manifest_items FOR EACH ROW EXECUTE FUNCTION public.reject_deck_preparation_manifest_mutation();


--
-- Name: deck_preparation_manifests deck_preparation_manifests_immutable; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER deck_preparation_manifests_immutable BEFORE UPDATE ON public.deck_preparation_manifests FOR EACH ROW EXECUTE FUNCTION public.reject_deck_preparation_manifest_mutation();


--
-- Name: deck_preparation_runs deck_preparation_runs_identity_immutable; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER deck_preparation_runs_identity_immutable BEFORE UPDATE ON public.deck_preparation_runs FOR EACH ROW EXECUTE FUNCTION public.reject_deck_preparation_run_identity_mutation();


--
-- Name: enrichment_cache enrichment_cache_immutable; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER enrichment_cache_immutable BEFORE UPDATE ON public.enrichment_cache FOR EACH ROW EXECUTE FUNCTION public.reject_enrichment_cache_mutation();


--
-- Name: source_content_revisions source_content_revisions_immutable; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER source_content_revisions_immutable BEFORE UPDATE ON public.source_content_revisions FOR EACH ROW EXECUTE FUNCTION public.reject_source_content_revision_mutation();


--
-- Name: source_material_unit_snapshots source_material_unit_snapshots_immutable; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER source_material_unit_snapshots_immutable BEFORE UPDATE ON public.source_material_unit_snapshots FOR EACH ROW EXECUTE FUNCTION public.reject_source_content_revision_mutation();


--
-- Name: source_material_units source_material_units_immutable; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER source_material_units_immutable BEFORE UPDATE ON public.source_material_units FOR EACH ROW EXECUTE FUNCTION public.reject_source_content_revision_mutation();


--
-- Name: source_materials source_materials_content_immutable; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER source_materials_content_immutable BEFORE UPDATE ON public.source_materials FOR EACH ROW EXECUTE FUNCTION public.reject_source_material_content_replacement();


--
-- Name: analysis_jobs analysis_jobs_owner_id_corpus_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.analysis_jobs
    ADD CONSTRAINT analysis_jobs_owner_id_corpus_id_fkey FOREIGN KEY (owner_id, corpus_id) REFERENCES public.corpora(owner_id, id) ON DELETE SET NULL;


--
-- Name: analysis_jobs analysis_jobs_owner_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.analysis_jobs
    ADD CONSTRAINT analysis_jobs_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: analysis_jobs analysis_jobs_owner_id_source_material_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.analysis_jobs
    ADD CONSTRAINT analysis_jobs_owner_id_source_material_id_fkey FOREIGN KEY (owner_id, source_material_id) REFERENCES public.source_materials(owner_id, id) ON DELETE CASCADE;


--
-- Name: analysis_jobs analysis_jobs_run_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.analysis_jobs
    ADD CONSTRAINT analysis_jobs_run_fkey FOREIGN KEY (owner_id, analysis_run_id, source_material_id) REFERENCES public.analysis_runs(owner_id, id, source_material_id);


--
-- Name: analysis_run_attempts analysis_run_attempts_owner_id_run_id_source_material_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.analysis_run_attempts
    ADD CONSTRAINT analysis_run_attempts_owner_id_run_id_source_material_id_fkey FOREIGN KEY (owner_id, run_id, source_material_id) REFERENCES public.analysis_runs(owner_id, id, source_material_id) ON DELETE CASCADE;


--
-- Name: analysis_runs analysis_runs_owner_id_corpus_id_source_material_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.analysis_runs
    ADD CONSTRAINT analysis_runs_owner_id_corpus_id_source_material_id_fkey FOREIGN KEY (owner_id, corpus_id, source_material_id) REFERENCES public.corpora(owner_id, id, source_material_id);


--
-- Name: analysis_runs analysis_runs_owner_id_source_material_id_content_revision_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.analysis_runs
    ADD CONSTRAINT analysis_runs_owner_id_source_material_id_content_revision_fkey FOREIGN KEY (owner_id, source_material_id, content_revision_id) REFERENCES public.source_content_revisions(owner_id, source_material_id, revision_id);


--
-- Name: analysis_runs analysis_runs_owner_id_source_material_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.analysis_runs
    ADD CONSTRAINT analysis_runs_owner_id_source_material_id_fkey FOREIGN KEY (owner_id, source_material_id) REFERENCES public.source_materials(owner_id, id) ON DELETE CASCADE;


--
-- Name: book_aliases book_aliases_connection_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.book_aliases
    ADD CONSTRAINT book_aliases_connection_fkey FOREIGN KEY (connection_id) REFERENCES public.opds_connections(id) ON DELETE CASCADE;


--
-- Name: book_aliases book_aliases_owner_id_book_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.book_aliases
    ADD CONSTRAINT book_aliases_owner_id_book_id_fkey FOREIGN KEY (owner_id, book_id) REFERENCES public.books(owner_id, id) ON DELETE CASCADE;


--
-- Name: book_current_analyses book_current_analyses_owner_id_analysis_run_id_source_mate_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.book_current_analyses
    ADD CONSTRAINT book_current_analyses_owner_id_analysis_run_id_source_mate_fkey FOREIGN KEY (owner_id, analysis_run_id, source_material_id) REFERENCES public.analysis_runs(owner_id, id, source_material_id);


--
-- Name: book_current_analyses book_current_analyses_owner_id_book_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.book_current_analyses
    ADD CONSTRAINT book_current_analyses_owner_id_book_id_fkey FOREIGN KEY (owner_id, book_id) REFERENCES public.books(owner_id, id) ON DELETE CASCADE;


--
-- Name: book_current_analyses book_current_analyses_owner_id_source_material_id_book_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.book_current_analyses
    ADD CONSTRAINT book_current_analyses_owner_id_source_material_id_book_id_fkey FOREIGN KEY (owner_id, source_material_id, book_id) REFERENCES public.source_materials(owner_id, id, book_id);


--
-- Name: book_membership book_membership_owner_id_book_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.book_membership
    ADD CONSTRAINT book_membership_owner_id_book_id_fkey FOREIGN KEY (owner_id, book_id) REFERENCES public.books(owner_id, id) ON DELETE CASCADE;


--
-- Name: books books_owner_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.books
    ADD CONSTRAINT books_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: cards cards_owner_id_deck_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cards
    ADD CONSTRAINT cards_owner_id_deck_id_fkey FOREIGN KEY (owner_id, deck_id) REFERENCES public.decks(owner_id, id) ON DELETE CASCADE;


--
-- Name: cards cards_owner_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cards
    ADD CONSTRAINT cards_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: catalogue_sync_status catalogue_sync_status_connection_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.catalogue_sync_status
    ADD CONSTRAINT catalogue_sync_status_connection_id_fkey FOREIGN KEY (connection_id) REFERENCES public.opds_connections(id) ON DELETE CASCADE;


--
-- Name: catalogue_sync_status catalogue_sync_status_owner_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.catalogue_sync_status
    ADD CONSTRAINT catalogue_sync_status_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: corpora corpora_analysis_run_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.corpora
    ADD CONSTRAINT corpora_analysis_run_fkey FOREIGN KEY (owner_id, analysis_run_id, source_material_id) REFERENCES public.analysis_runs(owner_id, id, source_material_id);


--
-- Name: corpora corpora_artifact_hash_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.corpora
    ADD CONSTRAINT corpora_artifact_hash_fkey FOREIGN KEY (artifact_hash) REFERENCES public.normalized_corpus_artifacts(content_hash);


--
-- Name: corpora corpora_owner_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.corpora
    ADD CONSTRAINT corpora_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: corpora corpora_owner_id_source_material_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.corpora
    ADD CONSTRAINT corpora_owner_id_source_material_id_fkey FOREIGN KEY (owner_id, source_material_id) REFERENCES public.source_materials(owner_id, id) ON DELETE CASCADE;


--
-- Name: corpus_sentences corpus_sentences_owner_id_corpus_id_analysis_run_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.corpus_sentences
    ADD CONSTRAINT corpus_sentences_owner_id_corpus_id_analysis_run_id_fkey FOREIGN KEY (owner_id, corpus_id, analysis_run_id) REFERENCES public.corpora(owner_id, id, analysis_run_id) ON DELETE CASCADE;


--
-- Name: corpus_tokens corpus_tokens_owner_id_corpus_id_analysis_run_id_sentence__fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.corpus_tokens
    ADD CONSTRAINT corpus_tokens_owner_id_corpus_id_analysis_run_id_sentence__fkey FOREIGN KEY (owner_id, corpus_id, analysis_run_id, sentence_ordinal) REFERENCES public.corpus_sentences(owner_id, corpus_id, analysis_run_id, sentence_ordinal) ON DELETE CASCADE;


--
-- Name: curated_sentences curated_sentences_owner_id_example_sentence_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.curated_sentences
    ADD CONSTRAINT curated_sentences_owner_id_example_sentence_id_fkey FOREIGN KEY (owner_id, example_sentence_id) REFERENCES public.example_sentences(owner_id, id) ON DELETE CASCADE;


--
-- Name: curated_sentences curated_sentences_owner_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.curated_sentences
    ADD CONSTRAINT curated_sentences_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: deck_preparation_batch_chunk_items deck_preparation_batch_chunk__owner_id_preparation_id_run__fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deck_preparation_batch_chunk_items
    ADD CONSTRAINT deck_preparation_batch_chunk__owner_id_preparation_id_run__fkey FOREIGN KEY (owner_id, preparation_id, run_id, chunk_id, generation) REFERENCES public.deck_preparation_batch_chunks(owner_id, preparation_id, run_id, id, generation) ON DELETE CASCADE;


--
-- Name: deck_preparation_batch_chunk_items deck_preparation_batch_chunk_owner_id_preparation_id_run__fkey1; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deck_preparation_batch_chunk_items
    ADD CONSTRAINT deck_preparation_batch_chunk_owner_id_preparation_id_run__fkey1 FOREIGN KEY (owner_id, preparation_id, run_id, ordinal, candidate_digest) REFERENCES public.deck_preparation_manifest_items(owner_id, preparation_id, run_id, ordinal, candidate_digest) ON DELETE CASCADE;


--
-- Name: deck_preparation_batch_chunks deck_preparation_batch_chunks_owner_id_preparation_id_run__fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deck_preparation_batch_chunks
    ADD CONSTRAINT deck_preparation_batch_chunks_owner_id_preparation_id_run__fkey FOREIGN KEY (owner_id, preparation_id, run_id) REFERENCES public.deck_preparation_runs(owner_id, preparation_id, id) ON DELETE CASCADE;


--
-- Name: deck_preparation_manifest_items deck_preparation_manifest_ite_owner_id_preparation_id_run__fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deck_preparation_manifest_items
    ADD CONSTRAINT deck_preparation_manifest_ite_owner_id_preparation_id_run__fkey FOREIGN KEY (owner_id, preparation_id, run_id) REFERENCES public.deck_preparation_manifests(owner_id, preparation_id, run_id) ON DELETE CASCADE;


--
-- Name: deck_preparation_manifests deck_preparation_manifests_owner_id_preparation_id_run_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deck_preparation_manifests
    ADD CONSTRAINT deck_preparation_manifests_owner_id_preparation_id_run_id_fkey FOREIGN KEY (owner_id, preparation_id, run_id) REFERENCES public.deck_preparation_runs(owner_id, preparation_id, id) ON DELETE CASCADE;


--
-- Name: deck_preparation_runs deck_preparation_runs_owner_id_preparation_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deck_preparation_runs
    ADD CONSTRAINT deck_preparation_runs_owner_id_preparation_id_fkey FOREIGN KEY (owner_id, preparation_id) REFERENCES public.deck_preparations(owner_id, id) ON DELETE CASCADE;


--
-- Name: deck_preparation_translation_outcomes deck_preparation_translation__owner_id_preparation_id_run__fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deck_preparation_translation_outcomes
    ADD CONSTRAINT deck_preparation_translation__owner_id_preparation_id_run__fkey FOREIGN KEY (owner_id, preparation_id, run_id, ordinal) REFERENCES public.deck_preparation_manifest_items(owner_id, preparation_id, run_id, ordinal) ON DELETE CASCADE;


--
-- Name: deck_preparation_vocabulary deck_preparation_vocabulary_owner_id_deck_preparation_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deck_preparation_vocabulary
    ADD CONSTRAINT deck_preparation_vocabulary_owner_id_deck_preparation_id_fkey FOREIGN KEY (owner_id, deck_preparation_id) REFERENCES public.deck_preparations(owner_id, id) ON DELETE CASCADE;


--
-- Name: deck_preparation_vocabulary deck_preparation_vocabulary_owner_id_language_canonical_le_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deck_preparation_vocabulary
    ADD CONSTRAINT deck_preparation_vocabulary_owner_id_language_canonical_le_fkey FOREIGN KEY (owner_id, language, canonical_lemma, upos) REFERENCES public.generated_vocabulary(owner_id, language, canonical_lemma, upos);


--
-- Name: deck_preparations deck_preparations_analysis_run_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deck_preparations
    ADD CONSTRAINT deck_preparations_analysis_run_fkey FOREIGN KEY (owner_id, analysis_run_id, source_material_id) REFERENCES public.analysis_runs(owner_id, id, source_material_id);


--
-- Name: deck_preparations deck_preparations_book_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deck_preparations
    ADD CONSTRAINT deck_preparations_book_fkey FOREIGN KEY (owner_id, book_id) REFERENCES public.books(owner_id, id) ON DELETE CASCADE;


--
-- Name: deck_preparations deck_preparations_current_run_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deck_preparations
    ADD CONSTRAINT deck_preparations_current_run_fkey FOREIGN KEY (owner_id, id, current_run_id) REFERENCES public.deck_preparation_runs(owner_id, preparation_id, id);


--
-- Name: deck_preparations deck_preparations_owner_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deck_preparations
    ADD CONSTRAINT deck_preparations_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: deck_preparations deck_preparations_owner_id_source_material_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.deck_preparations
    ADD CONSTRAINT deck_preparations_owner_id_source_material_id_fkey FOREIGN KEY (owner_id, source_material_id) REFERENCES public.source_materials(owner_id, id) ON DELETE CASCADE;


--
-- Name: decks decks_owner_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.decks
    ADD CONSTRAINT decks_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: example_sentences example_sentences_owner_id_corpus_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.example_sentences
    ADD CONSTRAINT example_sentences_owner_id_corpus_id_fkey FOREIGN KEY (owner_id, corpus_id) REFERENCES public.corpora(owner_id, id) ON DELETE CASCADE;


--
-- Name: example_sentences example_sentences_owner_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.example_sentences
    ADD CONSTRAINT example_sentences_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: frequency_entries frequency_entries_dataset_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.frequency_entries
    ADD CONSTRAINT frequency_entries_dataset_id_fkey FOREIGN KEY (dataset_id) REFERENCES public.frequency_datasets(id) ON DELETE CASCADE;


--
-- Name: generated_vocabulary generated_vocabulary_owner_id_first_deck_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.generated_vocabulary
    ADD CONSTRAINT generated_vocabulary_owner_id_first_deck_id_fkey FOREIGN KEY (owner_id, first_deck_id) REFERENCES public.decks(owner_id, id);


--
-- Name: generated_vocabulary generated_vocabulary_owner_id_first_source_material_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.generated_vocabulary
    ADD CONSTRAINT generated_vocabulary_owner_id_first_source_material_id_fkey FOREIGN KEY (owner_id, first_source_material_id) REFERENCES public.source_materials(owner_id, id) ON DELETE SET NULL (first_source_material_id);


--
-- Name: generated_vocabulary generated_vocabulary_owner_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.generated_vocabulary
    ADD CONSTRAINT generated_vocabulary_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: known_vocabulary known_vocabulary_owner_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.known_vocabulary
    ADD CONSTRAINT known_vocabulary_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: opds_connections opds_connections_owner_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.opds_connections
    ADD CONSTRAINT opds_connections_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: primary_goals primary_goals_owner_id_book_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.primary_goals
    ADD CONSTRAINT primary_goals_owner_id_book_id_fkey FOREIGN KEY (owner_id, book_id) REFERENCES public.books(owner_id, id) ON DELETE CASCADE;


--
-- Name: primary_goals primary_goals_owner_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.primary_goals
    ADD CONSTRAINT primary_goals_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: processing_history processing_history_owner_id_corpus_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.processing_history
    ADD CONSTRAINT processing_history_owner_id_corpus_id_fkey FOREIGN KEY (owner_id, corpus_id) REFERENCES public.corpora(owner_id, id) ON DELETE CASCADE;


--
-- Name: processing_history processing_history_owner_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.processing_history
    ADD CONSTRAINT processing_history_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: reading_journey_membership reading_journey_membership_journey_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reading_journey_membership
    ADD CONSTRAINT reading_journey_membership_journey_fkey FOREIGN KEY (owner_id, language) REFERENCES public.reading_journeys(owner_id, language) ON DELETE CASCADE;


--
-- Name: reading_journey_membership reading_journey_membership_owner_id_book_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reading_journey_membership
    ADD CONSTRAINT reading_journey_membership_owner_id_book_id_fkey FOREIGN KEY (owner_id, book_id) REFERENCES public.books(owner_id, id) ON DELETE CASCADE;


--
-- Name: reading_journeys reading_journeys_owner_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.reading_journeys
    ADD CONSTRAINT reading_journeys_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: selection_candidates selection_candidates_owner_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.selection_candidates
    ADD CONSTRAINT selection_candidates_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: sessions sessions_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.sessions
    ADD CONSTRAINT sessions_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: shared_lemmas shared_lemmas_content_hash_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.shared_lemmas
    ADD CONSTRAINT shared_lemmas_content_hash_fkey FOREIGN KEY (content_hash) REFERENCES public.normalized_corpus_artifacts(content_hash) ON DELETE CASCADE;


--
-- Name: source_content_revisions source_content_revisions_owner_id_source_material_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.source_content_revisions
    ADD CONSTRAINT source_content_revisions_owner_id_source_material_id_fkey FOREIGN KEY (owner_id, source_material_id) REFERENCES public.source_materials(owner_id, id) ON DELETE CASCADE;


--
-- Name: source_material_unit_snapshots source_material_unit_snapshots_content_revision_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.source_material_unit_snapshots
    ADD CONSTRAINT source_material_unit_snapshots_content_revision_fkey FOREIGN KEY (owner_id, source_material_id, content_revision_id) REFERENCES public.source_content_revisions(owner_id, source_material_id, revision_id);


--
-- Name: source_material_unit_snapshots source_material_unit_snapshots_owner_id_source_material_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.source_material_unit_snapshots
    ADD CONSTRAINT source_material_unit_snapshots_owner_id_source_material_id_fkey FOREIGN KEY (owner_id, source_material_id) REFERENCES public.source_materials(owner_id, id) ON DELETE CASCADE;


--
-- Name: source_material_units source_material_units_snapshot_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.source_material_units
    ADD CONSTRAINT source_material_units_snapshot_fkey FOREIGN KEY (owner_id, source_material_id, snapshot_id) REFERENCES public.source_material_unit_snapshots(owner_id, source_material_id, snapshot_id) ON DELETE CASCADE;


--
-- Name: source_materials source_materials_book_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.source_materials
    ADD CONSTRAINT source_materials_book_fkey FOREIGN KEY (owner_id, book_id) REFERENCES public.books(owner_id, id) ON DELETE SET NULL;


--
-- Name: source_materials source_materials_current_content_revision_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.source_materials
    ADD CONSTRAINT source_materials_current_content_revision_fkey FOREIGN KEY (owner_id, current_content_revision_id) REFERENCES public.source_content_revisions(owner_id, revision_id) DEFERRABLE INITIALLY DEFERRED;


--
-- Name: source_materials source_materials_current_snapshot_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.source_materials
    ADD CONSTRAINT source_materials_current_snapshot_fkey FOREIGN KEY (owner_id, id, current_snapshot_id) REFERENCES public.source_material_unit_snapshots(owner_id, source_material_id, snapshot_id) DEFERRABLE INITIALLY DEFERRED;


--
-- Name: source_materials source_materials_owner_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.source_materials
    ADD CONSTRAINT source_materials_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: vocabulary_states vocabulary_states_owner_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.vocabulary_states
    ADD CONSTRAINT vocabulary_states_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- PostgreSQL database dump complete
--
