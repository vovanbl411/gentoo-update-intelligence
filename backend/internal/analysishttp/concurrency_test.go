package analysishttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/vovanbl411/gentoo-update-intelligence/internal/analysisstore"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/parser"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/reportservice"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/repositorymanager"
)

const queuePlan = `{"plan":"[ebuild U] dev-libs/widget-2::gentoo [1::gentoo]"}`

func awaitStart(t *testing.T, started <-chan struct{}) {
	t.Helper()
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("analysis did not start")
	}
}

func TestAnalysisConcurrencyLimits(t *testing.T) {
	for _, limit := range []int{1, 2, 0} {
		t.Run(fmt.Sprint(limit), func(t *testing.T) {
			store := &memoryStore{autoIDs: true}
			started := make(chan struct{}, 4)
			release := make(chan struct{}, 4)
			handler, err := NewHandlerWithConcurrencyLimit(store, func(context.Context, parser.Result, map[string]repositorymanager.Revision) (reportservice.MetadataLookup, error) {
				started <- struct{}{}
				<-release
				return &sequenceProvider{}, nil
			}, limit)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { close(release) })
			var ids []string
			for i := 0; i < 4; i++ {
				response := newPost(handler, queuePlan)
				if response.Code != http.StatusAccepted {
					t.Fatalf("POST: %s", response.Body.String())
				}
				var job jobResponse
				if err := json.Unmarshal(response.Body.Bytes(), &job); err != nil {
					t.Fatal(err)
				}
				ids = append(ids, job.ID)
			}
			active := limit
			if limit == 0 {
				active = 4
			}
			for i := 0; i < active; i++ {
				awaitStart(t, started)
			}
			if limit > 0 {
				// No waiting job may reach provider preparation while slots are occupied.
				select {
				case <-started:
					t.Fatal("analysis started above the limit")
				case <-time.After(50 * time.Millisecond):
				}
				for _, id := range ids[active:] {
					job, _, _ := store.GetJob(context.Background(), id)
					if job.Status != "pending" || job.Stage != "queued" || len(job.Revisions) != 0 {
						t.Fatalf("waiting job changed: %#v", job)
					}
				}
				// One released slot must admit exactly the next job.
				release <- struct{}{}
				awaitStart(t, started)
				job, _, _ := store.GetJob(context.Background(), ids[active])
				if job.Status != "running" {
					t.Fatalf("next job status = %s", job.Status)
				}
			}
			for i := 0; i < 4; i++ {
				release <- struct{}{}
			}
			for _, id := range ids {
				if got := waitJob(t, handler, id); got.Status != "completed" {
					t.Fatalf("job %s: %#v", id, got)
				}
			}
		})
	}
}

func TestResumeQueueKeepsPinsAndContinuesAfterInvalidJob(t *testing.T) {
	ids := []string{
		"00000000-0000-4000-8000-000000000001",
		"00000000-0000-4000-8000-000000000002",
		"00000000-0000-4000-8000-000000000003",
	}
	pins := []byte(`{"gentoo":{"Name":"gentoo","Commit":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}`)
	store := &memoryStore{jobs: map[string]analysisstore.Job{
		ids[0]: {ID: ids[0], Status: "pending", Request: []byte(`invalid`)},
		ids[1]: {ID: ids[1], Status: "running", Request: []byte(queuePlan), Revisions: pins},
		ids[2]: {ID: ids[2], Status: "pending", Request: []byte(queuePlan)},
	}}
	started := make(chan bool, 2)
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	handler := NewHandlerWithPinnedProviderFactory(store, func(_ context.Context, _ parser.Result, revisions map[string]repositorymanager.Revision) (reportservice.MetadataLookup, error) {
		started <- revisions["gentoo"].Commit == "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
		<-release
		return nil, errors.New("provider unavailable")
	})
	if err := handler.Resume(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := waitJob(t, handler, ids[0]); got.Status != "failed" {
		t.Fatalf("invalid job: %#v", got)
	}
	select {
	case pinned := <-started:
		if !pinned {
			t.Fatal("pinned revision was lost")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("interrupted job was not resumed")
	}
	job, _, _ := store.GetJob(context.Background(), ids[2])
	if job.Status != "pending" {
		t.Fatalf("waiting job status = %s", job.Status)
	}
	// Release the resumed job; its failure must free the slot for the next job.
	release <- struct{}{}
	select {
	case <-started:
	case <-time.After(2 * time.Second):
		t.Fatal("provider failure did not free the slot")
	}
	release <- struct{}{}
	for _, id := range ids[1:] {
		if got := waitJob(t, handler, id); got.Status != "failed" {
			t.Fatalf("failed provider job: %#v", got)
		}
	}
}

type temporarilyUnavailableQueue struct {
	*memoryStore
	mu     sync.Mutex
	failed bool
}

func (s *temporarilyUnavailableQueue) ClaimNextJob(ctx context.Context) (analysisstore.Job, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.failed {
		s.failed = true
		return analysisstore.Job{}, false, errors.New("temporary database error")
	}
	return s.memoryStore.ClaimNextJob(ctx)
}

func TestQueueRetriesWithoutAnotherPost(t *testing.T) {
	store := &temporarilyUnavailableQueue{memoryStore: &memoryStore{id: testID}}
	handler := NewHandler(store, &sequenceProvider{})
	if got := newPost(handler, queuePlan); got.Code != http.StatusAccepted {
		t.Fatal(got.Body.String())
	}
	if got := waitJob(t, handler, testID); got.Status != "completed" {
		t.Fatalf("job: %#v", got)
	}
}

func TestNegativeConcurrencyLimitRejected(t *testing.T) {
	if _, err := NewHandlerWithConcurrencyLimit(nil, nil, -1); err == nil {
		t.Fatal("negative limit accepted")
	}
}

type emptyProbeQueue struct {
	*memoryStore
	once    sync.Once
	probed  chan struct{}
	release chan struct{}
}

func (s *emptyProbeQueue) ClaimNextJob(ctx context.Context) (analysisstore.Job, bool, error) {
	job, found, err := s.memoryStore.ClaimNextJob(ctx)
	s.once.Do(func() {
		close(s.probed)
		<-s.release
	})
	return job, found, err
}

func TestPostDuringEmptyQueueProbeIsNotLost(t *testing.T) {
	store := &emptyProbeQueue{
		memoryStore: &memoryStore{id: testID},
		probed:      make(chan struct{}), release: make(chan struct{}),
	}
	handler := NewHandler(store, &sequenceProvider{})
	handler.schedule()
	select {
	case <-store.probed:
	case <-time.After(2 * time.Second):
		t.Fatal("queue was not probed")
	}
	// The dispatcher has observed no rows but has not yet gone idle. A POST
	// here must wake it again rather than leave the saved job waiting forever.
	response := newPost(handler, queuePlan)
	close(store.release)
	if response.Code != http.StatusAccepted {
		t.Fatal(response.Body.String())
	}
	if got := waitJob(t, handler, testID); got.Status != "completed" {
		t.Fatalf("job: %#v", got)
	}
}
