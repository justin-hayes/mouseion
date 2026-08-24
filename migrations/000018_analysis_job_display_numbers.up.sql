ALTER TABLE analysis_jobs ADD COLUMN display_number bigint;

WITH numbered AS (
    SELECT river_job_id,
           row_number() OVER (PARTITION BY owner_id ORDER BY created_at, river_job_id) AS display_number
    FROM analysis_jobs
)
UPDATE analysis_jobs
SET display_number = numbered.display_number
FROM numbered
WHERE analysis_jobs.river_job_id = numbered.river_job_id;

ALTER TABLE analysis_jobs ALTER COLUMN display_number SET NOT NULL;
ALTER TABLE analysis_jobs ADD CONSTRAINT analysis_jobs_owner_display_number_key UNIQUE (owner_id, display_number);
