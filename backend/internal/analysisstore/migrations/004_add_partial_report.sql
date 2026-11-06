ALTER TABLE analysis_jobs
    ADD COLUMN IF NOT EXISTS partial_report JSONB;
