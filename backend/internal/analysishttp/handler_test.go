package analysishttp

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/vovanbl411/gentoo-update-intelligence/internal/metadatadiff"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/metadataprovider"
	"github.com/vovanbl411/gentoo-update-intelligence/internal/reportservice"
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
	for _, status := range []int{http.StatusServiceUnavailable, http.StatusCreated} {
		request := httptest.NewRequest(http.MethodPost, "/analyses", strings.NewReader(`{"plan":"[ebuild U] dev-libs/widget-2::gentoo [1::gentoo]"}`))
		request.Header.Set("Content-Type", "application/json")
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		if response.Code != status {
			t.Fatalf("status = %d, want %d: %s", response.Code, status, response.Body.String())
		}
	}
	if calls != 2 || provider.calls != 2 || store.saveCalls != 1 {
		t.Fatalf("factory calls %d, lookup calls %d, saves %d", calls, provider.calls, store.saveCalls)
	}
}

func TestPostPersistsFullResultAndGetReturnsIt(t *testing.T) {
	store := &memoryStore{id: testID}
	provider := &sequenceProvider{}
	handler := NewHandler(store, provider)
	body := `{"plan":"[ebuild U] dev-libs/widget-2::gentoo [1::gentoo]\n[ebuild U] dev-libs/gadget-2::gentoo [1::gentoo]"}`

	post := httptest.NewRequest(http.MethodPost, "/analyses", strings.NewReader(body))
	post.Header.Set("Content-Type", "application/json")
	created := httptest.NewRecorder()
	handler.ServeHTTP(created, post)
	if created.Code != http.StatusCreated {
		t.Fatalf("POST status = %d, body = %s", created.Code, created.Body.String())
	}
	var createdBody response
	if err := json.Unmarshal(created.Body.Bytes(), &createdBody); err != nil {
		t.Fatal(err)
	}
	if createdBody.ID != testID || string(createdBody.Result) != string(store.result) {
		t.Fatalf("POST response does not match saved record: ID=%q result=%s saved=%s", createdBody.ID, createdBody.Result, store.result)
	}
	var saved resultView
	if err := json.Unmarshal(store.result, &saved); err != nil {
		t.Fatal(err)
	}
	if len(saved.Reports) != 2 {
		t.Fatalf("saved report count = %d, want 2", len(saved.Reports))
	}
	if got := saved.Reports[0].BeforeLookup.Result.Status; got != metadataprovider.StatusNotFound {
		t.Fatalf("before lookup status = %q, want not_found", got)
	}
	if got := saved.Reports[0].AfterLookup.Result.Status; got != metadataprovider.StatusRepositoryUnavailable {
		t.Fatalf("after lookup status = %q, want repository_unavailable", got)
	}
	if got := saved.Reports[1].BeforeLookup.Err; got == nil || *got != "fixture lookup error" {
		t.Fatalf("saved Lookup error = %v, want fixture lookup error", got)
	}
	if saved.Reports[1].AfterLookup.Result == nil || saved.Reports[1].AfterLookup.Result.Status != metadataprovider.StatusFound || saved.Reports[1].AfterLookup.Result.Snapshot == nil {
		t.Fatalf("second report did not preserve found Lookup result and snapshot: %#v", saved.Reports[1].AfterLookup)
	}

	get := httptest.NewRequest(http.MethodGet, "/analyses/"+testID, nil)
	loaded := httptest.NewRecorder()
	handler.ServeHTTP(loaded, get)
	if loaded.Code != http.StatusOK {
		t.Fatalf("GET status = %d, body = %s", loaded.Code, loaded.Body.String())
	}
	var loadedBody response
	if err := json.Unmarshal(loaded.Body.Bytes(), &loadedBody); err != nil {
		t.Fatal(err)
	}
	if loadedBody.ID != createdBody.ID || !jsonEqual(loadedBody.Result, createdBody.Result) {
		t.Fatalf("GET differs from POST: POST=%s GET=%s", created.Body.Bytes(), loaded.Body.Bytes())
	}
}

func TestPostStorageFailureDoesNotReturnCreatedOrID(t *testing.T) {
	store := &memoryStore{id: testID, saveErr: errors.New("database unavailable")}
	handler := NewHandler(store, &sequenceProvider{})
	request := httptest.NewRequest(http.MethodPost, "/analyses", strings.NewReader(`{"plan":"unsupported input"}`))
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
	store := &memoryStore{id: testID, result: savedResult}
	var unavailable *metadataprovider.Provider
	handler := NewHandler(store, unavailable)

	post := httptest.NewRequest(http.MethodPost, "/analyses", strings.NewReader(`{"plan":"[ebuild U] dev-libs/libei-1.6.0-r1::gentoo [1.6.0::gentoo]"}`))
	post.Header.Set("Content-Type", "application/json")
	postResponse := httptest.NewRecorder()
	handler.ServeHTTP(postResponse, post)
	if postResponse.Code != http.StatusServiceUnavailable {
		t.Fatalf("POST status = %d, want 503", postResponse.Code)
	}
	if strings.Contains(postResponse.Body.String(), testID) || strings.Contains(postResponse.Body.String(), `"id"`) {
		t.Fatalf("POST without Provider returned an ID: %s", postResponse.Body.String())
	}
	if store.saveCalls != 0 {
		t.Fatalf("POST without Provider attempted %d saves, want 0", store.saveCalls)
	}

	get := httptest.NewRequest(http.MethodGet, "/analyses/"+testID, nil)
	getResponse := httptest.NewRecorder()
	handler.ServeHTTP(getResponse, get)
	if getResponse.Code != http.StatusOK {
		t.Fatalf("GET status = %d, want 200: %s", getResponse.Code, getResponse.Body.String())
	}
	var got response
	if err := json.Unmarshal(getResponse.Body.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.ID != testID || !jsonEqual(got.Result, savedResult) {
		t.Fatalf("GET changed saved analysis: ID=%q result=%s", got.ID, got.Result)
	}
}

func TestGetUnknownAndInvalidIDsReturnNotFound(t *testing.T) {
	handler := NewHandler(&memoryStore{}, &sequenceProvider{})
	for _, id := range []string{"28b28f31-7e85-4e6a-a839-84658f32cc74", "not-a-uuid"} {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/analyses/"+id, nil))
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
			request := httptest.NewRequest(http.MethodPost, "/analyses", strings.NewReader(test.body))
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
	id        string
	result    []byte
	saveErr   error
	saveCalls int
}

func (s *memoryStore) Save(_ context.Context, result []byte) (string, error) {
	s.saveCalls++
	s.result = append([]byte(nil), result...)
	return s.id, s.saveErr
}

func (s *memoryStore) Get(_ context.Context, id string) ([]byte, bool, error) {
	if id != s.id || s.result == nil {
		return nil, false, nil
	}
	return append([]byte(nil), s.result...), true, nil
}

type sequenceProvider struct{ calls int }

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
