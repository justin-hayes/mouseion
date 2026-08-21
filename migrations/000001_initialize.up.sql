CREATE TABLE users (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), username text NOT NULL UNIQUE,
 is_admin boolean NOT NULL DEFAULT false, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE language_profiles (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), owner_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 language text NOT NULL, display_name text NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(owner_id, language)
);
CREATE TABLE source_materials (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), owner_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 language text NOT NULL, source_identifier text NOT NULL, title text NOT NULL, media_type text NOT NULL,
 content_hash text NOT NULL, content bytea NOT NULL, full_text text NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(owner_id, source_identifier), UNIQUE(owner_id, content_hash), UNIQUE(owner_id, id)
);
-- Shared artifacts deliberately contain no source text, sentence text, or owner.
CREATE TABLE normalized_corpus_artifacts (
 content_hash text PRIMARY KEY, language text NOT NULL, schema_version text NOT NULL,
 normalization_profile text NOT NULL, normalization_version text NOT NULL,
 analyzer_name text NOT NULL, analyzer_version text NOT NULL, created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE shared_lemmas (
 id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
 content_hash text NOT NULL REFERENCES normalized_corpus_artifacts(content_hash) ON DELETE CASCADE,
 language text NOT NULL, canonical_lemma text NOT NULL, upos text NOT NULL,
 morphology jsonb NOT NULL DEFAULT '{}'::jsonb, frequency bigint NOT NULL CHECK(frequency >= 0),
 UNIQUE(content_hash, canonical_lemma, upos, morphology)
);
CREATE TABLE corpora (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), owner_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 source_material_id uuid NOT NULL, artifact_hash text NOT NULL REFERENCES normalized_corpus_artifacts(content_hash),
 status text NOT NULL DEFAULT 'pending', created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(owner_id, source_material_id), UNIQUE(owner_id, id),
 FOREIGN KEY(owner_id, source_material_id) REFERENCES source_materials(owner_id, id) ON DELETE CASCADE
);
CREATE TABLE example_sentences (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), owner_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 corpus_id uuid NOT NULL, sentence_key text NOT NULL, sentence_text text NOT NULL,
 source_location jsonb NOT NULL DEFAULT '{}'::jsonb, created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(owner_id, corpus_id, sentence_key), UNIQUE(owner_id, id),
 FOREIGN KEY(owner_id, corpus_id) REFERENCES corpora(owner_id, id) ON DELETE CASCADE
);
CREATE TABLE known_vocabulary (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), owner_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 language text NOT NULL, canonical_lemma text NOT NULL, upos text NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(owner_id, language, canonical_lemma, upos)
);
CREATE TABLE vocabulary_states (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), owner_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 language text NOT NULL, canonical_lemma text NOT NULL, upos text NOT NULL,
 state text NOT NULL CHECK(state IN ('candidate','accepted','generated','ignored','known')),
 updated_at timestamptz NOT NULL DEFAULT now(), UNIQUE(owner_id, language, canonical_lemma, upos)
);
CREATE TABLE curated_sentences (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), owner_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 example_sentence_id uuid NOT NULL, language text NOT NULL, canonical_lemma text NOT NULL, upos text NOT NULL,
 notes text NOT NULL DEFAULT '', created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(owner_id, language, canonical_lemma, upos),
 FOREIGN KEY(owner_id, example_sentence_id) REFERENCES example_sentences(owner_id, id) ON DELETE CASCADE
);
CREATE TABLE decks (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), owner_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 language text NOT NULL, name text NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(owner_id, language, name), UNIQUE(owner_id, id)
);
CREATE TABLE cards (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), owner_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 deck_id uuid NOT NULL, dedup_key text NOT NULL, canonical_lemma text NOT NULL, upos text NOT NULL,
 front text NOT NULL, back text NOT NULL, created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(owner_id, dedup_key),
 FOREIGN KEY(owner_id, deck_id) REFERENCES decks(owner_id, id) ON DELETE CASCADE
);
CREATE TABLE processing_history (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), owner_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 corpus_id uuid, operation text NOT NULL, status text NOT NULL, details jsonb NOT NULL DEFAULT '{}'::jsonb,
 started_at timestamptz NOT NULL DEFAULT now(), completed_at timestamptz,
 FOREIGN KEY(owner_id, corpus_id) REFERENCES corpora(owner_id, id) ON DELETE CASCADE
);
-- Global reference data has no owner. Population is a separate admin seed/import concern.
CREATE TABLE frequency_datasets (
 id uuid PRIMARY KEY DEFAULT gen_random_uuid(), language text NOT NULL, name text NOT NULL,
 version text NOT NULL, created_at timestamptz NOT NULL DEFAULT now(), UNIQUE(language, name, version)
);
CREATE TABLE frequency_entries (
 dataset_id uuid NOT NULL REFERENCES frequency_datasets(id) ON DELETE CASCADE,
 language text NOT NULL, canonical_lemma text NOT NULL, upos text NOT NULL,
 rank bigint NOT NULL CHECK(rank > 0), frequency double precision NOT NULL CHECK(frequency >= 0),
 PRIMARY KEY(dataset_id, canonical_lemma, upos), UNIQUE(dataset_id, rank)
);
CREATE INDEX shared_lemmas_identity_idx ON shared_lemmas(language, canonical_lemma, upos);
CREATE INDEX processing_history_owner_started_idx ON processing_history(owner_id, started_at DESC);
