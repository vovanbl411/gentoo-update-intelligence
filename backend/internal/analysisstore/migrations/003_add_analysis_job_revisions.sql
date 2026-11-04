ALTER TABLE analysis_jobs
    ADD COLUMN IF NOT EXISTS revisions JSONB;
