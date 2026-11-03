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

type Store interface {
	Save(context.Context, []byte) (string, error)
	Get(context.Context, string) ([]byte, bool, error)
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
	return nil
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
