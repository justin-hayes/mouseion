CREATE TABLE analysis_jobs (
 river_job_id bigint PRIMARY KEY,
 owner_id uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
 source_material_id uuid NOT NULL,
 content_hash text NOT NULL,
 corpus_id uuid,
 progress smallint NOT NULL DEFAULT 0 CHECK (progress BETWEEN 0 AND 100),
 error text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(owner_id, content_hash),
 FOREIGN KEY(owner_id, source_material_id) REFERENCES source_materials(owner_id, id) ON DELETE CASCADE,
 FOREIGN KEY(owner_id, corpus_id) REFERENCES corpora(owner_id, id) ON DELETE SET NULL
);

CREATE INDEX analysis_jobs_owner_created_idx ON analysis_jobs(owner_id, created_at DESC);
