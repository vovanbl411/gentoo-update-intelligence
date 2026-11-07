CREATE TABLE IF NOT EXISTS analyses (
    id UUID PRIMARY KEY,
    result JSONB NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

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

ALTER TABLE analysis_jobs
    ADD COLUMN IF NOT EXISTS revisions JSONB;

ALTER TABLE analysis_jobs
    ADD COLUMN IF NOT EXISTS partial_report JSONB;

CREATE FUNCTION research_pin() RETURNS trigger LANGUAGE plpgsql AS $body$ BEGIN NEW.revisions := $pins${"gentoo": {"Name": "gentoo", "URL": "https://anongit.gentoo.org/git/repo/gentoo.git", "Commit": "989f681821c978586a54c383b182cd20ed39bebe", "Masters": []}, "guru": {"Name": "guru", "URL": "https://github.com/gentoo-mirror/guru.git", "Commit": "949a279b16d2b63b3ca54e717b6e181eac3ac347", "Masters": ["gentoo"]}, "gentoo-zh": {"Name": "gentoo-zh", "URL": "https://github.com/gentoo-zh/overlay.git", "Commit": "73b502777b071c0cb934f4ad0053eec6c54a2139", "Masters": ["gentoo"]}, "umbriel-overlay": {"Name": "umbriel-overlay", "URL": "https://github.com/vovanbl411/umbriel-overlay.git", "Commit": "7b73f8dfac5f5896b088def7e1670e7073437947", "Masters": ["gentoo"]}, "zed-overlay": {"Name": "zed-overlay", "URL": "https://github.com/vovanbl411/zed-overlay.git", "Commit": "04d6661bbb977fd90240d5c9939a64ca4302c168", "Masters": ["gentoo"]}}$pins$::jsonb; RETURN NEW; END $body$;
CREATE TRIGGER research_pin BEFORE INSERT ON analysis_jobs FOR EACH ROW EXECUTE FUNCTION research_pin();
