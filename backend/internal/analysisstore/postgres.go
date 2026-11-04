// Package analysisstore persists serialized analysis results in PostgreSQL.
package analysisstore

import (
	"context"
	"crypto/rand"
	"database/sql"
	_ "embed"
	"encoding/hex"
	"fmt"
	"strings"

	_ "github.com/jackc/pgx/v5/stdlib"
)

//go:embed migrations/001_create_analyses.sql
var createAnalysesTable string

//go:embed migrations/002_create_analysis_jobs.sql
var createAnalysisJobsTable string

//go:embed migrations/003_add_analysis_job_revisions.sql
var addAnalysisJobRevisions string

type Store interface {
	Save(context.Context, []byte) (string, error)
	Get(context.Context, string) ([]byte, bool, error)
	CreateJob(context.Context, []byte) (string, error)
	UpdateJob(context.Context, JobUpdate) error
	GetJob(context.Context, string) (Job, bool, error)
	UnfinishedJobs(context.Context) ([]Job, error)
}

type Job struct {
	ID        string
	Request   []byte
	Status    string
	Stage     string
	Completed int
	Total     int
	Revisions []byte
	Result    []byte
	Error     string
}

type JobUpdate struct {
	ID        string
	Status    string
	Stage     string
	Completed int
	Total     int
	Revisions []byte
	Result    []byte
	Error     string
}

type Postgres struct {
	db *sql.DB
}

func Open(ctx context.Context, dsn string) (*Postgres, error) {
	if strings.TrimSpace(dsn) == "" {
		return nil, fmt.Errorf("database URL is required")
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		return nil, fmt.Errorf("open PostgreSQL connection: %w", err)
	}
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		return nil, fmt.Errorf("connect to PostgreSQL: %w", err)
	}
	return &Postgres{db: db}, nil
}

func (p *Postgres) Close() error {
	return p.db.Close()
}

func (p *Postgres) Migrate(ctx context.Context) error {
	if _, err := p.db.ExecContext(ctx, createAnalysesTable); err != nil {
		return fmt.Errorf("create analyses table: %w", err)
	}
	if _, err := p.db.ExecContext(ctx, createAnalysisJobsTable); err != nil {
		return fmt.Errorf("create analysis jobs table: %w", err)
	}
	if _, err := p.db.ExecContext(ctx, addAnalysisJobRevisions); err != nil {
		return fmt.Errorf("add analysis job revisions: %w", err)
	}
	return nil
}

func (p *Postgres) CreateJob(ctx context.Context, request []byte) (string, error) {
	id, err := newUUID()
	if err != nil {
		return "", fmt.Errorf("generate analysis job ID: %w", err)
	}
	var insertedID string
	err = p.db.QueryRowContext(ctx,
		`INSERT INTO analysis_jobs (id, request) VALUES ($1::uuid, $2::jsonb) RETURNING id::text`,
		id, string(request),
	).Scan(&insertedID)
	if err != nil {
		return "", fmt.Errorf("insert analysis job: %w", err)
	}
	return insertedID, nil
}

func (p *Postgres) UpdateJob(ctx context.Context, update JobUpdate) error {
	result, err := p.db.ExecContext(ctx, `
		UPDATE analysis_jobs
		SET status = $2, stage = $3, completed = $4, total = $5,
		    result = NULLIF($6, '')::jsonb, error = NULLIF($7, ''),
		    revisions = COALESCE(NULLIF($8, '')::jsonb, revisions), updated_at = now()
		WHERE id = $1::uuid`, update.ID, update.Status, update.Stage, update.Completed, update.Total, string(update.Result), update.Error, string(update.Revisions))
	if err != nil {
		return fmt.Errorf("update analysis job: %w", err)
	}
	count, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("read analysis job update result: %w", err)
	}
	if count == 0 {
		return fmt.Errorf("update analysis job: job %s does not exist", update.ID)
	}
	return nil
}

func (p *Postgres) GetJob(ctx context.Context, id string) (Job, bool, error) {
	var job Job
	var request, result []byte
	err := p.db.QueryRowContext(ctx, `
		SELECT id::text, request::text, status, stage, completed, total, COALESCE(revisions::text, ''),
		       COALESCE(result::text, ''), COALESCE(error, '')
		FROM analysis_jobs WHERE id = $1::uuid`, id).
		Scan(&job.ID, &request, &job.Status, &job.Stage, &job.Completed, &job.Total, &job.Revisions, &result, &job.Error)
	if err == sql.ErrNoRows {
		return Job{}, false, nil
	}
	if err != nil {
		return Job{}, false, fmt.Errorf("read analysis job: %w", err)
	}
	job.Request, job.Result = request, result
	return job, true, nil
}

func (p *Postgres) UnfinishedJobs(ctx context.Context) ([]Job, error) {
	rows, err := p.db.QueryContext(ctx, `
		SELECT id::text, request::text, status, stage, completed, total, COALESCE(revisions::text, ''),
		       COALESCE(result::text, ''), COALESCE(error, '')
	FROM analysis_jobs WHERE status IN ('pending', 'running') ORDER BY created_at`)
	if err != nil {
		return nil, fmt.Errorf("list unfinished analysis jobs: %w", err)
	}
	defer rows.Close()
	var jobs []Job
	for rows.Next() {
		var job Job
		var request, result []byte
		if err := rows.Scan(&job.ID, &request, &job.Status, &job.Stage, &job.Completed, &job.Total, &job.Revisions, &result, &job.Error); err != nil {
			return nil, fmt.Errorf("scan unfinished analysis job: %w", err)
		}
		job.Request, job.Result = request, result
		jobs = append(jobs, job)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate unfinished analysis jobs: %w", err)
	}
	return jobs, nil
}

func (p *Postgres) Save(ctx context.Context, result []byte) (string, error) {
	id, err := newUUID()
	if err != nil {
		return "", fmt.Errorf("generate analysis ID: %w", err)
	}
	var insertedID string
	err = p.db.QueryRowContext(ctx,
		`INSERT INTO analyses (id, result) VALUES ($1::uuid, $2::jsonb) RETURNING id::text`,
		id, string(result),
	).Scan(&insertedID)
	if err != nil {
		return "", fmt.Errorf("insert analysis: %w", err)
	}
	return insertedID, nil
}

func (p *Postgres) Get(ctx context.Context, id string) ([]byte, bool, error) {
	var result []byte
	err := p.db.QueryRowContext(ctx,
		`SELECT result::text FROM analyses WHERE id = $1::uuid`, id,
	).Scan(&result)
	if err == sql.ErrNoRows {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("read analysis: %w", err)
	}
	return result, true, nil
}

func newUUID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = value[6]&0x0f | 0x40
	value[8] = value[8]&0x3f | 0x80
	encoded := hex.EncodeToString(value[:])
	return encoded[:8] + "-" + encoded[8:12] + "-" + encoded[12:16] + "-" + encoded[16:20] + "-" + encoded[20:], nil
}
