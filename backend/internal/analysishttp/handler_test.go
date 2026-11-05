package analysishttp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/vovanbl411/gentoo-update-intelligence/internal/analysisstore"

	"github.com/vovanbl411/gentoo-update-intelligence/internal/metadatadiff"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/metadataprovider"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/parser"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/reportservice"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/repositorymanager"
)

const testID = "28b28f31-7e85-4e6a-a839-84658f32cc74"

func TestProviderFactoryOncePerAnalysisAndRecovers(t *testing.T) {
	store := &memoryStore{id: testID}
	calls := 0
	provider := &sequenceProvider{}
	handler := NewHandlerWithProviderFactory(store, func() (reportservice.MetadataLookup, error) {
		calls++
		if calls == 1 {
			return nil, errors.New("not published")
		}
		return provider, nil
	})
	post := newPost(handler, `{"plan":"[ebuild U] dev-libs/widget-2::gentoo [1::gentoo]"}`)
	if post.Code != http.StatusAccepted {
		t.Fatalf("POST status = %d: %s", post.Code, post.Body.String())
	}
	got := waitJob(t, handler, testID)
	if got.Status != "failed" || (got.Error == nil || got.Error.Message != "metadata provider is unavailable") {
		t.Fatalf("first job = %#v, want provider failure", got)
	}
	secondStore := &memoryStore{id: "18b28f31-7e85-4e6a-a839-84658f32cc74"}
	second := NewHandlerWithProviderFactory(secondStore, func() (reportservice.MetadataLookup, error) { return provider, nil })
	secondPost := newPost(second, `{"plan":"[ebuild U] dev-libs/widget-2::gentoo [1::gentoo]"}`)
	if secondPost.Code != http.StatusAccepted {
		t.Fatalf("second POST status = %d: %s", secondPost.Code, secondPost.Body.String())
	}
	if got := waitJob(t, second, secondStore.id); got.Status != "completed" {
		t.Fatalf("second job = %#v, want completed", got)
	}
	if calls != 1 || provider.calls != 2 || store.createCalls != 1 || secondStore.createCalls != 1 {
		t.Fatalf("factory calls %d, lookup calls %d, first creates %d, second creates %d", calls, provider.calls, store.createCalls, secondStore.createCalls)
	}
}

func TestPostPersistsFullResultAndGetReturnsIt(t *testing.T) {
	store := &memoryStore{id: testID}
	provider := &sequenceProvider{}
	handler := NewHandler(store, provider)
	body := `{"plan":"[ebuild U] dev-libs/widget-2::gentoo [1::gentoo]\n[ebuild U] dev-libs/gadget-2::gentoo [1::gentoo]"}`

	post := httptest.NewRequest(http.MethodPost, "/api/v1/analyses", strings.NewReader(body))
	post.Header.Set("Content-Type", "application/json")
	created := httptest.NewRecorder()
	handler.ServeHTTP(created, post)
	if created.Code != http.StatusAccepted {
		t.Fatalf("POST status = %d, body = %s", created.Code, created.Body.String())
	}
	var accepted jobResponse
	if err := json.Unmarshal(created.Body.Bytes(), &accepted); err != nil {
		t.Fatal(err)
	}
	if accepted.ID != testID || accepted.Status != "pending" {
		t.Fatalf("POST response = %#v, want pending job", accepted)
	}
	completed := waitJob(t, handler, testID)
	if completed.Status != "completed" || len(completed.Result) == 0 {
		t.Fatalf("GET job = %#v, want completed persisted result", completed)
	}
	var saved resultDTO
	if err := json.Unmarshal(completed.Result, &saved); err != nil {
		t.Fatal(err)
	}
	if len(saved.Reports) != 2 {
		t.Fatalf("saved report count = %d, want 2", len(saved.Reports))
	}
	if got := saved.Reports[0].BeforeLookup.Result.Status; got != "not_found" {
		t.Fatalf("before lookup status = %q, want not_found", got)
	}
	if got := saved.Reports[0].AfterLookup.Result.Status; got != "repository_unavailable" {
		t.Fatalf("after lookup status = %q, want repository_unavailable", got)
	}
	if got := saved.Reports[1].BeforeLookup.Error; got == nil || got.Message != "fixture lookup error" {
		t.Fatalf("saved Lookup error = %v, want fixture lookup error", got)
	}
	if saved.Reports[1].AfterLookup.Result == nil || saved.Reports[1].AfterLookup.Result.Status != "found" || saved.Reports[1].AfterLookup.Result.Snapshot == nil {
		t.Fatalf("second report did not preserve found Lookup result and snapshot: %#v", saved.Reports[1].AfterLookup)
	}

	get := httptest.NewRequest(http.MethodGet, "/api/v1/analyses/"+testID, nil)
	loaded := httptest.NewRecorder()
	handler.ServeHTTP(loaded, get)
	if loaded.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body = %s", loaded.Code, loaded.Body.String())
	}
	var loadedBody jobResponse
	if err := json.Unmarshal(loaded.Body.Bytes(), &loadedBody); err != nil {
		t.Fatal(err)
	}
	if loadedBody.ID != accepted.ID || !jsonEqual(loadedBody.Result, completed.Result) {
		t.Fatalf("GET result differs from stored result: GET=%s", loaded.Body.Bytes())
	}
}

func TestGetReportsProgressWhileAnalysisIsRunning(t *testing.T) {
	store := &memoryStore{id: testID}
	provider := &blockingProvider{started: make(chan struct{}), release: make(chan struct{})}
	handler := NewHandler(store, provider)
	post := newPost(handler, `{"plan":"[ebuild U] dev-libs/widget-2::gentoo [1::gentoo]\n[ebuild U] dev-libs/widget-2::gentoo [1::gentoo]"}`)
	if post.Code != http.StatusAccepted {
		t.Fatalf("POST status = %d: %s", post.Code, post.Body.String())
	}
	select {
	case <-provider.started:
	case <-time.After(time.Second):
		t.Fatal("analysis did not reach metadata lookup")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/analyses/"+testID, nil))
	var job jobResponse
	if err := json.Unmarshal(response.Body.Bytes(), &job); err != nil {
		t.Fatal(err)
	}
	if job.Status != "running" || job.Progress.Stage != "analyzing" || job.Progress.Completed != 0 || job.Progress.Total != 2 {
		t.Fatalf("in-progress job = %#v", job)
	}
	close(provider.release)
	if completed := waitJob(t, handler, testID); completed.Status != "completed" || completed.Progress.Completed != 2 || completed.Progress.Total != 2 {
		t.Fatalf("final job = %#v, want two unique lookups completed", completed)
	}
}

func TestPostStorageFailureDoesNotReturnCreatedOrID(t *testing.T) {
	store := &memoryStore{id: testID, saveErr: errors.New("database unavailable")}
	handler := NewHandler(store, &sequenceProvider{})
	request := httptest.NewRequest(http.MethodPost, "/api/v1/analyses", strings.NewReader(`{"plan":"unsupported input"}`))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("POST status = %d, want 503", response.Code)
	}
	if strings.Contains(response.Body.String(), testID) || strings.Contains(response.Body.String(), `"id"`) {
		t.Fatalf("failed save response leaked an ID: %s", response.Body.String())
	}
}

func TestUnavailableProviderOnlyBlocksPost(t *testing.T) {
	savedResult := []byte(`{"Plan":{"Status":"parsed"},"Analysis":{"Status":"parsed","Diagnostics":[],"Findings":[]},"Reports":[]}`)
	store := &memoryStore{id: testID}
	var unavailable *metadataprovider.Provider
	handler := NewHandler(store, unavailable)

	post := httptest.NewRequest(http.MethodPost, "/api/v1/analyses", strings.NewReader(`{"plan":"[ebuild U] dev-libs/libei-1.6.0-r1::gentoo [1.6.0::gentoo]"}`))
	post.Header.Set("Content-Type", "application/json")
	postResponse := httptest.NewRecorder()
	handler.ServeHTTP(postResponse, post)
	if postResponse.Code != http.StatusAccepted {
		t.Fatalf("POST status = %d, want 202", postResponse.Code)
	}
	if got := waitJob(t, handler, testID); got.Status != "failed" {
		t.Fatalf("job status = %q, want failed", got.Status)
	}
	if store.createCalls != 1 {
		t.Fatalf("accepted job persistence attempted %d creates, want 1", store.createCalls)
	}

	legacyHandler := NewHandler(&memoryStore{id: testID, result: savedResult}, unavailable)
	get := httptest.NewRequest(http.MethodGet, "/api/v1/analyses/"+testID, nil)
	getResponse := httptest.NewRecorder()
	legacyHandler.ServeHTTP(getResponse, get)
	if getResponse.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200: %s", getResponse.Code, getResponse.Body.String())
	}
	var got jobResponse
	if err := json.Unmarshal(getResponse.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ID != testID || got.Status != "completed" || !jsonEqual(got.Result, mustV1Result(t, savedResult)) {
		t.Fatalf("GET changed saved analysis: ID=%q result=%s", got.ID, got.Result)
	}
}

func TestResumeContinuesInterruptedJob(t *testing.T) {
	store := &memoryStore{id: testID, jobs: map[string]analysisstore.Job{
		testID: {ID: testID, Request: []byte(`{"plan":"[ebuild U] dev-libs/widget-2::gentoo [1::gentoo]"}`), Status: "running", Stage: "analyzing"},
	}}
	handler := NewHandler(store, &sequenceProvider{})
	if err := handler.Resume(context.Background()); err != nil {
		t.Fatal(err)
	}
	job := waitJob(t, handler, testID)
	if job.Status != "completed" || job.Progress.Total != 2 || job.Progress.Completed != 2 || len(job.Result) == 0 {
		t.Fatalf("resumed job status/progress/result = %s/%d/%d/%t", job.Status, job.Progress.Completed, job.Progress.Total, len(job.Result) > 0)
	}
}

func TestResumePassesPersistedRepositoryRevisionsToProvider(t *testing.T) {
	pinned := []byte(`{"gentoo":{"Name":"gentoo","Commit":"` + strings.Repeat("a", 40) + `"}}`)
	store := &memoryStore{id: testID, jobs: map[string]analysisstore.Job{
		testID: {ID: testID, Request: []byte(`{"plan":"[ebuild U] dev-libs/widget-2::gentoo [1::gentoo]"}`), Status: "running", Stage: "analyzing", Revisions: pinned},
	}}
	called := make(chan bool, 1)
	handler := NewHandlerWithPinnedProviderFactory(store, func(_ context.Context, _ parser.Result, revisions map[string]repositorymanager.Revision) (reportservice.MetadataLookup, error) {
		called <- revisions["gentoo"].Commit == strings.Repeat("a", 40)
		return &sequenceProvider{}, nil
	})
	if err := handler.Resume(context.Background()); err != nil {
		t.Fatal(err)
	}
	if completed := waitJob(t, handler, testID); completed.Status != "completed" {
		t.Fatalf("resumed job = %#v", completed)
	}
	if !<-called {
		t.Fatal("provider factory did not receive the persisted Gentoo revision")
	}
}

func TestCompletionUpdateRetriesTemporaryStorageFailure(t *testing.T) {
	store := &memoryStore{id: testID, failCompleted: 1}
	handler := NewHandler(store, &sequenceProvider{})
	if response := newPost(handler, `{"plan":"[ebuild U] dev-libs/widget-2::gentoo [1::gentoo]"}`); response.Code != http.StatusAccepted {
		t.Fatalf("POST status = %d: %s", response.Code, response.Body.String())
	}
	completed := waitJob(t, handler, testID)
	if completed.Status != "completed" || len(completed.Result) == 0 {
		t.Fatalf("job after retry = %#v", completed)
	}
}

func TestGetUnknownAndInvalidIDsReturnNotFound(t *testing.T) {
	handler := NewHandler(&memoryStore{}, &sequenceProvider{})
	for _, id := range []string{"28b28f31-7e85-4e6a-a839-84658f32cc74", "not-a-uuid"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/analyses/"+id, nil))
		if response.Code != http.StatusNotFound {
			t.Errorf("GET %q status = %d, want 404", id, response.Code)
		}
	}
}

func TestPostRejectsInvalidRequests(t *testing.T) {
	handler := NewHandler(&memoryStore{id: testID}, &sequenceProvider{})
	tests := []struct {
		name        string
		contentType string
		body        string
		wantStatus  int
	}{
		{name: "media type", contentType: "text/plain", body: `{"plan":"x"}`, wantStatus: http.StatusUnsupportedMediaType},
		{name: "unknown field", contentType: "application/json", body: `{"plan":"x","unexpected":true}`, wantStatus: http.StatusBadRequest},
		{name: "empty plan", contentType: "application/json", body: `{"plan":"  "}`, wantStatus: http.StatusBadRequest},
		{name: "invalid package", contentType: "application/json", body: `{"plan":"x","locally_patched_packages":[{"category":"../etc","name":"widget"}]}`, wantStatus: http.StatusBadRequest},
		{name: "trailing JSON", contentType: "application/json", body: `{"plan":"x"} {}`, wantStatus: http.StatusBadRequest},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			request := httptest.NewRequest(http.MethodPost, "/api/v1/analyses", strings.NewReader(test.body))
			request.Header.Set("Content-Type", test.contentType)
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.wantStatus {
				t.Fatalf("status = %d, body = %s, want %d", response.Code, response.Body.String(), test.wantStatus)
			}
		})
	}
}

type memoryStore struct {
	mu            sync.RWMutex
	id            string
	result        []byte
	saveErr       error
	saveCalls     int
	createCalls   int
	failCompleted int
	autoIDs       bool
	jobs          map[string]analysisstore.Job
}

func (s *memoryStore) Save(_ context.Context, result []byte) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saveCalls++
	s.result = append([]byte(nil), result...)
	return s.id, s.saveErr
}

func (s *memoryStore) Get(_ context.Context, id string) ([]byte, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	if id != s.id || s.result == nil {
		return nil, false, nil
	}
	return append([]byte(nil), s.result...), true, nil
}

func (s *memoryStore) CreateJob(_ context.Context, request []byte) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.createCalls++
	if s.saveErr != nil {
		return "", s.saveErr
	}
	if s.jobs == nil {
		s.jobs = make(map[string]analysisstore.Job)
	}
	if s.autoIDs {
		s.id = fmt.Sprintf("00000000-0000-4000-8000-%012d", s.createCalls)
	}
	s.jobs[s.id] = analysisstore.Job{ID: s.id, Request: append([]byte(nil), request...), Status: "pending", Stage: "queued"}
	return s.id, nil
}

func (s *memoryStore) UpdateJob(_ context.Context, update analysisstore.JobUpdate) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if update.Status == "completed" && s.failCompleted > 0 {
		s.failCompleted--
		return errors.New("temporary database error")
	}
	if s.saveErr != nil {
		return s.saveErr
	}
	job := s.jobs[update.ID]
	job.ID, job.Status, job.Stage = update.ID, update.Status, update.Stage
	job.Completed, job.Total, job.Error = update.Completed, update.Total, update.Error
	job.Result = append([]byte(nil), update.Result...)
	if len(update.Revisions) > 0 {
		job.Revisions = append([]byte(nil), update.Revisions...)
	}
	s.jobs[update.ID] = job
	return nil
}

func (s *memoryStore) GetJob(_ context.Context, id string) (analysisstore.Job, bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	job, ok := s.jobs[id]
	job.Request = append([]byte(nil), job.Request...)
	job.Result = append([]byte(nil), job.Result...)
	job.Revisions = append([]byte(nil), job.Revisions...)
	return job, ok, nil
}

func (s *memoryStore) RequeueInterruptedJobs(_ context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	for id, job := range s.jobs {
		if job.Status == "running" {
			job.Status, job.Stage, job.Completed, job.Total = "pending", "queued", 0, 0
			s.jobs[id] = job
		}
	}
	return nil
}

func (s *memoryStore) ClaimNextJob(_ context.Context) (analysisstore.Job, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var ids []string
	for id, job := range s.jobs {
		if job.Status == "pending" {
			ids = append(ids, id)
		}
	}
	sort.Strings(ids)
	if len(ids) == 0 {
		return analysisstore.Job{}, false, nil
	}
	job := s.jobs[ids[0]]
	job.Status, job.Stage = "running", "preparing"
	s.jobs[job.ID] = job
	return job, true, nil
}

func newPost(handler http.Handler, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/analyses", strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}

func waitJob(t *testing.T, handler http.Handler, id string) jobResponse {
	t.Helper()
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/v1/analyses/"+id, nil))
		var job jobResponse
		if err := json.Unmarshal(response.Body.Bytes(), &job); err != nil {
			t.Fatal(err)
		}
		if job.Status == "completed" || job.Status == "failed" {
			return job
		}
		time.Sleep(time.Millisecond)
	}
	t.Fatal("analysis job did not finish")
	return jobResponse{}
}

type sequenceProvider struct{ calls int }

type blockingProvider struct {
	started chan struct{}
	release chan struct{}
	once    sync.Once
}

func (p *blockingProvider) Lookup(query metadataprovider.Query) (metadataprovider.LookupResult, error) {
	p.once.Do(func() {
		close(p.started)
		<-p.release
	})
	return metadataprovider.LookupResult{Status: metadataprovider.StatusNotFound}, nil
}

func (p *sequenceProvider) Lookup(query metadataprovider.Query) (metadataprovider.LookupResult, error) {
	p.calls++
	switch p.calls {
	case 1:
		return metadataprovider.LookupResult{Status: metadataprovider.StatusNotFound}, nil
	case 2:
		return metadataprovider.LookupResult{Status: metadataprovider.StatusRepositoryUnavailable}, nil
	case 3:
		return metadataprovider.LookupResult{}, errors.New("fixture lookup error")
	default:
		snapshot := metadatadiff.Normalize(metadatadiff.Source{
			Repository: query.Repository, Category: query.Category, Name: query.Name, Version: query.Version,
		}, "EAPI=8\nSLOT=0\nIUSE=\n")
		return metadataprovider.LookupResult{Status: metadataprovider.StatusFound, Snapshot: &snapshot}, nil
	}
}

func jsonEqual(left, right []byte) bool {
	var leftValue any
	var rightValue any
	return json.Unmarshal(left, &leftValue) == nil && json.Unmarshal(right, &rightValue) == nil && equalJSONValues(leftValue, rightValue)
}

func equalJSONValues(left, right any) bool {
	leftJSON, leftErr := json.Marshal(left)
	rightJSON, rightErr := json.Marshal(right)
	return leftErr == nil && rightErr == nil && string(leftJSON) == string(rightJSON)
}
