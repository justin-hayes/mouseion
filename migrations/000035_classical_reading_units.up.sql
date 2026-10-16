-- Classical reading units registered by an operator: a Work, its Edition, one
-- Reading unit of that Edition, and the association of a Book to a Reading
-- unit. Work, Edition and Reading unit describe a text; they carry no learner
-- state. The definition values are compiled, reviewed application data.
--
-- The Book-Reading unit association is immutable: a Book belongs to exactly one
-- Reading unit, and an owner has at most one Book per Reading unit. A Book
-- registered this way also carries new metadata provenance (see domain).

CREATE TABLE public.works (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    definition_key text NOT NULL,
    title text NOT NULL,
    author text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT works_definition_key_check CHECK ((btrim(definition_key) <> ''::text)),
    CONSTRAINT works_title_check CHECK ((btrim(title) <> ''::text)),
    CONSTRAINT works_author_check CHECK ((btrim(author) <> ''::text))
);

CREATE TABLE public.editions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    work_id uuid NOT NULL,
    definition_key text NOT NULL,
    label text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT editions_definition_key_check CHECK ((btrim(definition_key) <> ''::text)),
    CONSTRAINT editions_label_check CHECK ((btrim(label) <> ''::text))
);

CREATE TABLE public.reading_units (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    edition_id uuid NOT NULL,
    definition_key text NOT NULL,
    citation_start text NOT NULL,
    citation_end text NOT NULL,
    source_file_name text NOT NULL,
    source_file_size bigint NOT NULL,
    source_file_sha256 text NOT NULL,
    interpretation_version text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT reading_units_citation_end_check CHECK ((btrim(citation_end) <> ''::text)),
    CONSTRAINT reading_units_citation_start_check CHECK ((btrim(citation_start) <> ''::text)),
    CONSTRAINT reading_units_definition_key_check CHECK ((btrim(definition_key) <> ''::text)),
    CONSTRAINT reading_units_interpretation_version_check CHECK ((btrim(interpretation_version) <> ''::text)),
    CONSTRAINT reading_units_source_file_name_check CHECK ((btrim(source_file_name) <> ''::text)),
    CONSTRAINT reading_units_source_file_sha256_check CHECK ((source_file_sha256 ~ '^[0-9a-f]{64}$'::text)),
    CONSTRAINT reading_units_source_file_size_check CHECK ((source_file_size > 0))
);

CREATE TABLE public.book_reading_units (
    owner_id uuid NOT NULL,
    book_id uuid NOT NULL,
    reading_unit_id uuid NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL
);

ALTER TABLE ONLY public.works
    ADD CONSTRAINT works_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.works
    ADD CONSTRAINT works_definition_key_key UNIQUE (definition_key);

ALTER TABLE ONLY public.editions
    ADD CONSTRAINT editions_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.editions
    ADD CONSTRAINT editions_definition_key_key UNIQUE (definition_key);

ALTER TABLE ONLY public.reading_units
    ADD CONSTRAINT reading_units_pkey PRIMARY KEY (id);

ALTER TABLE ONLY public.reading_units
    ADD CONSTRAINT reading_units_definition_key_key UNIQUE (definition_key);

ALTER TABLE ONLY public.book_reading_units
    ADD CONSTRAINT book_reading_units_pkey PRIMARY KEY (book_id);

-- At most one Book per owner and Reading unit.
ALTER TABLE ONLY public.book_reading_units
    ADD CONSTRAINT book_reading_units_owner_id_reading_unit_id_key UNIQUE (owner_id, reading_unit_id);

ALTER TABLE ONLY public.editions
    ADD CONSTRAINT editions_work_id_fkey FOREIGN KEY (work_id) REFERENCES public.works(id) ON DELETE RESTRICT;

ALTER TABLE ONLY public.reading_units
    ADD CONSTRAINT reading_units_edition_id_fkey FOREIGN KEY (edition_id) REFERENCES public.editions(id) ON DELETE RESTRICT;

ALTER TABLE ONLY public.book_reading_units
    ADD CONSTRAINT book_reading_units_owner_id_book_id_fkey FOREIGN KEY (owner_id, book_id) REFERENCES public.books(owner_id, id) ON DELETE CASCADE;

ALTER TABLE ONLY public.book_reading_units
    ADD CONSTRAINT book_reading_units_reading_unit_id_fkey FOREIGN KEY (reading_unit_id) REFERENCES public.reading_units(id) ON DELETE RESTRICT;

-- The association is immutable: a Book is never re-pointed to another Reading
-- unit. Deleting a Book cascades its association away.
CREATE FUNCTION public.reject_book_reading_unit_mutation() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
 RAISE EXCEPTION 'book-Reading unit associations are immutable';
END;
$$;

CREATE TRIGGER book_reading_units_immutable BEFORE UPDATE ON public.book_reading_units FOR EACH ROW EXECUTE FUNCTION public.reject_book_reading_unit_mutation();

