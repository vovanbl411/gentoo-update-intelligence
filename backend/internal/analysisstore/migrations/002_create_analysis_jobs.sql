CREATE TABLE IF NOT EXISTS analysis_jobs (
    id UUID PRIMARY KEY,
    request JSONB NOT NULL,
    status TEXT NOT NULL CHECK (status IN ('pending', 'running', 'completed', 'failed')),
    stage TEXT NOT NULL DEFAULT 'queued',
    completed INTEGER NOT NULL DEFAULT 0 CHECK (completed >= 0),
    total INTEGER NOT NULL DEFAULT 0 CHECK (total >= 0),
    result JSONB,
    error TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (completed <= total OR total = 0)
);

CREATE INDEX IF NOT EXISTS analysis_jobs_unfinished_idx
    ON analysis_jobs (created_at)
    WHERE status IN ('pending', 'running');
