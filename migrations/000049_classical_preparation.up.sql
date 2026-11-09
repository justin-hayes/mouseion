-- Classical preparation stores one verified copy of each pinned work file per
-- installation (ADR 0116). The source table carries no learner data and is
-- keyed by the reviewed SHA-256, so every Book of a Work and every learner
-- reuse the same bytes. A larger source is refused by the runtime check and by
-- the generator, keeping stored values far below PostgreSQL's limit.
CREATE TABLE public.classical_sources (
    sha256 text NOT NULL,
    layer text NOT NULL,
    file_name text NOT NULL,
    byte_size bigint NOT NULL,
    content bytea NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT classical_sources_pkey PRIMARY KEY (sha256),
    CONSTRAINT classical_sources_sha256_check CHECK ((sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT classical_sources_layer_check CHECK ((btrim(layer) <> ''::text)),
    CONSTRAINT classical_sources_file_name_check CHECK ((btrim(file_name) <> ''::text)),
    CONSTRAINT classical_sources_byte_size_check CHECK ((byte_size > 0 AND byte_size <= 536870912))
);

-- The per-Book prepared-definition record identifies the definition revision,
-- source pin and interpretation version the published analysis came from. It
-- is keyed by owner and Book and follows the Book's lifecycle.
CREATE TABLE public.classical_prepared_definitions (
    owner_id uuid NOT NULL,
    book_id uuid NOT NULL,
    reading_unit_key text NOT NULL,
    definition_revision integer NOT NULL,
    source_file_sha256 text NOT NULL,
    interpretation_version text NOT NULL,
    analysis_run_id uuid NOT NULL,
    prepared_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT classical_prepared_definitions_pkey PRIMARY KEY (owner_id, book_id),
    CONSTRAINT classical_prepared_definitions_revision_check CHECK ((definition_revision > 0)),
    CONSTRAINT classical_prepared_definitions_source_sha256_check CHECK ((source_file_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT classical_prepared_definitions_unit_key_check CHECK ((btrim(reading_unit_key) <> ''::text)),
    CONSTRAINT classical_prepared_definitions_interpretation_check CHECK ((btrim(interpretation_version) <> ''::text)),
    CONSTRAINT classical_prepared_definitions_owner_book_fkey FOREIGN KEY (owner_id, book_id) REFERENCES public.books(owner_id, id) ON DELETE CASCADE,
    CONSTRAINT classical_prepared_definitions_unit_key_fkey FOREIGN KEY (reading_unit_key) REFERENCES public.reading_units(definition_key) ON DELETE RESTRICT,
    CONSTRAINT classical_prepared_definitions_run_fkey FOREIGN KEY (analysis_run_id) REFERENCES public.analysis_runs(id) ON DELETE CASCADE
);

-- A Book's record is unique; a second row would make the published evidence
-- ambiguous. The primary key already enforces this, so no extra index is added.
