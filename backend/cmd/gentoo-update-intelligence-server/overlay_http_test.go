package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vovanbl411/gentoo-update-intelligence/internal/analysishttp"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/analysisstore"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/metadataprovider"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/parser"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/repositorymanager"
)

type overlayTestStore struct {
	mu  sync.RWMutex
	job analysisstore.Job
}

func (*overlayTestStore) Save(_ context.Context, _ []byte) (string, error) {
	return "28b28f31-7e85-4e6a-a839-84658f32cc74", nil
}
func (*overlayTestStore) Get(_ context.Context, _ string) ([]byte, bool, error) {
	return nil, false, nil
}
func (s *overlayTestStore) CreateJob(_ context.Context, request []byte) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.job = analysisstore.Job{ID: "28b28f31-7e85-4e6a-a839-84658f32cc74", Request: request, Status: "pending", Stage: "queued"}
	return s.job.ID, nil
}
func (s *overlayTestStore) UpdateJob(_ context.Context, update analysisstore.JobUpdate) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.job.Status, s.job.Stage = update.Status, update.Stage
	s.job.Completed, s.job.Total, s.job.Error = update.Completed, update.Total, update.Error
	s.job.Result = update.Result
	if len(update.Revisions) > 0 {
		s.job.Revisions = update.Revisions
	}
	return nil
}
func (s *overlayTestStore) GetJob(_ context.Context, _ string) (analysisstore.Job, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job := s.job
	job.Result = append([]byte(nil), job.Result...)
	return job, job.ID != "", nil
}
func (*overlayTestStore) RequeueInterruptedJobs(_ context.Context) error {
	return nil
}
func (s *overlayTestStore) ClaimNextJob(_ context.Context) (analysisstore.Job, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.job.Status != "pending" {
		return analysisstore.Job{}, false, nil
	}
	s.job.Status, s.job.Stage = "running", "preparing"
	return s.job, true, nil
}

type overlayResponse struct {
	Result struct {
		Repositories map[string]repositorymanager.Revision
		Reports      []struct {
			BeforeLookup struct {
				Result *metadataprovider.LookupResult
				Error  *struct {
					Message string `json:"message"`
				} `json:"error"`
			} `json:"before_lookup"`
			AfterLookup struct {
				Result *metadataprovider.LookupResult
				Error  *struct {
					Message string `json:"message"`
				} `json:"error"`
			} `json:"after_lookup"`
		}
	} `json:"result"`
}

func postOverlayPlan(t *testing.T, handler http.Handler, plan string) overlayResponse {
	t.Helper()
	body, err := json.Marshal(map[string]string{"plan": plan})
	if err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodPost, "/api/v1/analyses", bytes.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusAccepted {
		t.Fatalf("POST = %d: %s", response.Code, response.Body.String())
	}
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		response = httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/analyses/28b28f31-7e85-4e6a-a839-84658f32cc74", nil))
		var job struct {
			Status string          `json:"status"`
			Result json.RawMessage `json:"result"`
			Error  string          `json:"error"`
		}
		if err := json.Unmarshal(response.Body.Bytes(), &job); err != nil {
			t.Fatal(err)
		}
		if job.Status == "failed" {
			t.Fatalf("analysis failed: %s", job.Error)
		}
		if job.Status == "completed" {
			var result overlayResponse
			if err := json.Unmarshal(append(append([]byte(`{"result":`), job.Result...), '}'), &result); err != nil {
				t.Fatal(err)
			}
			return result
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("analysis did not complete")
	return overlayResponse{}
}

func TestOverlayConfigurationAndRefreshThroughHTTP(t *testing.T) {
	root := t.TempDir()
	gentooPath := filepath.Join(root, "gentoo")
	gentooCommit := fixtureRepo(t, gentooPath, "EAPI=8\nSLOT=0\nIUSE=gentoo\n")
	overlayPath := filepath.Join(root, "zh")
	firstCommit := fixtureRepo(t, overlayPath, "EAPI=8\nSLOT=0\nIUSE=old\n")
	umbrielPath := filepath.Join(root, "umbriel")
	fixtureRepo(t, umbrielPath, "EAPI=8\nSLOT=0\nIUSE=umbriel\n")
	catalog := []repositorymanager.Repository{
		{Name: "gentoo-zh", URL: overlayPath, Masters: []string{"gentoo"}},
		{Name: "umbriel-overlay", URL: umbrielPath, Masters: []string{"gentoo"}},
		{Name: "duplicate", URL: overlayPath, Masters: []string{"gentoo"}},
		{Name: "duplicate", URL: umbrielPath, Masters: []string{"gentoo"}},
	}
	manager, err := repositorymanager.New(filepath.Join(root, "mirrors"), append(repositorymanager.KnownOverlays(), catalog...))
	if err != nil {
		t.Fatal(err)
	}
	config := metadataprovider.Config{Repository: "gentoo", SourceURL: "https://example.invalid/gentoo", LocalPath: gentooPath, Commit: gentooCommit}
	factory := newProviderFactory(config, manager, workerSettings{})
	handler := analysishttp.NewHandlerWithPlanProviderFactory(&overlayTestStore{}, factory)
	plan := "[ebuild U] dev-libs/widget-1::gentoo-zh [1::gentoo-zh]"
	first := postOverlayPlan(t, handler, plan)
	if first.Result.Repositories["gentoo-zh"].Commit != firstCommit {
		t.Fatalf("first revision = %#v", first.Result.Repositories)
	}
	pinned, err := factory(context.Background(), parser.Parse(plan))
	if err != nil {
		t.Fatal(err)
	}

	if err := os.WriteFile(filepath.Join(overlayPath, "metadata/md5-cache/dev-libs/widget-2"), []byte("EAPI=8\nSLOT=0\nIUSE=new\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	fixtureGit(t, overlayPath, "add", ".")
	fixtureGit(t, overlayPath, "commit", "-qm", "new version")
	secondCommit := fixtureGit(t, overlayPath, "rev-parse", "HEAD")
	second := postOverlayPlan(t, handler, "[ebuild U] dev-libs/widget-2::gentoo-zh [1::gentoo-zh]")
	if second.Result.Repositories["gentoo-zh"].Commit != secondCommit || second.Result.Reports[0].AfterLookup.Result == nil || second.Result.Reports[0].AfterLookup.Result.Status != metadataprovider.StatusFound {
		t.Fatalf("updated result = %#v", second.Result)
	}
	persistedPins, err := json.Marshal(first.Result.Repositories)
	if err != nil {
		t.Fatal(err)
	}
	var restoredPins map[string]repositorymanager.Revision
	if err := json.Unmarshal(persistedPins, &restoredPins); err != nil {
		t.Fatal(err)
	}
	resumeFactory := newResumableProviderFactory(config, manager, workerSettings{})
	restoredProvider, err := resumeFactory(context.Background(), parser.Parse(plan), restoredPins)
	if err != nil {
		t.Fatalf("restore provider with persisted revisions: %v", err)
	}
	restoredRevision := restoredProvider.(interface {
		RepositoryRevisions() map[string]repositorymanager.Revision
	}).RepositoryRevisions()["gentoo-zh"]
	if restoredRevision.Commit != firstCommit {
		t.Fatalf("restored overlay commit = %s, want original %s", restoredRevision.Commit, firstCommit)
	}
	old, err := pinned.Lookup(metadataprovider.Query{Repository: "gentoo-zh", Category: "dev-libs", Name: "widget", Version: "1"})
	if err != nil || old.Snapshot == nil || old.Snapshot.Source.Commit != firstCommit {
		t.Fatalf("old lookup = %#v, %v", old, err)
	}

	if err := os.Rename(overlayPath, overlayPath+"-offline"); err != nil {
		t.Fatal(err)
	}
	fallback := postOverlayPlan(t, handler, "[ebuild U] dev-libs/widget-2::gentoo-zh [1::gentoo-zh]")
	if fallback.Result.Repositories["gentoo-zh"].Commit != secondCommit || fallback.Result.Reports[0].AfterLookup.Result == nil || fallback.Result.Reports[0].AfterLookup.Result.Status != metadataprovider.StatusFound {
		t.Fatalf("fallback = %#v", fallback.Result)
	}

	umbriel := postOverlayPlan(t, handler, "[ebuild U] dev-libs/widget-1::umbriel-overlay [1::umbriel-overlay]")
	if umbriel.Result.Repositories["umbriel-overlay"].Masters[0] != "gentoo" {
		t.Fatalf("umbriel = %#v", umbriel.Result.Repositories)
	}
	for _, tc := range []struct{ name, reason string }{{"unknown", "unknown repository"}, {"duplicate", "ambiguous repository"}} {
		result := postOverlayPlan(t, handler, "[ebuild U] dev-libs/widget-1::"+tc.name+" [1::umbriel-overlay]")
		lookup := result.Result.Reports[0].AfterLookup
		if lookup.Error == nil || !strings.Contains(lookup.Error.Message, tc.reason) {
			t.Fatalf("%s reason = %#v", tc.name, lookup)
		}
	}
	missingInstalled := postOverlayPlan(t, handler, "[ebuild U] dev-libs/widget-1::unknown")
	if got := missingInstalled.Result.Reports[0].AfterLookup.Error; got == nil || !strings.Contains(got.Message, "unknown repository") {
		t.Fatalf("unknown target with missing installed version = %v", got)
	}
	unknownBefore := postOverlayPlan(t, handler, "[ebuild U] dev-libs/widget-1::umbriel-overlay [1::unknown]")
	if got := unknownBefore.Result.Reports[0].BeforeLookup.Error; got == nil || !strings.Contains(got.Message, "unknown repository") {
		t.Fatalf("unknown installed repository = %v", got)
	}
}
