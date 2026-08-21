ALTER TABLE selection_candidates
 ADD COLUMN ranking_global_pct double precision,
 ADD COLUMN ranking_corpus_pct double precision,
 ADD COLUMN ranking_priority boolean,
 ADD COLUMN ranking_cross_text integer,
 ADD COLUMN ranking_score double precision,
 ADD COLUMN ranked_at timestamptz;

ALTER TABLE selection_candidates
 ADD CONSTRAINT selection_candidates_ranking_global_pct_check CHECK (ranking_global_pct BETWEEN 0 AND 1),
 ADD CONSTRAINT selection_candidates_ranking_corpus_pct_check CHECK (ranking_corpus_pct BETWEEN 0 AND 1),
 ADD CONSTRAINT selection_candidates_ranking_cross_text_check CHECK (ranking_cross_text >= 1);
