-- Development convenience only (ADR 0038, ADR 0070).
ALTER TABLE public.corpus_tokens
    DROP CONSTRAINT corpus_tokens_head_unprinted_node_fkey,
    DROP CONSTRAINT corpus_tokens_head_unprinted_node_self_check,
    DROP COLUMN head_unprinted_node;
DROP TABLE public.corpus_unprinted_syntax_nodes;
