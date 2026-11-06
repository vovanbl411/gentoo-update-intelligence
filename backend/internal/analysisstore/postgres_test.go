package analysisstore

import (
	"context"
	"database/sql"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
)

// Uses a private schema, never application tables. The caller supplies an
// isolated test PostgreSQL; ordinary unit runs skip this integration check.
func TestPostgresQueue(t *testing.T) {
	dsn := os.Getenv("ANALYSISSTORE_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("set ANALYSISSTORE_TEST_DATABASE_URL to run PostgreSQL integration checks")
	}
	ctx := context.Background()
	admin, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { admin.Close() })
	id, err := newUUID()
	if err != nil {
		t.Fatal(err)
	}
	schema := "queue_test_" + strings.ReplaceAll(id, "-", "")
	if _, err := admin.ExecContext(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := admin.ExecContext(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	query := u.Query()
	query.Set("search_path", schema)
	u.RawQuery = query.Encode()
	store, err := Open(ctx, u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	for i := 0; i < 2; i++ {
		if err := store.Migrate(ctx); err != nil {
			t.Fatal(err)
		}
	}
	var ids []string
	for i := 0; i < 3; i++ {
		id, err := store.CreateJob(ctx, []byte(`{"plan":"test"}`))
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	first, found, err := store.ClaimNextJob(ctx)
	if err != nil || !found || first.ID != ids[0] || first.Status != "running" || first.Stage != "preparing" {
		t.Fatalf("first claim: %#v, %t, %v", first, found, err)
	}
	pins := []byte(`{"gentoo":{"Commit":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}`)
	if err := store.UpdateJob(ctx, JobUpdate{ID: first.ID, Status: "running", Stage: "analyzing", Completed: 1, Total: 2, Revisions: pins, PartialReport: []byte(`{"ready":true}`)}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpdateJob(ctx, JobUpdate{ID: first.ID, Status: "running", Stage: "analyzing", Completed: 2, Total: 2}); err != nil { t.Fatal(err) }
	if err := store.UpdateJob(ctx, JobUpdate{ID: ids[2], Status: "completed", Stage: "completed", Result: []byte(`{"saved":true}`)}); err != nil {
		t.Fatal(err)
	}
	if err := store.RequeueInterruptedJobs(ctx); err != nil {
		t.Fatal(err)
	}
	restored, _, err := store.GetJob(ctx, first.ID)
	if err != nil || restored.Status != "pending" || restored.Stage != "queued" || restored.Completed != 0 || restored.Total != 0 || !strings.Contains(string(restored.Revisions), "aaaaaaaa") || !strings.Contains(string(restored.PartialReport), `"ready": true`) {
		t.Fatalf("requeued job: %#v, %v", restored, err)
	}
	terminal, _, err := store.GetJob(ctx, ids[2])
	if err != nil || terminal.Status != "completed" || len(terminal.Result) == 0 {
		t.Fatalf("terminal job changed: %#v, %v", terminal, err)
	}
	// Concurrent claims must never return the same job, and terminal jobs
	// must not be picked. Two pending rows remain after recovery.
	claims := make(chan string, 8)
	var workers sync.WaitGroup
	for i := 0; i < 8; i++ {
		workers.Add(1)
		go func() {
			defer workers.Done()
			job, found, err := store.ClaimNextJob(ctx)
			if err != nil {
				t.Error(err)
				return
			}
			if found {
				claims <- job.ID
			}
		}()
	}
	workers.Wait()
	close(claims)
	seen := map[string]bool{}
	for id := range claims {
		if seen[id] || id == ids[2] {
			t.Fatalf("invalid duplicate/terminal claim: %s", id)
		}
		seen[id] = true
	}
	if len(seen) != 2 || !seen[ids[0]] || !seen[ids[1]] {
		t.Fatalf("claims: %v", seen)
	}
}
