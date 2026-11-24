-- ADR 0122: an imported Ancient Greek run keeps the AGDT syntax its source file
-- carries. A printed token's relation and head already fit corpus_tokens
-- (dependency, head). Two things do not:
--
--   * AGDT ellipsis nodes have no printed token. They are stored here, per
--     sentence, with their relation and head. A node has no form, lemma,
--     morphology, vocabulary identity or Annotation occurrence.
--   * A token whose head is such a node cannot name it by ordinal, so it is
--     self-headed (head = token_ordinal, the existing root convention) and
--     head_unprinted_node holds the node's source id. A reader checks
--     head_unprinted_node first.
--
-- Structural DDL only, with no backfill: existing runs keep their rows
-- unchanged and get no nodes. Forward-only in production (ADR 0038, ADR 0070).
CREATE TABLE public.corpus_unprinted_syntax_nodes (
    owner_id uuid NOT NULL,
    analysis_run_id uuid NOT NULL,
    corpus_id uuid NOT NULL,
    sentence_ordinal bigint NOT NULL,
    -- The OGA token id within the source sentence.
    source_id text NOT NULL,
    dependency text NOT NULL,
    -- The printed token the node depends on, or NULL.
    head_token_ordinal bigint,
    -- The unprinted node the node depends on, or NULL. Both heads NULL is a root.
    head_unprinted_node text,
    CONSTRAINT corpus_unprinted_syntax_nodes_pkey PRIMARY KEY (analysis_run_id, sentence_ordinal, source_id),
    CONSTRAINT corpus_unprinted_syntax_nodes_source_id_check CHECK (source_id <> ''),
    CONSTRAINT corpus_unprinted_syntax_nodes_dependency_check CHECK (dependency <> ''),
    CONSTRAINT corpus_unprinted_syntax_nodes_head_token_check CHECK (head_token_ordinal IS NULL OR head_token_ordinal >= 0),
    CONSTRAINT corpus_unprinted_syntax_nodes_one_head_check CHECK (head_token_ordinal IS NULL OR head_unprinted_node IS NULL),
    -- Owner-scoped and cascading exactly like corpus_tokens.
    CONSTRAINT corpus_unprinted_syntax_nodes_sentence_fkey
        FOREIGN KEY (owner_id, corpus_id, analysis_run_id, sentence_ordinal)
        REFERENCES public.corpus_sentences(owner_id, corpus_id, analysis_run_id, sentence_ordinal) ON DELETE CASCADE,
    CONSTRAINT corpus_unprinted_syntax_nodes_head_node_fkey
        FOREIGN KEY (analysis_run_id, sentence_ordinal, head_unprinted_node)
        REFERENCES public.corpus_unprinted_syntax_nodes(analysis_run_id, sentence_ordinal, source_id)
);

ALTER TABLE public.corpus_tokens
    ADD COLUMN head_unprinted_node text,
    ADD CONSTRAINT corpus_tokens_head_unprinted_node_self_check
        CHECK (head_unprinted_node IS NULL OR head = token_ordinal),
    ADD CONSTRAINT corpus_tokens_head_unprinted_node_fkey
        FOREIGN KEY (analysis_run_id, sentence_ordinal, head_unprinted_node)
        REFERENCES public.corpus_unprinted_syntax_nodes(analysis_run_id, sentence_ordinal, source_id);
